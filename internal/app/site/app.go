package site

import (
	"github.com/rybkr/gitvista"
	"github.com/rybkr/gitvista/internal/hosted"
	"github.com/rybkr/gitvista/internal/repomanager"
	"github.com/rybkr/gitvista/internal/server"
)

// NewServer constructs the hosted GitVista site server.
func NewServer(rm *repomanager.RepoManager, addr string, allowedOrigins map[string]bool, hostedStore hosted.HostedStore) (*server.Server, error) {
	webFS, err := gitvista.GetSiteWebFS()
	if err != nil {
		return nil, err
	}
	docsFS, err := gitvista.GetSiteDocsFS()
	if err != nil {
		return nil, err
	}

	srv := server.NewHostedServer(addr, webFS)
	hostedHandler := hosted.NewHandler(srv, rm, hostedStore)
	hostedHandler.Logger = srv.Logger()
	hostedHandler.AllowedOrigins = allowedOrigins
	hostedHandler.DocsFS = docsFS
	hostedHandler.InstallScript = gitvista.GetInstallScript
	hostedHandler.CacheSize = srv.CacheSize()

	srv.AddRoutes(hostedHandler.RegisterRoutes)
	srv.AddMiddleware(hostedHandler.Middleware)
	srv.AddShutdownHook(hostedHandler.Close)
	return srv, nil
}
