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
	http.Handle("/", http.FileServer(http.FS(s.webFS)))
	http.HandleFunc("/health", s.handleHealth)
	http.HandleFunc("/api/repository", s.rateLimiter.middleware(s.handleRepository))
	http.HandleFunc("/api/tree/blame/", s.rateLimiter.middleware(s.handleTreeBlame))
	http.HandleFunc("/api/tree/", s.rateLimiter.middleware(s.handleTree))
	http.HandleFunc("/api/blob/", s.rateLimiter.middleware(s.handleBlob))
	http.HandleFunc("/api/commit/diff/", s.rateLimiter.middleware(s.handleCommitDiff))
	http.HandleFunc("/api/working-tree/diff", s.rateLimiter.middleware(s.handleWorkingTreeDiff))
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

func (s *Server) Shutdown() {
	log.Printf("%s Server shutting down...", logInfo)
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
