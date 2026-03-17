package local

import (
	"github.com/rybkr/gitvista"
	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/server"
)

// NewServer constructs the local GitVista application server.
func NewServer(repo *gitcore.Repository, addr string) (*server.Server, error) {
	webFS, err := gitvista.GetLocalWebFS()
	if err != nil {
		return nil, err
	}
	return server.NewLocalServer(repo, addr, webFS), nil
}
