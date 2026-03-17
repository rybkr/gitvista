package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rybkr/gitvista/gitcore"
)

// extractHashParam validates the request method, extracts a hex hash from the URL
// path after the given prefix, and returns the cached repository and session.
// On failure it writes an HTTP error and returns ok=false.
func (s *Server) extractHashParam(w http.ResponseWriter, r *http.Request, prefix string) (gitcore.Hash, *gitcore.Repository, *RepoSession, bool) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return "", nil, nil, false
	}

	path := strings.TrimPrefix(r.URL.Path, prefix)
	if path == "" || path == r.URL.Path {
		http.Error(w, "Missing hash in path", http.StatusBadRequest)
		return "", nil, nil, false
	}
	path = strings.TrimPrefix(path, "/")

	hash, err := gitcore.NewHash(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid hash format: %v", err), http.StatusBadRequest)
		return "", nil, nil, false
	}

	session := sessionFromCtx(r.Context())
	if session == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return "", nil, nil, false
	}

	repo := session.Repo()
	if repo == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return "", nil, nil, false
	}

	return hash, repo, session, true
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(configResponse{Mode: s.modeString()}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) handleRepository(w http.ResponseWriter, r *http.Request) {
	session := sessionFromCtx(r.Context())
	if session == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	repo := session.Repo()
	if repo == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	currentBranch := ""
	headRef := repo.HeadRef()
	if headRef != "" {
		if name, ok := strings.CutPrefix(headRef, "refs/heads/"); ok {
			currentBranch = name
		}
	}

	branches := repo.Branches()
	tagNames := repo.TagNames()
	repoName := repo.Name()
	if hostedRepo, ok := hostedRepoFromCtx(r.Context()); ok && hostedRepo.DisplayName != "" {
		repoName = hostedRepo.DisplayName
	}

	response := repositoryResponse{
		Name:          repoName,
		CurrentBranch: currentBranch,
		HeadDetached:  repo.HeadDetached(),
		HeadHash:      repo.Head(),
		Upstream:      repo.CurrentBranchUpstream(),
		CommitCount:   repo.CommitCount(),
		BranchCount:   len(branches),
		TagCount:      len(tagNames),
		Tags:          tagNames,
		Description:   repo.Description(),
		Remotes:       repo.Remotes(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	treeHash, repo, _, ok := s.extractHashParam(w, r, "/api/tree/")
	if !ok {
		return
	}

	tree, err := repo.GetTree(treeHash)
	if err != nil {
		s.logger.Error("Failed to load tree", "hash", treeHash, "err", err)
		http.Error(w, "Tree not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tree); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) handleBlob(w http.ResponseWriter, r *http.Request) {
	blobHash, repo, _, ok := s.extractHashParam(w, r, "/api/blob/")
	if !ok {
		return
	}

	content, err := repo.GetBlob(blobHash)
	if err != nil {
		s.logger.Error("Failed to load blob", "hash", blobHash, "err", err)
		http.Error(w, "Blob not found", http.StatusNotFound)
		return
	}

	isBinary := isBinaryContent(content)
	response := blobResponse{
		Hash:      string(blobHash),
		Size:      len(content),
		Binary:    isBinary,
		Truncated: false,
	}

	if isBinary {
		response.Content = ""
	} else {
		const maxSize = 512 * 1024
		if len(content) > maxSize {
			content = content[:maxSize]
			response.Truncated = true
		}
		response.Content = string(content)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// isBinaryContent delegates to gitcore.IsBinaryContent to avoid duplication.
func isBinaryContent(content []byte) bool {
	return gitcore.IsBinaryContent(content)
}

func (s *Server) handleTreeBlame(w http.ResponseWriter, r *http.Request) {
	commitHash, repo, session, ok := s.extractHashParam(w, r, "/api/tree/blame/")
	if !ok {
		return
	}

	dirPath := r.URL.Query().Get("path")
	sanitized, err := sanitizePath(dirPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid path: %v", err), http.StatusBadRequest)
		return
	}
	dirPath = sanitized

	cacheKey := string(commitHash) + ":" + dirPath
	blame, ok := session.blameCache.Get(cacheKey)
	if !ok {
		result, blameErr := repo.GetFileBlame(commitHash, dirPath)
		if blameErr != nil {
			s.logger.Error("Failed to compute blame", "hash", commitHash, "path", dirPath, "err", blameErr)
			http.Error(w, "Blame computation failed", http.StatusNotFound)
			return
		}
		blame = result
		session.blameCache.Put(cacheKey, blame)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(blameEntriesResponse{Entries: blame}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleCommitDiff routes to either the commit-level diff list or the
// file-level line diff based on whether the URL path ends with "/file".
func (s *Server) handleCommitDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/commit/diff/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "Missing commit hash in path", http.StatusBadRequest)
		return
	}

	isFileDiff := strings.HasSuffix(path, "/file")
	commitHashStr := path
	if isFileDiff {
		commitHashStr = strings.TrimSuffix(path, "/file")
	}
	commitHashStr = strings.TrimPrefix(commitHashStr, "/")

	commitHash, err := gitcore.NewHash(commitHashStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid commit hash format: %v", err), http.StatusBadRequest)
		return
	}

	session := sessionFromCtx(r.Context())
	if session == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	repo := session.Repo()
	if repo == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	if isFileDiff {
		s.handleFileDiff(w, r, repo, commitHash, session)
		return
	}
	s.handleCommitDiffList(w, repo, commitHash, session)
}

// resolveCommitAndParent looks up a commit and its first parent's tree hash.
// On failure it writes an HTTP error and returns ok=false.
func resolveCommitAndParent(w http.ResponseWriter, repo *gitcore.Repository, commitHash gitcore.Hash) (*gitcore.Commit, gitcore.Hash, bool) {
	commits := repo.Commits()
	commit, exists := commits[commitHash]
	if !exists {
		http.Error(w, fmt.Sprintf("Commit not found: %s", commitHash), http.StatusNotFound)
		return nil, "", false
	}

	var parentTreeHash gitcore.Hash
	if len(commit.Parents) > 0 {
		parentCommit, exists := commits[commit.Parents[0]]
		if !exists {
			http.Error(w, fmt.Sprintf("Parent commit not found: %s", commit.Parents[0]), http.StatusNotFound)
			return nil, "", false
		}
		parentTreeHash = parentCommit.Tree
	}
	return commit, parentTreeHash, true
}

// allDeletions reports whether every diff line across all hunks is a deletion.
func allDeletions(hunks []gitcore.DiffHunk) bool {
	for _, hunk := range hunks {
		for _, line := range hunk.Lines {
			if line.Type != "deletion" {
				return false
			}
		}
	}
	return true
}
