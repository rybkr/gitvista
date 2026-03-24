// Package server implements the GitVista HTTP server and WebSocket handlers.
package server

import (
	"bytes"
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rybkr/gitvista/gitcore"
)

const (
	readHeaderTimeout = 5 * time.Second
	maxHeaderBytes    = 1 << 20 // 1 MiB
)

// FrontendConfig controls which frontend shell the server exposes.
type FrontendConfig struct {
	IndexPath   string
	SPAFallback bool
	ConfigMode  string
}

// Server contains all behavior for the GitVista application server.
type Server struct {
	addr        string
	webFS       fs.FS
	indexPath   string
	spaFallback bool
	rateLimiter *rateLimiter
	httpServer  *http.Server
	logger      *slog.Logger

	configMode    string
	localSession  *RepoSession
	cacheSize     int
	extraRoutes   []func(*http.ServeMux)
	middlewares   []func(http.Handler) http.Handler
	shutdownHooks []func() error

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewServer constructs a local-mode Server ready to be started.
// Backward-compatible alias for NewLocalServer.
func NewServer(repo *gitcore.Repository, addr string, webFS fs.FS) *Server {
	return NewSingleRepoServer(repo, addr, webFS)
}

// NewLocalServer constructs a Server in local mode with a single repository.
func NewLocalServer(repo *gitcore.Repository, addr string, webFS fs.FS) *Server {
	return NewSingleRepoServer(repo, addr, webFS)
}

// NewSingleRepoServer constructs a Server that serves a single repository.
func NewSingleRepoServer(repo *gitcore.Repository, addr string, webFS fs.FS) *Server {
	s := NewFrontendServer(addr, webFS, FrontendConfig{
		IndexPath:   "local/index.html",
		SPAFallback: false,
		ConfigMode:  "local",
	})

	s.localSession = NewRepoSession(SessionConfig{
		ID:          "local",
		InitialRepo: repo,
		ReloadFn: func() (*gitcore.Repository, error) {
			return gitcore.NewRepository(repo.GitDir())
		},
		CacheSize: s.cacheSize,
		Logger:    s.logger,
	})

	return s
}

// NewFrontendServer constructs a server shell for a specific frontend.
func NewFrontendServer(addr string, webFS fs.FS, frontend FrontendConfig) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	rl := newRateLimiter(100, 200, time.Second)

	cacheSize := readCacheSize()

	if frontend.IndexPath == "" {
		frontend.IndexPath = "local/index.html"
	}
	if frontend.ConfigMode == "" {
		frontend.ConfigMode = "local"
	}

	return &Server{
		addr:        addr,
		webFS:       webFS,
		indexPath:   frontend.IndexPath,
		spaFallback: frontend.SPAFallback,
		rateLimiter: rl,
		logger:      slog.Default(),
		configMode:  frontend.ConfigMode,
		cacheSize:   cacheSize,
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (s *Server) AddRoutes(register func(*http.ServeMux)) {
	s.extraRoutes = append(s.extraRoutes, register)
}

func (s *Server) AddMiddleware(middleware func(http.Handler) http.Handler) {
	s.middlewares = append(s.middlewares, middleware)
}

func (s *Server) AddShutdownHook(hook func() error) {
	s.shutdownHooks = append(s.shutdownHooks, hook)
}

func (s *Server) Logger() *slog.Logger {
	return s.logger
}

func (s *Server) CacheSize() int {
	return s.cacheSize
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

// Start begins serving and blocks until the server exits or encounters a fatal error.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/config", s.handleConfig)

	if s.localSession != nil {
		ls := s.localSession
		ls.Start()

		mux.HandleFunc("/api/repository", writeDeadline(s.rateLimiter.middleware(withLocalSession(ls, s.handleRepository))))
		mux.HandleFunc("/api/tree/blame/", writeDeadline(s.rateLimiter.middleware(withLocalSession(ls, s.handleTreeBlame))))
		mux.HandleFunc("/api/tree/", writeDeadline(s.rateLimiter.middleware(withLocalSession(ls, s.handleTree))))
		mux.HandleFunc("/api/blob/", writeDeadline(s.rateLimiter.middleware(withLocalSession(ls, s.handleBlob))))
		mux.HandleFunc("/api/commit/diff/", writeDeadline(s.rateLimiter.middleware(withLocalSession(ls, s.handleCommitDiff))))
		mux.HandleFunc("/api/commits/diffstats", writeDeadline(s.rateLimiter.middleware(withLocalSession(ls, s.handleBulkDiffStats))))
		mux.HandleFunc("/api/analytics", writeDeadline(s.rateLimiter.middleware(withLocalSession(ls, s.handleAnalytics))))
		mux.HandleFunc("/api/index/diff", writeDeadline(s.rateLimiter.middleware(withLocalSession(ls, s.handleIndexDiff))))
		mux.HandleFunc("/api/working-tree/diff", writeDeadline(s.rateLimiter.middleware(withLocalSession(ls, s.handleWorkingTreeDiff))))
		mux.HandleFunc("/api/merge-preview/file", writeDeadline(s.rateLimiter.middleware(withLocalSession(ls, s.handleMergePreviewFileDiff))))
		mux.HandleFunc("/api/merge-preview", writeDeadline(s.rateLimiter.middleware(withLocalSession(ls, s.handleMergePreview))))
		mux.HandleFunc("/api/graph/commits", writeDeadline(s.rateLimiter.middleware(withLocalSession(ls, s.handleGraphCommits))))
		mux.HandleFunc("/api/ws", withLocalSession(ls, s.handleWebSocket))
	}
	for _, register := range s.extraRoutes {
		register(mux)
	}
	mux.Handle("/", s.staticHandler())

	handler := requestLogger(s.logger, mux)
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		handler = s.middlewares[i](handler)
	}
	handler = securityHeadersMiddleware(handler)

	s.httpServer = s.newHTTPServer(handler)

	if s.localSession != nil {
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

func (s *Server) staticHandler() http.Handler {
	fileServer := http.FileServer(http.FS(s.webFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			fileServer.ServeHTTP(w, r)
			return
		}

		cleanPath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		if cleanPath == "/" || cleanPath == "/index.html" {
			s.serveIndexHTML(w, r)
			return
		}
		if cleanPath == "/api" || strings.HasPrefix(cleanPath, "/api/") {
			http.NotFound(w, r)
			return
		}

		assetPath := strings.TrimPrefix(cleanPath, "/")
		if s.assetExists(assetPath) {
			fileServer.ServeHTTP(w, r)
			return
		}

		if looksLikeStaticAsset(cleanPath) {
			http.NotFound(w, r)
			return
		}

		if s.spaFallback {
			s.serveIndexHTML(w, r)
			return
		}

		http.NotFound(w, r)
	})
}

func (s *Server) serveIndexHTML(w http.ResponseWriter, r *http.Request) {
	body, err := fs.ReadFile(s.webFS, s.indexPath)
	if err != nil {
		http.Error(w, "frontend index not found", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, path.Base(s.indexPath), time.Time{}, bytes.NewReader(body))
}

func (s *Server) assetExists(name string) bool {
	if name == "" || name == "." {
		return false
	}
	info, err := fs.Stat(s.webFS, name)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func looksLikeStaticAsset(reqPath string) bool {
	base := path.Base(reqPath)
	return strings.Contains(base, ".")
}

func (s *Server) modeString() string {
	if s.configMode != "" {
		return s.configMode
	}
	return "local"
}

// newHTTPServer builds the net/http server with explicit production-safe
// timeout and header size limits.
func (s *Server) newHTTPServer(handler http.Handler) *http.Server {
	// WriteTimeout must remain 0 because WebSocket connections are long-lived.
	// Non-WebSocket handlers enforce per-response write deadlines via the
	// writeDeadline middleware applied at the route level.
	return &http.Server{
		Addr:              s.addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    maxHeaderBytes,
	}
}

func (s *Server) HandleRepoRequest(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/repository" && r.Method == http.MethodGet:
		s.handleRepository(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/tree/blame/"):
		s.handleTreeBlame(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/tree/"):
		s.handleTree(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/blob/"):
		s.handleBlob(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/commit/diff/"):
		s.handleCommitDiff(w, r)
	case r.URL.Path == "/api/commits/diffstats" && r.Method == http.MethodGet:
		s.handleBulkDiffStats(w, r)
	case r.URL.Path == "/api/analytics" && r.Method == http.MethodGet:
		s.handleAnalytics(w, r)
	case r.URL.Path == "/api/index/diff" && r.Method == http.MethodGet:
		s.handleIndexDiff(w, r)
	case r.URL.Path == "/api/graph/commits" && r.Method == http.MethodGet:
		s.handleGraphCommits(w, r)
	case r.URL.Path == "/api/working-tree/diff" && r.Method == http.MethodGet:
		s.handleWorkingTreeDiff(w, r)
	case r.URL.Path == "/api/merge-preview/file" && r.Method == http.MethodGet:
		s.handleMergePreviewFileDiff(w, r)
	case r.URL.Path == "/api/merge-preview" && r.Method == http.MethodGet:
		s.handleMergePreview(w, r)
	case r.URL.Path == "/api/ws":
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

	if s.localSession != nil {
		s.localSession.Close()
	}

	for _, hook := range s.shutdownHooks {
		if err := hook(); err != nil {
			s.logger.Error("Shutdown hook failed", "err", err)
		}
	}

	s.logger.Info("Server shutdown complete", "elapsed", time.Since(start).Round(time.Millisecond))
}
