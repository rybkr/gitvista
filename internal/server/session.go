package server

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rybkr/gitvista/internal/gitcore"
)

// ReloadFunc returns a freshly-loaded repository, used by updateRepository to
// reload state from disk. For local mode this calls gitcore.NewRepository; for
// SaaS mode it returns the latest repo pointer from the RepoManager.
type ReloadFunc func() (*gitcore.Repository, error)

// RepoSession holds per-repository state that was previously embedded in the
// monolithic Server struct. Each session manages its own cached repository,
// WebSocket clients, broadcast channel, and LRU caches.
type RepoSession struct {
	id       string
	logger   *slog.Logger
	reloadFn ReloadFunc

	cacheMu sync.RWMutex
	cached  struct{ repo *gitcore.Repository }

	clientsMu sync.RWMutex
	clients   map[*websocket.Conn]*sync.Mutex

	broadcast chan UpdateMessage

	blameCache *LRUCache[any]
	diffCache  *LRUCache[any]

	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	clientWg sync.WaitGroup // tracks clientReadPump/clientWritePump goroutines
}

// SessionConfig holds initialization parameters for a RepoSession.
type SessionConfig struct {
	ID          string
	InitialRepo *gitcore.Repository
	ReloadFn    ReloadFunc
	CacheSize   int
	Logger      *slog.Logger
}

// NewRepoSession constructs a RepoSession ready to be started.
func NewRepoSession(cfg SessionConfig) *RepoSession {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.CacheSize <= 0 {
		cfg.CacheSize = defaultCacheSize
	}

	ctx, cancel := context.WithCancel(context.Background())

	rs := &RepoSession{
		id:         cfg.ID,
		logger:     cfg.Logger.With("session", cfg.ID),
		reloadFn:   cfg.ReloadFn,
		clients:    make(map[*websocket.Conn]*sync.Mutex),
		broadcast:  make(chan UpdateMessage, broadcastChannelSize),
		blameCache: NewLRUCache[any](cfg.CacheSize),
		diffCache:  NewLRUCache[any](cfg.CacheSize),
		ctx:        ctx,
		cancel:     cancel,
	}
	rs.cached.repo = cfg.InitialRepo

	return rs
}

// Repo returns the current cached repository in a thread-safe manner.
func (rs *RepoSession) Repo() *gitcore.Repository {
	rs.cacheMu.RLock()
	repo := rs.cached.repo
	rs.cacheMu.RUnlock()
	return repo
}

// Start launches the broadcast goroutine.
func (rs *RepoSession) Start() {
	rs.wg.Add(1)
	go rs.handleBroadcast()
}

// Close cancels the session context, waits for server-side goroutines, sends
// WebSocket close frames to all clients, then force-closes connections.
func (rs *RepoSession) Close() {
	rs.cancel()
	rs.wg.Wait()

	// Send close frames to all connected clients.
	rs.clientsMu.RLock()
	clients := make([]*websocket.Conn, 0, len(rs.clients))
	for conn := range rs.clients {
		clients = append(clients, conn)
	}
	clientCount := len(clients)
	rs.clientsMu.RUnlock()

	if clientCount > 0 {
		rs.logger.Info("Sending close frames to WebSocket clients", "count", clientCount)
		closeMsg := websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down")
		deadline := time.Now().Add(1 * time.Second)
		for _, conn := range clients {
			_ = conn.WriteControl(websocket.CloseMessage, closeMsg, deadline)
		}

		// Brief grace period for clients to acknowledge the close frame.
		time.Sleep(500 * time.Millisecond)
	}

	// Force-close all remaining connections.
	rs.clientsMu.Lock()
	for conn := range rs.clients {
		if err := conn.Close(); err != nil {
			rs.logger.Error("Failed to close client connection", "err", err)
		}
	}
	rs.clients = make(map[*websocket.Conn]*sync.Mutex)
	rs.clientsMu.Unlock()

	// Wait for pump goroutines to finish (they will exit once connections close).
	rs.clientWg.Wait()

	if clientCount > 0 {
		rs.logger.Info("All WebSocket connections closed")
	}
}

