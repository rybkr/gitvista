package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rybkr/gitvista/internal/gitcore"
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
// SaaS mode it returns the latest repo pointer from the RepoManager.
type ReloadFunc func() (*gitcore.Repository, error)

// RepoSession holds per-repository state that was previously embedded in the
// monolithic Server struct. Each session manages its own cached repository,
// WebSocket clients, broadcast channel, and LRU caches.
type RepoSession struct {
	id       string
	logger   *slog.Logger
	reloadFn ReloadFunc

	updateMu sync.Mutex   // serializes updateRepository calls
	cacheMu  sync.RWMutex // guards cached.repo reads/writes
	cached   struct{ repo *gitcore.Repository }

	clientsMu sync.RWMutex
	clients   map[*websocket.Conn]*sync.Mutex

	broadcast chan UpdateMessage

	blameCache *LRUCache[any]
	diffCache  *LRUCache[any]

	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	clientWg sync.WaitGroup // tracks clientReadPump/clientWritePump goroutines

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

	var delta *gitcore.RepositoryDelta
	if oldRepo != nil {
		delta = newRepo.Diff(oldRepo)
	} else {
		delta = newRepo.Diff(gitcore.NewEmptyRepository())
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
		oldRepo.HeadDetached() != newRepo.HeadDetached()
	if headChanged {
		headInfo = buildHeadInfo(newRepo)
	}

	if !delta.IsEmpty() || status != nil || headInfo != nil {
		if oldRepo == nil && delta != nil && len(delta.AddedCommits) > 0 {
			rs.broadcastInitialBootstrap(delta, status, headInfo)
		} else {
			rs.broadcastUpdate(UpdateMessage{Delta: delta, Status: status, Head: headInfo})
		}
	} else {
		rs.logger.Debug("No changes detected after repository reload")
	}
}

func (rs *RepoSession) broadcastInitialBootstrap(
	delta *gitcore.RepositoryDelta,
	status *WorkingTreeStatus,
	headInfo *HeadInfo,
) {
	batches := planInitialCommitBatches(delta)
	if len(batches) == 0 {
		rs.broadcastUpdate(UpdateMessage{Delta: delta, Status: status, Head: headInfo})
		return
	}

	rs.logger.Info("Streaming initial bootstrap deltas",
		"batches", len(batches),
		"commits", len(delta.AddedCommits),
	)

	for i, batch := range batches {
		if len(batch) == 0 {
			continue
		}

		sent := make(map[gitcore.Hash]struct{}, len(batch))
		for _, c := range batch {
			if c != nil {
				sent[c.ID] = struct{}{}
			}
		}

		batchDelta := gitcore.NewRepositoryDelta()
		batchDelta.AddedCommits = batch
		batchDelta.AddedBranches = filterBranchesBySentHashes(delta.AddedBranches, sent)
		batchDelta.HeadHash = delta.HeadHash
		batchDelta.Bootstrap = true
		batchDelta.BootstrapComplete = i == len(batches)-1

		msg := UpdateMessage{Delta: batchDelta}
		if batchDelta.BootstrapComplete {
			// tags/stashes are replace-style fields on the frontend; send full sets
			// only on the final batch to avoid transient overwrite bugs.
			batchDelta.Tags = delta.Tags
			batchDelta.Stashes = delta.Stashes
			msg.Status = status
			msg.Head = headInfo
		}
		rs.broadcastUpdate(msg)
		if !batchDelta.BootstrapComplete {
			time.Sleep(bootstrapBatchPause)
		}
	}
}

func planInitialCommitBatches(delta *gitcore.RepositoryDelta) [][]*gitcore.Commit {
	if delta == nil || len(delta.AddedCommits) == 0 {
		return nil
	}

	ordered := slices.Clone(delta.AddedCommits)
	slices.SortFunc(ordered, func(a, b *gitcore.Commit) int {
		if a == nil || b == nil {
			if a == nil && b == nil {
				return 0
			}
			if a == nil {
				return 1
			}
			return -1
		}
		if a.Committer.When.Equal(b.Committer.When) {
			return strings.Compare(string(a.ID), string(b.ID))
		}
		if a.Committer.When.After(b.Committer.When) {
			return -1
		}
		return 1
	})

	commitByHash := make(map[gitcore.Hash]*gitcore.Commit, len(ordered))
	for _, c := range ordered {
		if c != nil {
			commitByHash[c.ID] = c
		}
	}

	priority := make(map[gitcore.Hash]struct{})
	for _, h := range collectRefTipHashes(delta) {
		if _, ok := commitByHash[h]; ok {
			priority[h] = struct{}{}
		}
	}

	targetHash := gitcore.Hash(delta.HeadHash)
	targetCommit, ok := commitByHash[targetHash]
	if !ok && len(ordered) > 0 {
		targetCommit = ordered[0]
		targetHash = targetCommit.ID
	}
	if targetHash != "" {
		priority[targetHash] = struct{}{}
	}

	if targetCommit != nil {
		targetSec := targetCommit.Committer.When.Unix()
		windowSec := int64(initialBootstrapWindowDays * 24 * 60 * 60)
		for _, c := range ordered {
			if c == nil {
				continue
			}
			if absInt64(c.Committer.When.Unix()-targetSec) <= windowSec {
				priority[c.ID] = struct{}{}
			}
		}
	}

	priorityOrdered := make([]*gitcore.Commit, 0, len(priority))
	remaining := make([]*gitcore.Commit, 0, len(ordered)-len(priority))
	lightweightRemaining := len(ordered) > forceModeMaxCommits
	for _, c := range ordered {
		if c == nil {
			continue
		}
		if _, ok := priority[c.ID]; ok {
			priorityOrdered = append(priorityOrdered, makeBootstrapCommit(c, false))
			continue
		}
		remaining = append(remaining, makeBootstrapCommit(c, lightweightRemaining))
	}

	batches := make([][]*gitcore.Commit, 0, int(math.Ceil(float64(len(ordered))/float64(bootstrapMaxCommitsPerBatch)))+1)
	appendBatches := func(commits []*gitcore.Commit, firstTarget int, defaultTarget int) {
		if len(commits) == 0 {
			return
		}
		target := firstTarget
		batch := make([]*gitcore.Commit, 0, bootstrapMaxCommitsPerBatch)
		estimated := 0
		for _, c := range commits {
			size := estimateBootstrapCommitSize(c)
			if len(batch) > 0 && (estimated+size > target || len(batch) >= bootstrapMaxCommitsPerBatch) {
				batches = append(batches, batch)
				batch = make([]*gitcore.Commit, 0, bootstrapMaxCommitsPerBatch)
				estimated = 0
				target = defaultTarget
			}
			batch = append(batch, c)
			estimated += size
		}
		if len(batch) > 0 {
			batches = append(batches, batch)
		}
	}

	appendBatches(priorityOrdered, bootstrapFirstBatchTarget, bootstrapBatchTarget)
	appendBatches(remaining, bootstrapBatchTarget, bootstrapBatchTarget)
	return batches
}

func makeBootstrapCommit(c *gitcore.Commit, lightweight bool) *gitcore.Commit {
	if c == nil {
		return nil
	}
	parents := append([]gitcore.Hash(nil), c.Parents...)
	if !lightweight {
		return &gitcore.Commit{
			ID:        c.ID,
			Tree:      c.Tree,
			Parents:   parents,
			Author:    c.Author,
			Committer: c.Committer,
			Message:   c.Message,
		}
	}
	return &gitcore.Commit{
		ID:      c.ID,
		Parents: parents,
		Author: gitcore.Signature{
			When: c.Author.When,
		},
		Committer: gitcore.Signature{
			When: c.Committer.When,
		},
	}
}

func estimateBootstrapCommitSize(c *gitcore.Commit) int {
	if c == nil {
		return 0
	}
	size := 180
	size += len(c.Message)
	size += len(c.Author.Name) + len(c.Author.Email)
	size += len(c.Committer.Name) + len(c.Committer.Email)
	size += len(c.Parents) * 44
	return size
}

func collectRefTipHashes(delta *gitcore.RepositoryDelta) []gitcore.Hash {
	if delta == nil {
		return nil
	}
	out := make([]gitcore.Hash, 0, len(delta.AddedBranches)+len(delta.Tags)+len(delta.Stashes)+1)
	if delta.HeadHash != "" {
		if h, err := gitcore.NewHash(delta.HeadHash); err == nil {
			out = append(out, h)
		}
	}
	for _, h := range delta.AddedBranches {
		out = append(out, h)
	}
	for _, h := range delta.Tags {
		if parsed, err := gitcore.NewHash(h); err == nil {
			out = append(out, parsed)
		}
	}
	for _, s := range delta.Stashes {
		if s != nil && s.Hash != "" {
			out = append(out, s.Hash)
		}
	}
	return out
}

func filterBranchesBySentHashes(branches map[string]gitcore.Hash, sent map[gitcore.Hash]struct{}) map[string]gitcore.Hash {
	filtered := make(map[string]gitcore.Hash)
	for name, hash := range branches {
		if _, ok := sent[hash]; ok {
			filtered[name] = hash
		}
	}
	return filtered
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func (rs *RepoSession) scheduleAnalyticsPrewarm(repo *gitcore.Repository) {
	if repo == nil {
		return
	}

	rs.analyticsMu.Lock()
	rs.analyticsGen++
	gen := rs.analyticsGen
	rs.analyticsMu.Unlock()

	rs.wg.Add(1)
	go func(repo *gitcore.Repository, gen uint64) {
		defer rs.wg.Done()
		periods := []string{"all", "3m", "6m", "1y"}
		for _, period := range periods {
			select {
			case <-rs.ctx.Done():
				return
			default:
			}
			if !rs.analyticsGenCurrent(gen) {
				return
			}

			key := analyticsCacheKey(repo, period)
			if _, ok := rs.diffCache.Get(key); ok {
				continue
			}

			analytics, err := buildAnalytics(repo, analyticsQuery{
				period:   period,
				cacheKey: period,
			})
			if err != nil {
				rs.logger.Warn("Analytics prewarm failed", "period", period, "err", err)
				continue
			}
			if !rs.analyticsGenCurrent(gen) {
				return
			}
			rs.diffCache.Put(key, analytics)
		}
	}(repo, gen)
}

func (rs *RepoSession) analyticsGenCurrent(gen uint64) bool {
	rs.analyticsMu.Lock()
	defer rs.analyticsMu.Unlock()
	return rs.analyticsGen == gen
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

func packetCommitCount(message UpdateMessage) int {
	if message.Delta == nil {
		return 0
	}
	return len(message.Delta.AddedCommits)
}

func marshalPacketPayload(message UpdateMessage) ([]byte, int, error) {
	commitCount := packetCommitCount(message)
	payload, err := json.Marshal(message)
	if err != nil {
		return nil, commitCount, err
	}
	return payload, commitCount, nil
}

func packetType(message UpdateMessage) string {
	switch {
	case message.Delta != nil && message.Delta.BootstrapComplete:
		return "bootstrap-final"
	case message.Delta != nil && message.Delta.Bootstrap:
		return "bootstrap"
	case message.Delta != nil:
		return "delta"
	case message.Summary != nil:
		return "summary"
	default:
		return "state"
	}
}

func logPacketSent(logger *slog.Logger, kind string, clients int, commitCount int, payloadBytes int) {
	logger.Debug("Packet sent",
		"type", kind,
		"clients", clients,
		"commits", commitCount,
		"bytes", payloadBytes,
		"totalBytes", payloadBytes*clients,
	)
}

// sendToAllClients writes a message to every connected WebSocket client.
// Clients that fail to receive the message are removed.
func (rs *RepoSession) sendToAllClients(message UpdateMessage) {
	var failedClients []*websocket.Conn
	payload, commitCount, err := marshalPacketPayload(message)
	if err != nil {
		rs.logger.Error("Failed to serialize outbound packet", "type", packetType(message), "err", err)
		return
	}
	sentClients := 0

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
			err2 = conn.WriteMessage(websocket.TextMessage, payload)
		}
		mu.Unlock()

		if err1 != nil {
			rs.logger.Error("Failed to set write deadline", "addr", conn.RemoteAddr(), "err", err1)
			failedClients = append(failedClients, conn)
		} else if err2 != nil {
			rs.logger.Error("Broadcast failed", "addr", conn.RemoteAddr(), "err", err2)
			failedClients = append(failedClients, conn)
		} else {
			sentClients++
		}
	}

	if sentClients > 0 {
		logPacketSent(rs.logger, packetType(message), sentClients, commitCount, len(payload))
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

// sendInitialState sends lightweight repository state to a newly connected client.
// Full graph summary is fetched over HTTP (/api/graph/summary) during frontend bootstrap
// to avoid very large initial WebSocket messages on big repositories.
func (rs *RepoSession) sendInitialState(conn *websocket.Conn) {
	repo := rs.Repo()
	if repo == nil {
		rs.logger.Error("Failed to send initial state: repository not available", "addr", conn.RemoteAddr())
		return
	}

	status := getWorkingTreeStatus(repo)
	headInfo := buildHeadInfo(repo)
	msg := UpdateMessage{
		Status: status,
		Head:   headInfo,
	}
	payload, commitCount, err := marshalPacketPayload(msg)
	if err != nil {
		rs.logger.Error("Failed to serialize initial state", "addr", conn.RemoteAddr(), "err", err)
		return
	}
	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		rs.logger.Error("Failed to set write deadline", "addr", conn.RemoteAddr(), "err", err)
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		rs.logger.Error("Failed to send initial state", "addr", conn.RemoteAddr(), "err", err)
		return
	}

	logPacketSent(rs.logger, "initial-state", 1, commitCount, len(payload))
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

func buildHeadInfo(repo *gitcore.Repository) *HeadInfo {
	headRef := repo.HeadRef()

	branchName := ""
	if headRef != "" {
		if name, ok := strings.CutPrefix(headRef, "refs/heads/"); ok {
			branchName = name
		}
	}

	tagNames := repo.TagNames()
	recentTags := tagNames
	if len(tagNames) > 5 {
		recentTags = tagNames[:5]
	}

	return &HeadInfo{
		Hash:        string(repo.Head()),
		Ref:         headRef,
		BranchName:  branchName,
		IsDetached:  repo.HeadDetached(),
		CommitCount: repo.CommitCount(),
		BranchCount: len(repo.Branches()),
		TagCount:    len(tagNames),
		Description: repo.Description(),
		Remotes:     repo.Remotes(),
		RecentTags:  recentTags,
	}
}
