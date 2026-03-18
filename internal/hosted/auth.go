package hosted

import (
	"net/http"
	"strings"
)

const (
	HostedRepoTokenHeader = "X-GitVista-Repo-Token"
	HostedRepoTokenQuery  = "access_token"
)

func hostedRepoToken(r *http.Request) string {
	token := strings.TrimSpace(r.Header.Get(HostedRepoTokenHeader))
	if token != "" {
		return token
	}
	return strings.TrimSpace(r.URL.Query().Get(HostedRepoTokenQuery))
}

func (h *Handler) AuthorizeRepo(accountSlug, repoID string, r *http.Request) (HostedRepo, error) {
	if h.Store == nil {
		return HostedRepo{}, ErrHostedRepoNotFound
	}
	return h.Store.AuthorizeRepo(accountSlug, repoID, hostedRepoToken(r))
}
