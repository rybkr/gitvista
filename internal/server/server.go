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

// Server manages the GitVista web server and WebSocket connections.
type Server struct {
	repo        *gitcore.Repository
	addr        string
	webFS       fs.FS
	rateLimiter *rateLimiter

	cacheMu sync.RWMutex
	cached  struct {
		repo *gitcore.Repository
	}

	clientsMu sync.RWMutex
	clients   map[*websocket.Conn]bool

	broadcast chan UpdateMessage

	// blameCache stores blame results keyed by "commitHash:dirPath"
	blameCache sync.Map

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewServer creates a new GitVista server instance.
func NewServer(repo *gitcore.Repository, addr string, webFS fs.FS) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	// Rate limiter: 100 requests per second with burst of 200
	rateLimiter := newRateLimiter(100, 200, time.Second)

	s := &Server{
		repo:        repo,
		addr:        addr,
		webFS:       webFS,
		rateLimiter: rateLimiter,
		clients:     make(map[*websocket.Conn]bool),
		broadcast:   make(chan UpdateMessage, broadcastChannelSize),
		ctx:         ctx,
		cancel:      cancel,
	}
	s.cached.repo = repo

	return s
}

// Start begins serving HTTP and WebSocket connections.
// It blocks until server exits or encounters a fatal error.
func (s *Server) Start() error {
	http.Handle("/", http.FileServer(http.FS(s.webFS)))

	// Health check has no rate limit
	http.HandleFunc("/health", s.handleHealth)

	// Apply rate limiting to expensive API endpoints
	http.HandleFunc("/api/repository", s.rateLimiter.middleware(s.handleRepository))
	http.HandleFunc("/api/tree/blame/", s.rateLimiter.middleware(s.handleTreeBlame))
	http.HandleFunc("/api/tree/", s.rateLimiter.middleware(s.handleTree))
	http.HandleFunc("/api/blob/", s.rateLimiter.middleware(s.handleBlob))

	// WebSocket has its own connection limits, no rate limit needed
	http.HandleFunc("/api/ws", s.handleWebSocket)

	s.wg.Add(1)
	go s.handleBroadcast()
	go func() {
		if err := s.startWatcher(); err != nil {
			log.Printf("watcher error: %v", err)
		}
	}()

	log.Printf("%s GitVista server starting on http://%s", logSuccess, s.addr)
	//nolint:gosec // G114: Server timeouts configured via reverse proxy in production
	return http.ListenAndServe(s.addr, nil)
}

// Shutdown gracefully stops the server and halts all connections.
func (s *Server) Shutdown() {
	log.Printf("%s Server shutting down...", logInfo)
	s.cancel()

	s.wg.Wait()

	s.clientsMu.Lock()
	for conn := range s.clients {
		if err := conn.Close(); err != nil {
			log.Printf("failed to close client connection: %v", err)
		}
	}
	s.clients = make(map[*websocket.Conn]bool)
	s.clientsMu.Unlock()

	log.Printf("%s Server shutdown complete", logSuccess)
}
