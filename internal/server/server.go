package server

import (
	"context"
	"io/fs"
	"log"
	"net/http"
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

	blameCache sync.Map // keyed by "commitHash:dirPath"
	diffCache  sync.Map // keyed by "commitHash" or "commitHash:filePath:ctxN"

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewServer(repo *gitcore.Repository, addr string, webFS fs.FS) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	rateLimiter := newRateLimiter(100, 200, time.Second)

	s := &Server{
		repo:        repo,
		addr:        addr,
		webFS:       webFS,
		rateLimiter: rateLimiter,
		clients:     make(map[*websocket.Conn]*sync.Mutex),
		broadcast:   make(chan UpdateMessage, broadcastChannelSize),
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
			log.Printf("watcher error: %v", err)
			s.wg.Done() // watchLoop never started; release the reserved slot
		}
	}()

	log.Printf("%s GitVista server starting on http://%s", logSuccess, s.addr)
	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) Shutdown() {
	log.Printf("%s Server shutting down...", logInfo)

	// Gracefully drain in-flight HTTP requests before stopping goroutines.
	if s.httpServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("%s HTTP server shutdown error: %v", logError, err)
		}
	}

	s.cancel()
	s.rateLimiter.Close()

	s.wg.Wait()

	s.clientsMu.Lock()
	for conn := range s.clients {
		if err := conn.Close(); err != nil {
			log.Printf("%s Failed to close client connection: %v", logError, err)
		}
	}
	s.clients = make(map[*websocket.Conn]*sync.Mutex)
	s.clientsMu.Unlock()

	log.Printf("%s Server shutdown complete", logSuccess)
}
