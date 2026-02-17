package server

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rybkr/gitvista/internal/gitcore"
)

// Server manages the GitVista web server and WebSocket connections.
type Server struct {
	repo  *gitcore.Repository
	addr  string
	webFS fs.FS

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

	s := &Server{
		repo:      repo,
		addr:      addr,
		webFS:     webFS,
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan UpdateMessage, broadcastChannelSize),
		ctx:       ctx,
		cancel:    cancel,
	}
	s.cached.repo = repo

	return s
}

// Start begins serving HTTP and WebSocket connections.
// It blocks until server exits or encounters a fatal error.
func (s *Server) Start() error {
	http.Handle("/", http.FileServer(http.FS(s.webFS)))

	http.HandleFunc("/health", s.handleHealth)
	http.HandleFunc("/api/repository", s.handleRepository)
	http.HandleFunc("/api/tree/blame/", s.handleTreeBlame)
	http.HandleFunc("/api/tree/", s.handleTree)
	http.HandleFunc("/api/blob/", s.handleBlob)
	http.HandleFunc("/api/ws", s.handleWebSocket)

	s.wg.Add(1)
	go s.handleBroadcast()
	go s.startWatcher()

	log.Printf("%s GitVista server starting on http://%s", logSuccess, s.addr)
	return http.ListenAndServe(s.addr, nil)
}

// Shutdown gracefully stops the server and halts all connections.
func (s *Server) Shutdown() {
	log.Printf("%s Server shutting down...", logInfo)
	s.cancel()

	s.wg.Wait()

	s.clientsMu.Lock()
	for conn := range s.clients {
		conn.Close()
	}
	s.clients = make(map[*websocket.Conn]bool)
	s.clientsMu.Unlock()

	log.Printf("%s Server shutdown complete", logSuccess)
}