// updateRepository reloads repository state and broadcasts changes to clients.
func (rs *RepoSession) updateRepository() {
	rs.logger.Debug("Updating repository")

	rs.cacheMu.RLock()
	oldRepo := rs.cached.repo
	rs.cacheMu.RUnlock()

	newRepo, err := rs.reloadFn()
	if err != nil {
		rs.logger.Error("Failed to reload repository", "err", err)
		return
	}

	var delta *gitcore.RepositoryDelta
	if oldRepo != nil {
		delta = newRepo.Diff(oldRepo)
	} else {
		delta = newRepo.Diff(gitcore.NewEmptyRepository())
	}

	rs.cacheMu.Lock()
	rs.cached.repo = newRepo
	rs.cacheMu.Unlock()

	status := getWorkingTreeStatus(newRepo)

	var headInfo *HeadInfo
	headChanged := oldRepo == nil ||
		oldRepo.Head() != newRepo.Head() ||
		oldRepo.HeadRef() != newRepo.HeadRef() ||
		oldRepo.HeadDetached() != newRepo.HeadDetached()
	if headChanged {
		headInfo = buildHeadInfo(newRepo)
	}

	if !delta.IsEmpty() || status != nil || headInfo != nil {
		rs.broadcastUpdate(UpdateMessage{Delta: delta, Status: status, Head: headInfo})
	} else {
		rs.logger.Debug("No changes detected after repository reload")
	}
}

// handleBroadcast reads from the broadcast channel and sends messages to all
// connected WebSocket clients. Runs until the session context is canceled.
func (rs *RepoSession) handleBroadcast() {
	defer rs.wg.Done()

	for {
		select {
		case <-rs.ctx.Done():
			rs.logger.Debug("Broadcast handler exiting")
			return
		case message := <-rs.broadcast:
			rs.sendToAllClients(message)
		}
	}
}

// sendToAllClients writes a message to every connected WebSocket client.
// Clients that fail to receive the message are removed.
func (rs *RepoSession) sendToAllClients(message UpdateMessage) {
	var failedClients []*websocket.Conn

	rs.clientsMu.RLock()
	snapshot := make(map[*websocket.Conn]*sync.Mutex, len(rs.clients))
	for conn, mu := range rs.clients {
		snapshot[conn] = mu
	}
	rs.clientsMu.RUnlock()

	for conn, mu := range snapshot {
		mu.Lock()
		err1 := conn.SetWriteDeadline(time.Now().Add(writeWait))
		var err2 error
		if err1 == nil {
			err2 = conn.WriteJSON(message)
		}
		mu.Unlock()

		if err1 != nil {
			rs.logger.Error("Failed to set write deadline", "addr", conn.RemoteAddr(), "err", err1)
			failedClients = append(failedClients, conn)
		} else if err2 != nil {
			rs.logger.Error("Broadcast failed", "addr", conn.RemoteAddr(), "err", err2)
			failedClients = append(failedClients, conn)
		}
	}

	if len(failedClients) > 0 {
		rs.clientsMu.Lock()
		for _, conn := range failedClients {
			delete(rs.clients, conn)
			if err := conn.Close(); err != nil {
				rs.logger.Error("Failed to close client connection", "err", err)
			}
		}
		remainingClients := len(rs.clients)
		rs.clientsMu.Unlock()

		rs.logger.Info("Removed failed clients",
			"removed", len(failedClients),
			"remaining", remainingClients,
		)
	}
}

// broadcastUpdate queues a message for broadcast. Non-blocking: drops the
// message if the channel is full.
func (rs *RepoSession) broadcastUpdate(message UpdateMessage) {
	select {
	case rs.broadcast <- message:
	default:
		rs.logger.Warn("Broadcast channel full, dropping message; clients may be slow")
	}
}

