package server

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/repositoryview"
)

const broadcastChannelSize = 256

const (
	initialBootstrapWindowDays  = 30
	bootstrapFirstBatchTarget   = 450 * 1024 // bytes (estimated JSON payload)
	bootstrapBatchTarget        = 300 * 1024 // bytes (estimated JSON payload)
	bootstrapMaxCommitsPerBatch = 300
	bootstrapBatchPause         = 8 * time.Millisecond
	forceModeMaxCommits         = 10000
)

// ReloadFunc returns a freshly-loaded repository, used by updateRepository to
// reload state from disk. For local mode this calls gitcore.NewRepository; for
// hosted mode it returns the latest repo pointer from the RepoManager.
type ReloadFunc func() (*gitcore.Repository, error)

// RepoSession holds per-repository state that was previously embedded in the
// monolithic Server struct. Each session manages its own cached repository,
// WebSocket clients, broadcast channel, and LRU caches.
type RepoSession struct {
	id       string
	logger   *slog.Logger
	reloadFn ReloadFunc

	updateMu sync.Mutex
	cacheMu  sync.RWMutex
	cached   struct{ repo *gitcore.Repository }

	clientsMu sync.RWMutex
	clients   map[*websocket.Conn]*sync.Mutex

	broadcast chan UpdateMessage

	blameCache *LRUCache[any]
	diffCache  *LRUCache[any]

	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	clientWg sync.WaitGroup

	analyticsMu  sync.Mutex
	analyticsGen uint64
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
	rs.scheduleAnalyticsPrewarm(cfg.InitialRepo)

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
		time.Sleep(500 * time.Millisecond)
	}

	rs.clientsMu.Lock()
	for conn := range rs.clients {
		if err := conn.Close(); err != nil {
			rs.logger.Error("Failed to close client connection", "err", err)
		}
	}
	rs.clients = make(map[*websocket.Conn]*sync.Mutex)
	rs.clientsMu.Unlock()

	rs.clientWg.Wait()

	if clientCount > 0 {
		rs.logger.Info("All WebSocket connections closed")
	}
}

// updateRepository reloads repository state and broadcasts changes to clients.
// Serialized via updateMu to prevent concurrent reloads from computing
// incorrect deltas against a stale oldRepo.
func (rs *RepoSession) updateRepository() {
	rs.updateMu.Lock()
	defer rs.updateMu.Unlock()

	rs.logger.Debug("Updating repository")

	rs.cacheMu.RLock()
	oldRepo := rs.cached.repo
	rs.cacheMu.RUnlock()

	newRepo, err := rs.reloadFn()
	if err != nil {
		rs.logger.Error("Failed to reload repository", "err", err)
		return
	}

	var delta *repositoryview.RepositoryDelta
	if oldRepo != nil {
		delta = repositoryview.DiffRepositories(newRepo, oldRepo)
	} else {
		delta = repositoryview.DiffRepositories(newRepo, gitcore.NewEmptyRepository())
	}

	rs.cacheMu.Lock()
	rs.cached.repo = newRepo
	rs.cacheMu.Unlock()
	rs.scheduleAnalyticsPrewarm(newRepo)

	status := getWorkingTreeStatus(newRepo)

	var headInfo *HeadInfo
	headChanged := oldRepo == nil ||
		oldRepo.Head() != newRepo.Head() ||
		oldRepo.HeadRef() != newRepo.HeadRef() ||
		oldRepo.HeadDetached() != newRepo.HeadDetached() ||
		!upstreamTrackingEqual(oldRepo.CurrentBranchUpstream(), newRepo.CurrentBranchUpstream())
	if headChanged {
		headInfo = buildHeadInfo(newRepo)
	}

	if !delta.IsEmpty() || status != nil || headInfo != nil {
		if oldRepo == nil && delta != nil && len(delta.AddedCommits) > 0 {
			rs.broadcastInitialBootstrap(delta, status, headInfo)
		} else {
			if delta != nil && !delta.IsEmpty() {
				rs.broadcastUpdate(UpdateMessage{Type: messageTypeGraphDelta, Delta: delta})
			}
			if status != nil {
				rs.broadcastUpdate(UpdateMessage{Type: messageTypeStatus, Status: status})
			}
			if headInfo != nil {
				rs.broadcastUpdate(UpdateMessage{Type: messageTypeHead, Head: headInfo})
			}
		}
	} else {
		rs.logger.Debug("No changes detected after repository reload")
	}
}

func upstreamTrackingEqual(a, b *gitcore.UpstreamTracking) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Ref == b.Ref &&
		a.BranchName == b.BranchName &&
		a.Hash == b.Hash &&
		a.Status == b.Status &&
		a.AheadCount == b.AheadCount &&
		a.BehindCount == b.BehindCount &&
		a.Reason == b.Reason
}
