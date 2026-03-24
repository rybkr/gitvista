package site

import (
	"github.com/rybkr/gitvista"
	"github.com/rybkr/gitvista/internal/hosted"
	"github.com/rybkr/gitvista/internal/repomanager"
)

// NewServer constructs the hosted GitVista site server.
func NewServer(rm *repomanager.RepoManager, addr string, allowedOrigins map[string]bool, hostedStore hosted.HostedStore) (*hosted.Server, error) {
	webFS, err := gitvista.GetSiteWebFS()
	if err != nil {
		return nil, err
	}
	docsFS, err := gitvista.GetSiteDocsFS()
	if err != nil {
		return nil, err
	}

	return hosted.NewServer(rm, addr, allowedOrigins, hostedStore, webFS, docsFS, gitvista.GetInstallScript), nil
}
