package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/analytics"
	"github.com/rybkr/gitvista/internal/repositoryview"
)

func (s *Server) handleMergePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	oursBranch := r.URL.Query().Get("ours")
	theirsBranch := r.URL.Query().Get("theirs")
	if oursBranch == "" || theirsBranch == "" {
		http.Error(w, "Missing 'ours' and/or 'theirs' query parameters", http.StatusBadRequest)
		return
	}

	session := SessionFromContext(r.Context())
	if session == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	repo := session.Repo()
	if repo == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	branches := repo.GraphBranches()
	oursHash, ok := branches[oursBranch]
	if !ok {
		http.Error(w, "Branch not found: ours", http.StatusNotFound)
		return
	}
	theirsHash, ok := branches[theirsBranch]
	if !ok {
		http.Error(w, "Branch not found: theirs", http.StatusNotFound)
		return
	}

	cacheKey := "merge-preview:" + string(oursHash) + ":" + string(theirsHash)
	if cached, ok := session.diffCache.Get(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(cached); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	result, err := gitcore.MergePreview(repo, oursHash, theirsHash)
	if err != nil {
		s.logger.Error("Merge preview computation failed", "ours", oursBranch, "theirs", theirsBranch, "err", err)
		http.Error(w, "Merge preview computation failed", http.StatusInternalServerError)
		return
	}

	response := mergePreviewResponse{
		OursBranch:    oursBranch,
		TheirsBranch:  theirsBranch,
		OursHash:      string(result.OursHash),
		TheirsHash:    string(result.TheirsHash),
		MergeBaseHash: string(result.MergeBaseHash),
		Entries:       result.Entries,
		Stats:         result.Stats,
	}

	session.diffCache.Put(cacheKey, response)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) handleMergePreviewFileDiff(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "Missing 'path' query parameter", http.StatusBadRequest)
		return
	}
	sanitized, err := sanitizePath(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid path: %v", err), http.StatusBadRequest)
		return
	}
	filePath = sanitized

	baseStr := r.URL.Query().Get("base")
	oursStr := r.URL.Query().Get("ours")
	theirsStr := r.URL.Query().Get("theirs")

	var baseHash, oursHash, theirsHash gitcore.Hash
	if baseStr != "" {
		baseHash, err = gitcore.NewHash(baseStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid base hash: %v", err), http.StatusBadRequest)
			return
		}
	}
	if oursStr != "" {
		oursHash, err = gitcore.NewHash(oursStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid ours hash: %v", err), http.StatusBadRequest)
			return
		}
	}
	if theirsStr != "" {
		theirsHash, err = gitcore.NewHash(theirsStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid theirs hash: %v", err), http.StatusBadRequest)
			return
		}
	}

	cacheKey := "merge-file:" + baseStr + ":" + oursStr + ":" + theirsStr + ":" + filePath
	if cached, ok := session.diffCache.Get(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(cached); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	oursUnchanged := oursHash == baseHash
	theirsUnchanged := theirsHash == baseHash

	var response any
	if oursUnchanged && !theirsUnchanged {
		fileDiff, diffErr := gitcore.ComputeFileDiff(repo, baseHash, theirsHash, filePath, gitcore.DefaultContextLines)
		if diffErr != nil {
			s.logger.Error("Failed to compute merge file diff", "path", filePath, "err", diffErr)
			http.Error(w, "Merge file diff computation failed", http.StatusInternalServerError)
			return
		}
		response = mergeUnifiedDiffResponse{
			Mode:      "unified",
			Path:      fileDiff.Path,
			OldHash:   string(fileDiff.OldHash),
			NewHash:   string(fileDiff.NewHash),
			IsBinary:  fileDiff.IsBinary,
			Truncated: fileDiff.Truncated,
			Hunks:     fileDiff.Hunks,
		}
	} else if theirsUnchanged && !oursUnchanged {
		fileDiff, diffErr := gitcore.ComputeFileDiff(repo, baseHash, oursHash, filePath, gitcore.DefaultContextLines)
		if diffErr != nil {
			s.logger.Error("Failed to compute merge file diff", "path", filePath, "err", diffErr)
			http.Error(w, "Merge file diff computation failed", http.StatusInternalServerError)
			return
		}
		response = mergeUnifiedDiffResponse{
			Mode:      "unified",
			Path:      fileDiff.Path,
			OldHash:   string(fileDiff.OldHash),
			NewHash:   string(fileDiff.NewHash),
			IsBinary:  fileDiff.IsBinary,
			Truncated: fileDiff.Truncated,
			Hunks:     fileDiff.Hunks,
		}
	} else {
		threeWay, diffErr := gitcore.ComputeThreeWayDiff(repo, baseHash, oursHash, theirsHash, filePath)
		if diffErr != nil {
			s.logger.Error("Failed to compute three-way diff", "path", filePath, "err", diffErr)
			http.Error(w, "Three-way diff computation failed", http.StatusInternalServerError)
			return
		}
		response = mergeThreeWayDiffResponse{
			Mode:         "three-way",
			Path:         threeWay.Path,
			ConflictType: threeWay.ConflictType,
			IsBinary:     threeWay.IsBinary,
			Truncated:    threeWay.Truncated,
			Regions:      threeWay.Regions,
			Stats:        threeWay.Stats,
		}
	}

	session.diffCache.Put(cacheKey, response)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

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
