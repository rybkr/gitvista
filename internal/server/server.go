package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rybkr/gitvista/internal/gitcore"
	"github.com/rybkr/gitvista/internal/repomanager"
)

// Mode distinguishes between local and SaaS operation.
type Mode int

const (
	// ModeLocal serves a single local Git repository.
	ModeLocal Mode = iota
	// ModeSaaS serves multiple cloned repositories via the repo manager.
	ModeSaaS
)

// Server contains all behavior for the GitVista application server.
type Server struct {
	addr        string
	webFS       fs.FS
	rateLimiter *rateLimiter
	httpServer  *http.Server
	logger      *slog.Logger

	mode          Mode
	localSession  *RepoSession             // non-nil in local mode
	sessionsMu    sync.RWMutex             // guards sessions map
	sessions      map[string]*RepoSession  // non-nil in SaaS mode
	repoManager   *repomanager.RepoManager // non-nil in SaaS mode
	cacheSize     int
	fetchInterval time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewServer constructs a local-mode Server ready to be started.
// Backward-compatible alias for NewLocalServer.
func NewServer(repo *gitcore.Repository, addr string, webFS fs.FS) *Server {
	return NewLocalServer(repo, addr, webFS)
}

// NewLocalServer constructs a Server in local mode with a single repository.
func NewLocalServer(repo *gitcore.Repository, addr string, webFS fs.FS) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	rl := newRateLimiter(100, 200, time.Second)

	cacheSize := readCacheSize()

	s := &Server{
		addr:        addr,
		webFS:       webFS,
		rateLimiter: rl,
		logger:      slog.Default(),
		mode:        ModeLocal,
		cacheSize:   cacheSize,
		ctx:         ctx,
		cancel:      cancel,
	}

	s.localSession = NewRepoSession(SessionConfig{
		ID:          "local",
		InitialRepo: repo,
		ReloadFn: func() (*gitcore.Repository, error) {
			return gitcore.NewRepository(repo.GitDir())
		},
		CacheSize: cacheSize,
		Logger:    s.logger,
	})

	return s
}

// NewSaaSServer constructs a Server in SaaS mode backed by a RepoManager.
func NewSaaSServer(rm *repomanager.RepoManager, addr string, webFS fs.FS) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	rl := newRateLimiter(100, 200, time.Second)

	cacheSize := readCacheSize()

	return &Server{
		addr:          addr,
		webFS:         webFS,
		rateLimiter:   rl,
		logger:        slog.Default(),
		mode:          ModeSaaS,
		sessions:      make(map[string]*RepoSession),
		repoManager:   rm,
		cacheSize:     cacheSize,
		fetchInterval: 10 * time.Second,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// readCacheSize reads the cache size from the GITVISTA_CACHE_SIZE env var.
func readCacheSize() int {
	cacheSize := defaultCacheSize
	if raw := os.Getenv("GITVISTA_CACHE_SIZE"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			cacheSize = n
		}
	}
	return cacheSize
}

// getOrCreateSession returns an existing session or lazily creates one when a
// SaaS repo is ready. Uses double-checked locking.
func (s *Server) getOrCreateSession(id string) (*RepoSession, error) {
	if s.mode == ModeLocal {
		if s.localSession != nil {
			return s.localSession, nil
		}
		return nil, fmt.Errorf("no local session available")
	}

	// Fast path: read lock
	s.sessionsMu.RLock()
	session, exists := s.sessions[id]
	s.sessionsMu.RUnlock()
	if exists {
		return session, nil
	}

	// Check that the repo exists and is ready in the RepoManager
	repo, err := s.repoManager.GetRepo(id)
	if err != nil {
		return nil, err
	}

	// Slow path: write lock, double-check
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()

	if session, exists = s.sessions[id]; exists {
		return session, nil
	}

	rm := s.repoManager
	session = NewRepoSession(SessionConfig{
		ID:          id,
		InitialRepo: repo,
		ReloadFn: func() (*gitcore.Repository, error) {
			return rm.GetRepo(id)
		},
		CacheSize: s.cacheSize,
		Logger:    s.logger,
	})
	session.Start()
	if s.fetchInterval > 0 {
		session.StartFetchTicker(s.fetchInterval)
	}
	s.sessions[id] = session

	s.logger.Info("Created session for repo", "id", id)
	return session, nil
}

// removeSession tears down and removes a session by ID.
func (s *Server) removeSession(id string) {
	if s.mode == ModeLocal {
		return
	}

	s.sessionsMu.Lock()
	session, exists := s.sessions[id]
	if exists {
		delete(s.sessions, id)
	}
	s.sessionsMu.Unlock()

	if exists {
		session.Close()
		s.logger.Info("Removed session for repo", "id", id)
	}
}

