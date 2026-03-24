package hosted

import (
	"io/fs"
	"log/slog"

	"github.com/rybkr/gitvista/internal/repomanager"
	"github.com/rybkr/gitvista/internal/server"
)

// Server composes the hosted product on top of the shared single-repo server.
type Server struct {
	base    *server.Server
	handler *Handler
}

func NewServer(rm *repomanager.RepoManager, addr string, allowedOrigins map[string]bool, store HostedStore, webFS fs.FS, docsFS fs.FS, installScript func() ([]byte, error)) *Server {
	base := server.NewFrontendServer(addr, webFS, server.FrontendConfig{
		IndexPath:   "site/index.html",
		SPAFallback: true,
		ConfigMode:  "hosted",
	})

	handler := NewHandler(base, rm, store)
	handler.Logger = base.Logger()
	handler.AllowedOrigins = allowedOrigins
	handler.DocsFS = docsFS
	handler.InstallScript = installScript
	handler.CacheSize = base.CacheSize()

	base.AddRoutes(handler.RegisterRoutes)
	base.AddMiddleware(handler.Middleware)
	base.AddShutdownHook(handler.Close)

	return &Server{
		base:    base,
		handler: handler,
	}
}

func (s *Server) Start() error {
	return s.base.Start()
}

func (s *Server) Shutdown() {
	s.base.Shutdown()
}

func (s *Server) Logger() *slog.Logger {
	return s.base.Logger()
}

func (s *Server) CacheSize() int {
	return s.base.CacheSize()
}

func (s *Server) Base() *server.Server {
	return s.base
}

func (s *Server) Handler() *Handler {
	return s.handler
}
