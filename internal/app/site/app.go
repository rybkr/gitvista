package site

import (
	"github.com/rybkr/gitvista"
	"github.com/rybkr/gitvista/internal/repomanager"
	"github.com/rybkr/gitvista/internal/server"
)

// NewServer constructs the hosted GitVista site server.
func NewServer(rm *repomanager.RepoManager, addr string, allowedOrigins map[string]bool, hostedStore server.HostedStore) (*server.Server, error) {
	webFS, err := gitvista.GetSiteWebFS()
	if err != nil {
		return nil, err
	}
	return server.NewHostedServerWithStore(rm, addr, webFS, allowedOrigins, hostedStore), nil
}
