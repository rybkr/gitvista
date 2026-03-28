package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/rybkr/gitvista/gitcore"
)

type diffStatEntry struct {
	FilesChanged int      `json:"filesChanged"`
	Files        []string `json:"files"`
}

type bulkDiffStatsResponse struct {
	Entries         map[string]diffStatEntry `json:"entries"`
	AnalyzedCommits int                      `json:"analyzedCommits"`
	TotalCommits    int                      `json:"totalCommits"`
	Limit           int                      `json:"limit"`
	Complete        bool                     `json:"complete"`
	SkippedTooLarge int                      `json:"skippedTooLarge"`
	SkippedOther    int                      `json:"skippedOther"`
}

func (s *Server) handleCommitDiffList(w http.ResponseWriter, repo *gitcore.Repository, commitHash gitcore.Hash, session *RepoSession) {
	cacheKey := string(commitHash)
	if cached, ok := session.diffCache.Get(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(cached); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	commit, parentTreeHash, ok := resolveCommitAndParent(w, repo, commitHash)
	if !ok {
		return
	}

	entries, err := gitcore.TreeDiff(repo, parentTreeHash, commit.Tree, "")
	if err != nil {
		s.logger.Error("Failed to compute diff", "commitHash", commitHash, "err", err)
		http.Error(w, "Diff computation failed", http.StatusInternalServerError)
		return
	}

	jsonEntries := make([]commitDiffEntryResponse, len(entries))
	stats := commitDiffStatsResponse{FilesChanged: len(entries)}
	for i, entry := range entries {
		jsonEntries[i] = commitDiffEntryResponse{
			Path:    entry.Path,
			OldPath: entry.OldPath,
			Status:  entry.Status.String(),
			OldHash: string(entry.OldHash),
			NewHash: string(entry.NewHash),
			Binary:  entry.IsBinary,
		}
		switch entry.Status {
		case gitcore.DiffStatusAdded:
			stats.Added++
		case gitcore.DiffStatusModified:
			stats.Modified++
		case gitcore.DiffStatusDeleted:
			stats.Deleted++
		case gitcore.DiffStatusRenamed:
			stats.Renamed++
		}
	}

	response := commitDiffResponse{
		CommitHash:     string(commitHash),
		ParentTreeHash: string(parentTreeHash),
		Entries:        jsonEntries,
		Stats:          stats,
	}

	session.diffCache.Put(cacheKey, response)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) handleBulkDiffStats(w http.ResponseWriter, r *http.Request) {
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

	limit := parseBulkDiffStatsLimit(r)
	cacheKey := "bulk-diffstats:v2:limit:" + strconv.Itoa(limit)
	if cached, ok := session.diffCache.Get(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(cached); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	commits := repo.Commits()
	totalCommits := len(commits)

	sortedCommits := make([]*gitcore.Commit, 0, len(commits))
	for _, commit := range commits {
		if commit != nil {
			sortedCommits = append(sortedCommits, commit)
		}
	}
	slices.SortFunc(sortedCommits, func(a, b *gitcore.Commit) int {
		if a.Committer.When.Equal(b.Committer.When) {
			return strings.Compare(string(a.ID), string(b.ID))
		}
		if a.Committer.When.After(b.Committer.When) {
			return -1
		}
		return 1
	})
	if limit > 0 && len(sortedCommits) > limit {
		sortedCommits = sortedCommits[:limit]
	}

	var mu sync.Mutex
	result := make(map[string]diffStatEntry, len(sortedCommits))
	tooLargeErrors := 0
	otherErrors := 0

	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup

	for _, commit := range sortedCommits {
		wg.Add(1)
		sem <- struct{}{}
		go func(c *gitcore.Commit) {
			defer wg.Done()
			defer func() { <-sem }()

			var parentTreeHash gitcore.Hash
			if len(c.Parents) > 0 {
				if parentCommit, exists := commits[c.Parents[0]]; exists {
					parentTreeHash = parentCommit.Tree
				}
			}

			entries, err := gitcore.TreeDiff(repo, parentTreeHash, c.Tree, "")
			if err != nil {
				mu.Lock()
				if strings.Contains(err.Error(), "diff too large") {
					tooLargeErrors++
				} else {
					otherErrors++
				}
				mu.Unlock()
				return
			}

			files := make([]string, len(entries))
			for i, entry := range entries {
				files[i] = entry.Path
			}

			mu.Lock()
			result[string(c.ID)] = diffStatEntry{
				FilesChanged: len(entries),
				Files:        files,
			}
			mu.Unlock()
		}(commit)
	}
	wg.Wait()

	if tooLargeErrors > 0 || otherErrors > 0 {
		s.logger.Warn("Bulk diff stats: skipped commits due to diff errors",
			"tooLarge", tooLargeErrors,
			"other", otherErrors,
			"analyzed", len(sortedCommits),
		)
	}

	response := bulkDiffStatsResponse{
		Entries:         result,
		AnalyzedCommits: len(sortedCommits),
		TotalCommits:    totalCommits,
		Limit:           limit,
		Complete:        len(sortedCommits) >= totalCommits && tooLargeErrors == 0 && otherErrors == 0,
		SkippedTooLarge: tooLargeErrors,
		SkippedOther:    otherErrors,
	}
	session.diffCache.Put(cacheKey, response)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func parseBulkDiffStatsLimit(r *http.Request) int {
	const (
		defaultLimit = 3000
		maxLimit     = 20000
	)
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultLimit
	}
	if n > maxLimit {
		return maxLimit
	}
	return n
}

func (s *Server) handleFileDiff(w http.ResponseWriter, r *http.Request, repo *gitcore.Repository, commitHash gitcore.Hash, session *RepoSession) {
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

	contextLines := parseDiffContextLines(r)

	cacheKey := string(commitHash) + ":" + filePath + ":ctx" + strconv.Itoa(contextLines)
	if cached, ok := session.diffCache.Get(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(cached); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	commit, parentTreeHash, ok := resolveCommitAndParent(w, repo, commitHash)
	if !ok {
		return
	}

	entries, err := gitcore.TreeDiff(repo, parentTreeHash, commit.Tree, "")
	if err != nil {
		s.logger.Error("Failed to compute diff for file diff", "commitHash", commitHash, "err", err)
		http.Error(w, "File diff computation failed", http.StatusInternalServerError)
		return
	}

	var targetEntry *gitcore.DiffEntry
	for i := range entries {
		if entries[i].Path == filePath {
			targetEntry = &entries[i]
			break
		}
	}

	if targetEntry == nil {
		s.logger.Error("File not found in commit diff", "commitHash", commitHash, "path", filePath)
		http.Error(w, "File diff computation failed", http.StatusNotFound)
		return
	}

	fileDiff, err := gitcore.ComputeFileDiff(repo, targetEntry.OldHash, targetEntry.NewHash, filePath, contextLines)
	if err != nil {
		s.logger.Error("Failed to compute file diff", "commitHash", commitHash, "path", filePath, "err", err)
		http.Error(w, "File diff computation failed", http.StatusInternalServerError)
		return
	}

	response := diffFileResponse{
		Path:      fileDiff.Path,
		Status:    targetEntry.Status.String(),
		OldHash:   string(fileDiff.OldHash),
		NewHash:   string(fileDiff.NewHash),
		IsBinary:  fileDiff.IsBinary,
		Truncated: fileDiff.Truncated,
		Hunks:     fileDiff.Hunks,
	}

	session.diffCache.Put(cacheKey, response)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) handleIndexDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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

	wts, err := gitcore.ComputeWorkingTreeStatus(repo)
	if err != nil {
		s.logger.Error("Failed to compute index diff status", "path", filePath, "err", err)
		http.Error(w, "Index diff failed", http.StatusInternalServerError)
		return
	}

	var fileStatus *gitcore.FileState
	for i := range wts.Files {
		if wts.Files[i].Path == filePath && wts.Files[i].StagedChange != 0 {
			fileStatus = &wts.Files[i]
			break
		}
	}
	if fileStatus == nil {
		http.Error(w, "No staged diff for path", http.StatusNotFound)
		return
	}

	contextLines := parseDiffContextLines(r)
	fileDiff, err := gitcore.ComputeFileDiff(repo, fileStatus.HeadHash, fileStatus.StagedHash, filePath, contextLines)
	if err != nil {
		s.logger.Error("Failed to compute index diff", "path", filePath, "err", err)
		http.Error(w, "Index diff failed", http.StatusInternalServerError)
		return
	}

	response := diffFileResponse{
		Path:      fileDiff.Path,
		Status:    fileStatus.StagedChange.String(),
		OldHash:   string(fileDiff.OldHash),
		NewHash:   string(fileDiff.NewHash),
		IsBinary:  fileDiff.IsBinary,
		Truncated: fileDiff.Truncated,
		Hunks:     fileDiff.Hunks,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) handleWorkingTreeDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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

	contextLines := parseDiffContextLines(r)
	fileDiff, err := gitcore.ComputeWorkingTreeFileDiff(repo, filePath, contextLines)
	if err != nil {
		s.logger.Error("Failed to compute working-tree diff", "path", filePath, "err", err)
		http.Error(w, "Working tree diff failed", http.StatusInternalServerError)
		return
	}

	status := gitcore.ChangeTypeModified.String()
	if fileDiff.OldHash == "" {
		status = gitcore.ChangeTypeAdded.String()
	} else if allDeletions(fileDiff.Hunks) && len(fileDiff.Hunks) > 0 {
		status = gitcore.ChangeTypeDeleted.String()
	}

	response := diffFileResponse{
		Path:      fileDiff.Path,
		Status:    status,
		OldHash:   string(fileDiff.OldHash),
		NewHash:   string(fileDiff.NewHash),
		IsBinary:  fileDiff.IsBinary,
		Truncated: fileDiff.Truncated,
		Hunks:     fileDiff.Hunks,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func parseDiffContextLines(r *http.Request) int {
	contextLines := gitcore.DefaultContextLines
	if raw := r.URL.Query().Get("context"); raw != "" {
		if n, parseErr := strconv.Atoi(raw); parseErr == nil && n > 0 && n <= 100 {
			contextLines = n
		}
	}
	return contextLines
}
