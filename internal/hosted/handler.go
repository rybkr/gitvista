package hosted

import (
	"bytes"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/repomanager"
	"github.com/rybkr/gitvista/internal/server"
)

type Handler struct {
	Logger         *slog.Logger
	Store          HostedStore
	RepoManager    *repomanager.RepoManager
	Server         *server.Server
	AllowedOrigins map[string]bool
	DocsFS         fs.FS
	InstallScript  func() ([]byte, error)
	CacheSize      int
	FetchInterval  time.Duration

	sessionsMu sync.RWMutex
	sessions   map[string]*server.RepoSession
}

func NewHandler(srv *server.Server, rm *repomanager.RepoManager, store HostedStore) *Handler {
	return &Handler{
		Logger:        slog.Default(),
		Server:        srv,
		RepoManager:   rm,
		Store:         store,
		CacheSize:     500,
		FetchInterval: 10 * time.Second,
		sessions:      make(map[string]*server.RepoSession),
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/docs", serverWriteDeadline(h.HandleDocs))
	mux.HandleFunc("/api/accounts", serverWriteDeadline(h.HandleAccounts))
	mux.HandleFunc("/api/accounts/", serverWriteDeadline(h.HandleAccountRoutes))
	mux.HandleFunc("/api/repos", serverWriteDeadline(h.HandleRepos))
	mux.HandleFunc("/api/repos/", serverWriteDeadline(h.HandleRepoRoutes))
	mux.HandleFunc("/install.sh", h.HandleInstallScript)
}

func (h *Handler) Middleware(next http.Handler) http.Handler {
	return corsMiddleware(h.AllowedOrigins, next)
}

func (h *Handler) Close() error {
	h.sessionsMu.Lock()
	for id, session := range h.sessions {
		session.Close()
		delete(h.sessions, id)
	}
	h.sessionsMu.Unlock()

	if h.Store != nil {
		return h.Store.Close()
	}
	return nil
}

func (h *Handler) logger() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

func hostedSessionKey(accountSlug, repoID string) string {
	return accountSlug + "/" + repoID
}

func (h *Handler) getOrCreateSession(hostedRepo HostedRepo) (*server.RepoSession, error) {
	id := hostedSessionKey(hostedRepo.AccountSlug, hostedRepo.ID)

	h.sessionsMu.RLock()
	session, exists := h.sessions[id]
	h.sessionsMu.RUnlock()
	if exists {
		return session, nil
	}

	repo, err := h.RepoManager.GetRepo(hostedRepo.ManagedRepoID)
	if err != nil {
		return nil, err
	}

	h.sessionsMu.Lock()
	defer h.sessionsMu.Unlock()

	if session, exists = h.sessions[id]; exists {
		return session, nil
	}

	rm := h.RepoManager
	session = server.NewRepoSession(server.SessionConfig{
		ID:          id,
		InitialRepo: repo,
		ReloadFn: func() (*gitcore.Repository, error) {
			return rm.GetRepo(hostedRepo.ManagedRepoID)
		},
		CacheSize: h.CacheSize,
		Logger:    h.logger(),
	})
	session.Start()
	if h.FetchInterval > 0 {
		session.StartFetchTicker(h.FetchInterval)
	}
	h.sessions[id] = session

	h.logger().Info("Created hosted session", "id", id)
	return session, nil
}

func (h *Handler) removeSession(id string) {
	h.sessionsMu.Lock()
	session, exists := h.sessions[id]
	if exists {
		delete(h.sessions, id)
	}
	h.sessionsMu.Unlock()

	if exists {
		session.Close()
		h.logger().Info("Removed hosted session", "id", id)
	}
}

func (h *Handler) HandleRepos(w http.ResponseWriter, r *http.Request) {
	accountSlug := DefaultHostedAccountSlug
	switch r.Method {
	case http.MethodPost:
		h.HandleAddRepo(w, r, accountSlug)
	case http.MethodGet:
		h.HandleListRepos(w, r, accountSlug)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) HandleAccountRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/accounts/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "Missing account slug", http.StatusBadRequest)
		return
	}

	accountSlug := path
	remainder := ""
	if idx := strings.IndexByte(path, '/'); idx >= 0 {
		accountSlug = path[:idx]
		remainder = path[idx:]
	}
	if accountSlug == "" {
		http.Error(w, "Missing account slug", http.StatusBadRequest)
		return
	}

	switch {
	case remainder == "/repos":
		switch r.Method {
		case http.MethodPost:
			h.HandleAddRepo(w, r, accountSlug)
		case http.MethodGet:
			h.HandleListRepos(w, r, accountSlug)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case strings.HasPrefix(remainder, "/repos/"):
		rewritten := *r.URL
		rewritten.Path = strings.TrimPrefix(remainder, "/repos")
		nextReq := r.Clone(r.Context())
		nextReq.URL = &rewritten
		h.handleAccountRepoRoutes(w, nextReq, accountSlug)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func (h *Handler) HandleRepoRoutes(w http.ResponseWriter, r *http.Request) {
	h.handleAccountRepoRoutes(w, r, DefaultHostedAccountSlug)
}

func (h *Handler) handleAccountRepoRoutes(w http.ResponseWriter, r *http.Request, accountSlug string) {
	path := strings.TrimPrefix(r.URL.Path, "/api/repos/")
	if path == r.URL.Path {
		path = strings.TrimPrefix(r.URL.Path, "/")
	}
	if path == "" {
		http.Error(w, "Missing repo ID", http.StatusBadRequest)
		return
	}

	id := path
	remainder := ""
	if idx := strings.IndexByte(path, '/'); idx >= 0 {
		id = path[:idx]
		remainder = path[idx:]
	}

	hostedRepo, err := h.AuthorizeRepo(accountSlug, id, r)
	if err != nil {
		http.Error(w, "Repository not available", http.StatusNotFound)
		return
	}

	switch {
	case remainder == "/status" && r.Method == http.MethodGet:
		h.HandleRepoStatus(w, r, hostedRepo)
		return
	case remainder == "/progress" && r.Method == http.MethodGet:
		h.HandleRepoProgress(w, r, hostedRepo)
		return
	case remainder == "" && r.Method == http.MethodDelete:
		h.HandleRemoveRepo(w, r, hostedRepo)
		return
	}

	session, err := h.getOrCreateSession(hostedRepo)
	if err != nil {
		h.logger().Error("Failed to get or create hosted session", "account", hostedRepo.AccountSlug, "id", hostedRepo.ID, "err", err)
		http.Error(w, "Repository not available", http.StatusNotFound)
		return
	}

	rewritten := *r.URL
	rewritten.Path = "/api" + remainder
	ctx := server.WithSessionContext(r.Context(), session)
	ctx = server.WithRepoNameOverrideContext(ctx, hostedRepo.DisplayName)
	nextReq := r.WithContext(ctx)
	nextReq.URL = &rewritten
	h.Server.HandleRepoRequest(w, nextReq)
}

func (h *Handler) HandleInstallScript(w http.ResponseWriter, r *http.Request) {
	if h.InstallScript == nil {
		http.NotFound(w, r)
		return
	}
	body, err := h.InstallScript()
	if err != nil {
		http.Error(w, "install.sh not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.ServeContent(w, r, "install.sh", time.Time{}, bytes.NewReader(body))
}

func serverWriteDeadline(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Second))
		next(w, r)
	}
}

func corsMiddleware(allowedOrigins map[string]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Vary", "Origin")
		}
		if origin != "" && allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) requireServer() error {
	if h.Server == nil {
		return fmt.Errorf("nil shared server")
	}
	return nil
}
