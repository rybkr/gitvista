package server

import (
	"github.com/rybkr/gitvista/internal/hosted"
	"github.com/rybkr/gitvista/internal/repomanager"
)

type HostedAccount = hosted.HostedAccount
type HostedRepo = hosted.HostedRepo
type HostedStore = hosted.HostedStore

const DefaultHostedAccountSlug = hosted.DefaultHostedAccountSlug

var errHostedRepoNotFound = hosted.ErrHostedRepoNotFound
var errHostedAccountNotFound = hosted.ErrHostedAccountNotFound

func NewHostedStore(rm *repomanager.RepoManager, databaseURL string) (HostedStore, error) {
	return hosted.NewHostedStore(rm, databaseURL)
}

func NewPostgresHostedStore(dataSourceName string, rm *repomanager.RepoManager) (HostedStore, error) {
	return hosted.NewPostgresHostedStore(dataSourceName, rm)
}

func newMemoryHostedStore(rm *repomanager.RepoManager) HostedStore {
	return hosted.NewMemoryHostedStore(rm)
}
