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

// AppConfig controls which embedded app shell the server exposes.
type AppConfig struct {
	IndexPath   string
	SPAFallback bool
}

// Server contains all behavior for the GitVista application server.
type Server struct {
	addr       string
	webFS      fs.FS
	app        AppConfig
	httpServer *http.Server
	logger     *slog.Logger

	session     *RepoSession
	cacheSize   int
	extraRoutes []func(*http.ServeMux)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewServer constructs a Server ready to serve a single repository on localhost.
func NewServer(repo *gitcore.Repository, addr string, webFS fs.FS) *Server {
	s := newConfiguredServer(addr, webFS, AppConfig{
		SPAFallback: false,
	})

	s.session = NewRepoSession(SessionConfig{
		ID:          "default",
		InitialRepo: repo,
		ReloadFn: func() (*gitcore.Repository, error) {
			return gitcore.NewRepository(repo.GitDir())
		},
		CacheSize: s.cacheSize,
		Logger:    s.logger,
	})

	return s
}

func newConfiguredServer(addr string, webFS fs.FS, app AppConfig) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	cacheSize := readCacheSize()

	if app.IndexPath == "" {
		app.IndexPath = defaultIndexPath(webFS)
	}

	return &Server{
		addr:      addr,
		webFS:     webFS,
		app:       app,
		logger:    slog.Default(),
		cacheSize: cacheSize,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func defaultIndexPath(webFS fs.FS) string {
	if _, err := fs.Stat(webFS, "index.html"); err == nil {
		return "index.html"
	}

	matches, err := fs.Glob(webFS, "*/index.html")
	if err == nil && len(matches) > 0 {
		return matches[0]
	}

	return "index.html"
}

// AddRoutes registers additional HTTP routes on the server mux before startup.
func (s *Server) AddRoutes(register func(*http.ServeMux)) {
	s.extraRoutes = append(s.extraRoutes, register)
}

// Logger returns the server logger.
func (s *Server) Logger() *slog.Logger {
	return s.logger
}

// CacheSize returns the configured cache size for server-side LRU caches.
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
// Start starts the HTTP server and blocks until it exits.
func (s *Server) Start() (err error) {
	started := false
	defer func() {
		if started {
			return
		}
		s.cancel()
		s.wg.Wait()
		if s.session != nil {
			s.session.Close()
		}
	}()

	if s.session != nil {
		s.session.Start()
	}

	mux := s.newServeMux()
	for _, register := range s.extraRoutes {
		register(mux)
	}
	mux.Handle("/", s.staticHandler())

	handler := requestLogger(s.logger, mux)
	handler = securityHeadersMiddleware(handler)

	s.httpServer = s.newHTTPServer(handler)

	if s.session != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if err := s.startWatcher(); err != nil {
				s.logger.Error("watcher error", "err", err)
			}
		}()
	}

	s.logger.Info("GitVista server starting", "addr", "http://"+s.addr)
	err = s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		started = true
		return nil
	}
	if err == nil {
		started = true
	}
	return err
}

func (s *Server) newServeMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/config", s.handleConfig)

	if s.session == nil {
		return mux
	}

	session := s.session
	mux.HandleFunc("/api/repository", writeDeadline(withSession(session, s.handleRepository)))
	mux.HandleFunc("/api/tree/blame/", writeDeadline(withSession(session, s.handleTreeBlame)))
	mux.HandleFunc("/api/tree/", writeDeadline(withSession(session, s.handleTree)))
	mux.HandleFunc("/api/blob/", writeDeadline(withSession(session, s.handleBlob)))
	mux.HandleFunc("/api/commit/diff/", writeDeadline(withSession(session, s.handleCommitDiff)))
	mux.HandleFunc("/api/commits/diffstats", writeDeadline(withSession(session, s.handleBulkDiffStats)))
	mux.HandleFunc("/api/analytics", writeDeadline(withSession(session, s.handleAnalytics)))
	mux.HandleFunc("/api/index/diff", writeDeadline(withSession(session, s.handleIndexDiff)))
	mux.HandleFunc("/api/working-tree/diff", writeDeadline(withSession(session, s.handleWorkingTreeDiff)))
	mux.HandleFunc("/api/merge-preview/file", writeDeadline(withSession(session, s.handleMergePreviewFileDiff)))
	mux.HandleFunc("/api/merge-preview", writeDeadline(withSession(session, s.handleMergePreview)))
	mux.HandleFunc("/api/graph/summary", writeDeadline(withSession(session, s.handleGraphSummary)))
	mux.HandleFunc("/api/graph/commits", writeDeadline(withSession(session, s.handleGraphCommits)))
	mux.HandleFunc("/api/ws", withSession(session, s.handleWebSocket))

	return mux
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

		if s.app.SPAFallback {
			s.serveIndexHTML(w, r)
			return
		}

		http.NotFound(w, r)
	})
}

func (s *Server) serveIndexHTML(w http.ResponseWriter, r *http.Request) {
	body, err := fs.ReadFile(s.webFS, s.app.IndexPath)
	if err != nil {
		http.Error(w, "frontend index not found", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, path.Base(s.app.IndexPath), time.Time{}, bytes.NewReader(body))
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

// newHTTPServer builds the net/http server with explicit production-safe timeout and header size limits.
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

// Shutdown gracefully shuts down the server and its background work.
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

	s.logger.Info("Waiting for watcher goroutines to exit")
	s.wg.Wait()
	s.logger.Info("Watcher goroutines stopped")

	if s.session != nil {
		s.session.Close()
	}

	s.logger.Info("Server shutdown complete", "elapsed", time.Since(start).Round(time.Millisecond))
}
