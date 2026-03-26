package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/analytics"
	"github.com/rybkr/gitvista/internal/repositoryview"
)

func (s *Server) handleGraphSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := SessionFromContext(r.Context())
	if session == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}
	session.updateRepository()
	repo := session.Repo()
	if repo == nil {
		http.Error(w, "Repository not available", http.StatusServiceUnavailable)
		return
	}
	summary := repositoryview.BuildGraphSummary(repo)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(summary); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) handleGraphCommits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := SessionFromContext(r.Context())
	if session == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}
	repo := session.Repo()
	if repo == nil {
		http.Error(w, "Repository not available", http.StatusServiceUnavailable)
		return
	}

	hashesParam := r.URL.Query().Get("hashes")
	if hashesParam == "" {
		http.Error(w, "Missing hashes parameter", http.StatusBadRequest)
		return
	}

	rawHashes := strings.Split(hashesParam, ",")
	const maxHashes = 500
	hashes := make([]gitcore.Hash, 0, min(len(rawHashes), maxHashes))
	for _, h := range rawHashes {
		h = strings.TrimSpace(h)
		if _, err := gitcore.NewHash(h); err == nil {
			hashes = append(hashes, gitcore.Hash(h))
		}
		if len(hashes) >= maxHashes {
			break
		}
	}

	response := graphCommitsResponse{Commits: repositoryview.AttributedCommits(repo, hashes)}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := SessionFromContext(r.Context())
	if session == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}
	repo := session.Repo()
	if repo == nil {
		http.Error(w, "Repository not available", http.StatusServiceUnavailable)
		return
	}

	query, err := analytics.ParseQuery(
		r.URL.Query().Get("period"),
		r.URL.Query().Get("start"),
		r.URL.Query().Get("end"),
	)
	if err != nil {
		http.Error(w, "Invalid analytics query", http.StatusBadRequest)
		return
	}

	cacheKey := analytics.CacheKey(repo, query.CacheKey)
	if cached, ok := session.diffCache.Get(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		if encodeErr := json.NewEncoder(w).Encode(cached); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	response, err := analytics.Build(repo, query)
	if err != nil {
		s.logger.Error("Failed to build analytics", "query", query.CacheKey, "err", err)
		http.Error(w, "Failed to build analytics", http.StatusInternalServerError)
		return
	}
	session.diffCache.Put(cacheKey, response)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
