package server

import (
	"net/http"

	"github.com/rybkr/gitvista/internal/hosted"
)

type accountResponse = hosted.AccountResponse
type repoResponse = hosted.RepoResponse

const hostedRepoTokenHeader = hosted.HostedRepoTokenHeader
const hostedRepoTokenQuery = hosted.HostedRepoTokenQuery

func (s *Server) hostedHandler() *hosted.Handler {
	return &hosted.Handler{
		Logger:        s.logger,
		Store:         s.hostedStore,
		RepoManager:   s.repoManager,
		RemoveSession: s.removeSession,
	}
}

func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	s.hostedHandler().HandleAccounts(w, r)
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	s.hostedHandler().HandleListAccounts(w, r)
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	s.hostedHandler().HandleCreateAccount(w, r)
}

func (s *Server) handleAddRepo(w http.ResponseWriter, r *http.Request, accountSlug string) {
	s.hostedHandler().HandleAddRepo(w, r, accountSlug)
}

func (s *Server) handleListRepos(w http.ResponseWriter, r *http.Request, accountSlug string) {
	s.hostedHandler().HandleListRepos(w, r, accountSlug)
}

func (s *Server) handleRepoStatus(w http.ResponseWriter, r *http.Request, hostedRepo HostedRepo) {
	s.hostedHandler().HandleRepoStatus(w, r, hostedRepo)
}

func (s *Server) handleRemoveRepo(w http.ResponseWriter, r *http.Request, hostedRepo HostedRepo) {
	s.hostedHandler().HandleRemoveRepo(w, r, hostedRepo)
}

func (s *Server) handleRepoProgress(w http.ResponseWriter, r *http.Request, hostedRepo HostedRepo) {
	s.hostedHandler().HandleRepoProgress(w, r, hostedRepo)
}

func (s *Server) authorizeHostedRepo(accountSlug, repoID string, r *http.Request) (HostedRepo, error) {
	return s.hostedHandler().AuthorizeRepo(accountSlug, repoID, r)
}
