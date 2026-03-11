package server

import (
	"net/http"
	"strings"
)

const (
	hostedRepoTokenHeader = "X-GitVista-Repo-Token"
	hostedRepoTokenQuery  = "access_token"
)

func hostedRepoToken(r *http.Request) string {
	token := strings.TrimSpace(r.Header.Get(hostedRepoTokenHeader))
	if token != "" {
		return token
	}
	return strings.TrimSpace(r.URL.Query().Get(hostedRepoTokenQuery))
}

func (s *Server) authorizeHostedRepo(accountSlug, repoID string, r *http.Request) (HostedRepo, error) {
	if s.hostedStore == nil {
		return HostedRepo{}, errHostedRepoNotFound
	}
	return s.hostedStore.AuthorizeRepo(accountSlug, repoID, hostedRepoToken(r))
}