// sendInitialState sends the full repository state to a newly connected client.
func (rs *RepoSession) sendInitialState(conn *websocket.Conn) {
	repo := rs.Repo()

	message := UpdateMessage{
		Delta:  repo.Diff(gitcore.NewEmptyRepository()),
		Status: getWorkingTreeStatus(repo),
		Head:   buildHeadInfo(repo),
	}

	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		rs.logger.Error("Failed to set write deadline", "addr", conn.RemoteAddr(), "err", err)
		return
	}
	if err := conn.WriteJSON(message); err != nil {
		rs.logger.Error("Failed to send initial state", "addr", conn.RemoteAddr(), "err", err)
		return
	}

	rs.logger.Info("Initial state sent", "addr", conn.RemoteAddr())
}

// registerClient adds a WebSocket connection to the session's client map and
// returns the per-connection write mutex.
func (rs *RepoSession) registerClient(conn *websocket.Conn) *sync.Mutex {
	writeMu := &sync.Mutex{}

	rs.clientsMu.Lock()
	rs.clients[conn] = writeMu
	clientCount := len(rs.clients)
	rs.clientsMu.Unlock()

	rs.logger.Info("WebSocket client registered", "addr", conn.RemoteAddr(), "totalClients", clientCount)
	return writeMu
}

// removeClient removes a WebSocket connection from the session's client map
// and closes it.
func (rs *RepoSession) removeClient(conn *websocket.Conn) {
	rs.clientsMu.Lock()
	defer rs.clientsMu.Unlock()

	if _, ok := rs.clients[conn]; ok {
		delete(rs.clients, conn)
		if err := conn.Close(); err != nil {
			rs.logger.Error("Failed to close connection", "addr", conn.RemoteAddr(), "err", err)
		}
		rs.logger.Info("WebSocket client removed", "totalClients", len(rs.clients))
	}
}

// clientReadPump blocks on reads to detect client disconnect, then closes
// the done channel to signal clientWritePump to stop.
func (rs *RepoSession) clientReadPump(conn *websocket.Conn, done chan struct{}) {
	defer rs.clientWg.Done()
	defer func() {
		if r := recover(); r != nil {
			rs.logger.Warn("Recovered panic in clientReadPump", "addr", conn.RemoteAddr(), "panic", r)
		}
		close(done)
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				rs.logger.Error("WebSocket read error", "addr", conn.RemoteAddr(), "err", err)
			}
			return
		}
	}
}

// clientWritePump sends keepalive pings. writeMu serializes writes with broadcasts.
func (rs *RepoSession) clientWritePump(conn *websocket.Conn, done chan struct{}, writeMu *sync.Mutex) {
	defer rs.clientWg.Done()
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	defer rs.removeClient(conn)

	for {
		select {
		case <-done:
			rs.logger.Info("WebSocket client disconnected", "addr", conn.RemoteAddr())
			return

		case <-ticker.C:
			writeMu.Lock()
			err1 := conn.SetWriteDeadline(time.Now().Add(writeWait))
			var err2 error
			if err1 == nil {
				err2 = conn.WriteMessage(websocket.PingMessage, nil)
			}
			writeMu.Unlock()

			if err1 != nil {
				rs.logger.Error("Failed to set write deadline", "addr", conn.RemoteAddr(), "err", err1)
			}
			if err2 != nil {
				rs.logger.Error("WebSocket ping failed", "addr", conn.RemoteAddr(), "err", err2)
				return
			}
		}
	}
}

// StartFetchTicker launches a goroutine that periodically calls updateRepository.
// Used in SaaS mode where the RepoManager fetches upstream changes and swaps the
// repo pointer; the session detects pointer changes and computes/broadcasts deltas.
func (rs *RepoSession) StartFetchTicker(interval time.Duration) {
	rs.wg.Add(1)
	go func() {
		defer rs.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-rs.ctx.Done():
				return
			case <-ticker.C:
				rs.updateRepository()
			}
		}
	}()
}
