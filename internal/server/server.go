package server

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rybkr/gitvista/internal/gitcore"
)

type Server struct {
	repo        *gitcore.Repository
	addr        string
	webFS       fs.FS
	rateLimiter *rateLimiter
	httpServer  *http.Server
	// logger is the structured logger for this server instance. It is
	// initialised from slog.Default() in NewServer so that the global handler
	// configured in main.go (format, level) is inherited automatically, while
	// still being injectable in tests via a null-writer handler.
	logger *slog.Logger

	cacheMu sync.RWMutex
	cached  struct {
		repo *gitcore.Repository
	}

	clientsMu sync.RWMutex
	// clients maps each WebSocket connection to its per-connection write mutex.
	// All writes to a conn (broadcasts and pings) must hold the per-conn mutex
	// to satisfy gorilla/websocket's "one concurrent writer" contract.
	clients map[*websocket.Conn]*sync.Mutex

	broadcast chan UpdateMessage

	// blameCache and diffCache are LRU caches bounded by cacheSize entries.
	// Keys are content-addressed (commit hash + path), so no invalidation is
	// needed on repository reload — a hash collision is cryptographically
	// impossible and stale entries are naturally evicted by the LRU policy.
	blameCache *LRUCache[any] // keyed by "commitHash:dirPath"
	diffCache  *LRUCache[any] // keyed by "commitHash" or "commitHash:filePath:ctxN"

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewServer constructs a Server ready to be started. The structured logger is
// taken from slog.Default() so it respects whatever handler main.go configured
// (text or JSON, level). Tests may override s.logger with a silent handler to
// suppress output.
func NewServer(repo *gitcore.Repository, addr string, webFS fs.FS) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	rateLimiter := newRateLimiter(100, 200, time.Second)

	// Allow operators to tune cache capacity via env var. Values that are
	// missing, zero, or negative fall back to the package default (500).
	cacheSize := defaultCacheSize
	if raw := os.Getenv("GITVISTA_CACHE_SIZE"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			cacheSize = n
		}
	}

	s := &Server{
		repo:        repo,
		addr:        addr,
		webFS:       webFS,
		rateLimiter: rateLimiter,
		logger:      slog.Default(),
		clients:     make(map[*websocket.Conn]*sync.Mutex),
		broadcast:   make(chan UpdateMessage, broadcastChannelSize),
		blameCache:  NewLRUCache[any](cacheSize),
		diffCache:   NewLRUCache[any](cacheSize),
		ctx:         ctx,
		cancel:      cancel,
	}
	s.cached.repo = repo

	return s
}

// Start begins serving and blocks until the server exits or encounters a fatal error.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(s.webFS)))
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/repository", s.rateLimiter.middleware(s.handleRepository))
	mux.HandleFunc("/api/tree/blame/", s.rateLimiter.middleware(s.handleTreeBlame))
	mux.HandleFunc("/api/tree/", s.rateLimiter.middleware(s.handleTree))
	mux.HandleFunc("/api/blob/", s.rateLimiter.middleware(s.handleBlob))
	mux.HandleFunc("/api/commit/diff/", s.rateLimiter.middleware(s.handleCommitDiff))
	mux.HandleFunc("/api/working-tree/diff", s.rateLimiter.middleware(s.handleWorkingTreeDiff))
	mux.HandleFunc("/api/ws", s.handleWebSocket)

	s.httpServer = &http.Server{
		Addr:        s.addr,
		Handler:     mux,
		ReadTimeout: 15 * time.Second,
		// WriteTimeout is intentionally 0: WebSocket connections are long-lived
		// and hijacked from net/http, so the HTTP-level write deadline does not
		// apply to them. Per-message write deadlines are enforced in websocket.go
		// via conn.SetWriteDeadline.
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	s.wg.Add(1)
	go s.handleBroadcast()
	// Reserve the WaitGroup slot for the watchLoop goroutine here, before the
	// outer goroutine starts, so s.wg.Add cannot race with s.wg.Wait in Shutdown.
	s.wg.Add(1)
	go func() {
		if err := s.startWatcher(); err != nil {
			s.logger.Error("watcher error", "err", err)
			s.wg.Done() // watchLoop never started; release the reserved slot
		}
	}()

	s.logger.Info("GitVista server starting", "addr", "http://"+s.addr)
	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) Shutdown() {
	s.logger.Info("Server shutting down")

	// Gracefully drain in-flight HTTP requests before stopping goroutines.
	if s.httpServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("HTTP server shutdown error", "err", err)
		}
	}

	s.cancel()
	s.rateLimiter.Close()

	s.wg.Wait()

	s.clientsMu.Lock()
	for conn := range s.clients {
		if err := conn.Close(); err != nil {
			s.logger.Error("Failed to close client connection", "err", err)
		}
	}
	s.clients = make(map[*websocket.Conn]*sync.Mutex)
	s.clientsMu.Unlock()

	s.logger.Info("Server shutdown complete")
}
