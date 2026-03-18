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

	srv := server.NewHostedServerWithStore(rm, addr, webFS, hostedStore)
	srv.SetAllowedOrigins(allowedOrigins)
	srv.SetDocsFS(docsFS)
	srv.SetInstallScript(gitvista.GetInstallScript)
	return srv, nil
}
