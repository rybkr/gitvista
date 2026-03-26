package gitcore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	UpstreamStatusUpToDate    = "up_to_date"
	UpstreamStatusAhead       = "ahead"
	UpstreamStatusBehind      = "behind"
	UpstreamStatusDiverged    = "diverged"
	UpstreamStatusUnavailable = "unavailable"
)

const (
	UpstreamReasonDetachedHead    = "detached_head"
	UpstreamReasonNoCurrentBranch = "no_current_branch"
	UpstreamReasonNoUpstream      = "no_upstream_config"
	UpstreamReasonMissingRef      = "missing_remote_ref"
	UpstreamReasonNoCommonBase    = "no_common_ancestor"
)

// UpstreamTracking summarizes how the current local branch relates to its configured upstream.
type UpstreamTracking struct {
	Ref         string `json:"ref,omitempty"`
	BranchName  string `json:"branchName,omitempty"`
	Hash        Hash   `json:"hash,omitempty"`
	Status      string `json:"status"`
	AheadCount  int    `json:"aheadCount,omitempty"`
	BehindCount int    `json:"behindCount,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type branchTrackingConfig struct {
	Remote   string
	MergeRef string
}

// CurrentBranchUpstream computes tracking information for the currently checked out branch.
func (r *Repository) CurrentBranchUpstream() *UpstreamTracking {
	headRef := r.HeadRef()
	if r.HeadDetached() {
		return &UpstreamTracking{
			Status: UpstreamStatusUnavailable,
			Reason: UpstreamReasonDetachedHead,
		}
	}
	if headRef == "" {
		return &UpstreamTracking{
			Status: UpstreamStatusUnavailable,
			Reason: UpstreamReasonNoCurrentBranch,
		}
	}

	branchName, ok := strings.CutPrefix(headRef, "refs/heads/")
	if !ok || branchName == "" {
		return &UpstreamTracking{
			Status: UpstreamStatusUnavailable,
			Reason: UpstreamReasonNoCurrentBranch,
		}
	}

	content, err := os.ReadFile(filepath.Join(r.gitDir, "config"))
	if err != nil {
		return &UpstreamTracking{
			Status: UpstreamStatusUnavailable,
			Reason: UpstreamReasonNoUpstream,
		}
	}

	tracking := parseBranchTrackingFromConfig(string(content))
	cfg, found := tracking[branchName]
	if !found || cfg.Remote == "" || cfg.MergeRef == "" {
		return &UpstreamTracking{
			Status: UpstreamStatusUnavailable,
			Reason: UpstreamReasonNoUpstream,
		}
	}

	mergeShort, ok := strings.CutPrefix(cfg.MergeRef, "refs/heads/")
	if !ok || mergeShort == "" {
		return &UpstreamTracking{
			Status: UpstreamStatusUnavailable,
			Reason: UpstreamReasonNoUpstream,
		}
	}

	upstreamRef := fmt.Sprintf("refs/remotes/%s/%s", cfg.Remote, mergeShort)

	r.mu.RLock()
	upstreamHash, ok := r.refs[upstreamRef]
	headHash := r.head
	r.mu.RUnlock()

	info := &UpstreamTracking{
		Ref:        upstreamRef,
		BranchName: fmt.Sprintf("%s/%s", cfg.Remote, mergeShort),
	}
	if !ok || upstreamHash == "" {
		info.Status = UpstreamStatusUnavailable
		info.Reason = UpstreamReasonMissingRef
		return info
	}

	info.Hash = upstreamHash
	info.Status, info.AheadCount, info.BehindCount, info.Reason = r.classifyUpstreamRelation(headHash, upstreamHash)
	return info
}

func (r *Repository) classifyUpstreamRelation(localHash, upstreamHash Hash) (status string, ahead int, behind int, reason string) {
	if localHash == "" || upstreamHash == "" {
		return UpstreamStatusUnavailable, 0, 0, UpstreamReasonMissingRef
	}
	if localHash == upstreamHash {
		return UpstreamStatusUpToDate, 0, 0, ""
	}

	base, err := MergeBase(r, localHash, upstreamHash)
	if err != nil {
		return UpstreamStatusUnavailable, 0, 0, UpstreamReasonNoCommonBase
	}

	ahead = countExclusiveCommits(r, localHash, upstreamHash)
	behind = countExclusiveCommits(r, upstreamHash, localHash)

	if base == upstreamHash {
		return UpstreamStatusAhead, ahead, 0, ""
	}
	if base == localHash {
		return UpstreamStatusBehind, 0, behind, ""
	}

	return UpstreamStatusDiverged, ahead, behind, ""
}

func countExclusiveCommits(r *Repository, include, exclude Hash) int {
	if include == "" || exclude == "" {
		return 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	commits := r.commitMap
	excluded := make(map[Hash]struct{})
	stack := []Hash{exclude}

	for len(stack) > 0 {
		hash := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, seen := excluded[hash]; seen {
			continue
		}
		excluded[hash] = struct{}{}

		commit, ok := commits[hash]
		if !ok {
			continue
		}
		stack = append(stack, commit.Parents...)
	}

	visited := make(map[Hash]struct{})
	stack = []Hash{include}
	count := 0
	for len(stack) > 0 {
		hash := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, seen := visited[hash]; seen {
			continue
		}
		visited[hash] = struct{}{}
		if _, seen := excluded[hash]; seen {
			continue
		}

		commit, ok := commits[hash]
		if !ok {
			continue
		}
		count++
		for _, parent := range commit.Parents {
			stack = append(stack, parent)
		}
	}

	return count
}

func parseBranchTrackingFromConfig(config string) map[string]branchTrackingConfig {
	result := make(map[string]branchTrackingConfig)
	currentBranch := ""

	for _, raw := range strings.Split(config, "\n") {
		line := strings.TrimSpace(raw)

		if strings.HasPrefix(line, "[branch \"") && strings.HasSuffix(line, "\"]") {
			start := strings.Index(line, "\"") + 1
			end := strings.LastIndex(line, "\"")
			if start > 0 && end > start {
				currentBranch = line[start:end]
				if _, exists := result[currentBranch]; !exists {
					result[currentBranch] = branchTrackingConfig{}
				}
			}
			continue
		}

		if strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "[branch") {
			currentBranch = ""
			continue
		}
		if currentBranch == "" {
			continue
		}

		cfg := result[currentBranch]
		switch {
		case strings.HasPrefix(line, "remote = "):
			cfg.Remote = strings.TrimPrefix(line, "remote = ")
		case strings.HasPrefix(line, "merge = "):
			cfg.MergeRef = strings.TrimPrefix(line, "merge = ")
		}
		result[currentBranch] = cfg
	}

	return result
}
