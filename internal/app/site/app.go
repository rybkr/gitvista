package site

import (
	"github.com/rybkr/gitvista"
	"github.com/rybkr/gitvista/internal/repomanager"
	"github.com/rybkr/gitvista/internal/server"
)

// NewServer constructs the hosted GitVista site server.
func NewServer(rm *repomanager.RepoManager, addr string, allowedOrigins map[string]bool) (*server.Server, error) {
	webFS, err := gitvista.GetSiteWebFS()
	if err != nil {
		return nil, err
	}
	return server.NewHostedServer(rm, addr, webFS, allowedOrigins), nil
}