// Start begins serving and blocks until the server exits or encounters a fatal error.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(s.webFS)))
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/config", s.handleConfig)

	const apiWriteDeadline = 30 * time.Second

	if s.mode == ModeLocal {
		ls := s.localSession
		ls.Start()

		mux.HandleFunc("/api/repository", writeDeadline(apiWriteDeadline, s.rateLimiter.middleware(withLocalSession(ls, s.handleRepository))))
		mux.HandleFunc("/api/tree/blame/", writeDeadline(apiWriteDeadline, s.rateLimiter.middleware(withLocalSession(ls, s.handleTreeBlame))))
		mux.HandleFunc("/api/tree/", writeDeadline(apiWriteDeadline, s.rateLimiter.middleware(withLocalSession(ls, s.handleTree))))
		mux.HandleFunc("/api/blob/", writeDeadline(apiWriteDeadline, s.rateLimiter.middleware(withLocalSession(ls, s.handleBlob))))
		mux.HandleFunc("/api/commit/diff/", writeDeadline(apiWriteDeadline, s.rateLimiter.middleware(withLocalSession(ls, s.handleCommitDiff))))
		mux.HandleFunc("/api/working-tree/diff", writeDeadline(apiWriteDeadline, s.rateLimiter.middleware(withLocalSession(ls, s.handleWorkingTreeDiff))))
		mux.HandleFunc("/api/ws", withLocalSession(ls, s.handleWebSocket))
	} else {
		// Repo management endpoints (SaaS mode only)
		mux.HandleFunc("/api/repos", writeDeadline(apiWriteDeadline, s.rateLimiter.middleware(s.handleRepos)))
		mux.HandleFunc("/api/repos/", writeDeadline(apiWriteDeadline, s.rateLimiter.middleware(s.handleRepoRoutes)))
	}

	// Build the handler chain: logging wraps the mux, and CORS wraps
	// logging in SaaS mode.
	var handler http.Handler = requestLogger(s.logger, mux)
	if s.mode == ModeSaaS {
		handler = corsMiddleware(handler)
	}

	// WriteTimeout must remain 0 because WebSocket connections are long-lived.
	// Non-WebSocket handlers enforce per-response write deadlines via the
	// writeDeadline middleware applied at the route level.
	s.httpServer = &http.Server{
		Addr:         s.addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	if s.mode == ModeLocal {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if err := s.startWatcher(); err != nil {
				s.logger.Error("watcher error", "err", err)
			}
		}()
	}

	s.logger.Info("GitVista server starting", "addr", "http://"+s.addr, "mode", s.modeString())
	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) modeString() string {
	if s.mode == ModeLocal {
		return "local"
	}
	return "saas"
}

// handleRepos dispatches /api/repos to the correct handler based on HTTP method.
func (s *Server) handleRepos(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleAddRepo(w, r)
	case http.MethodGet:
		s.handleListRepos(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRepoRoutes dispatches /api/repos/{id}/... to the correct handler.
// It rewrites r.URL.Path from /api/repos/{id}/tree/{hash} to /api/tree/{hash}
// so that existing handlers (which strip /api/tree/ etc.) work unchanged.
func (s *Server) handleRepoRoutes(w http.ResponseWriter, r *http.Request) {
	// Strip /api/repos/ prefix to get "{id}" or "{id}/..."
	path := r.URL.Path[len("/api/repos/"):]
	if path == "" {
		http.Error(w, "Missing repo ID", http.StatusBadRequest)
		return
	}

	// Extract id and remainder
	id := path
	remainder := ""
	if idx := strings.IndexByte(path, '/'); idx >= 0 {
		id = path[:idx]
		remainder = path[idx:]
	}

	// Non-session routes: status and delete operate on the repo ID directly.
	switch {
	case remainder == "/status" && r.Method == http.MethodGet:
		s.handleRepoStatus(w, r, id)
		return
	case remainder == "" && r.Method == http.MethodDelete:
		s.handleRemoveRepo(w, r, id)
		return
	}

	// Session-scoped routes: resolve the session using the already-extracted
	// ID, then rewrite the URL path so handlers see the same prefix they
	// expect in local mode (e.g. /api/tree/{hash}).
	session, err := s.getOrCreateSession(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	r.URL.Path = "/api" + remainder
	r = r.WithContext(withSessionCtx(r.Context(), session))

	switch {
	case remainder == "/repository" && r.Method == http.MethodGet:
		s.handleRepository(w, r)
	case strings.HasPrefix(remainder, "/tree/blame/"):
		s.handleTreeBlame(w, r)
	case strings.HasPrefix(remainder, "/tree/"):
		s.handleTree(w, r)
	case strings.HasPrefix(remainder, "/blob/"):
		s.handleBlob(w, r)
	case strings.HasPrefix(remainder, "/commit/diff/"):
		s.handleCommitDiff(w, r)
	case remainder == "/working-tree/diff" && r.Method == http.MethodGet:
		s.handleWorkingTreeDiff(w, r)
	case remainder == "/ws":
		s.handleWebSocket(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// Shutdown gracefully shuts down the server and all sessions.
func (s *Server) Shutdown() {
	start := time.Now()
	s.logger.Info("Server shutting down")

	if s.httpServer != nil {
		s.logger.Info("Stopping HTTP listener")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("HTTP server shutdown error", "err", err)
		}
		s.logger.Info("HTTP listener stopped", "elapsed", time.Since(start).Round(time.Millisecond))
	}

	s.logger.Info("Canceling server context")
	s.cancel()
	s.rateLimiter.Close()

	s.logger.Info("Waiting for watcher goroutines to exit")
	s.wg.Wait()
	s.logger.Info("Watcher goroutines stopped")

	// Close all sessions (sends close frames, force-closes connections)
	if s.mode == ModeLocal {
		if s.localSession != nil {
			s.localSession.Close()
		}
	} else {
		s.sessionsMu.Lock()
		sessionCount := len(s.sessions)
		s.sessionsMu.Unlock()
		s.logger.Info("Closing sessions", "count", sessionCount)

		s.sessionsMu.Lock()
		for id, session := range s.sessions {
			session.Close()
			delete(s.sessions, id)
		}
		s.sessionsMu.Unlock()
	}

	s.logger.Info("Server shutdown complete", "elapsed", time.Since(start).Round(time.Millisecond))
}
