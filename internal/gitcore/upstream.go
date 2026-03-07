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

	configPath := filepath.Join(r.gitDir, "config")
	content, err := os.ReadFile(configPath)
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

	if base == upstreamHash {
		return UpstreamStatusAhead, countCommitsSince(r, localHash, base), 0, ""
	}
	if base == localHash {
		return UpstreamStatusBehind, 0, countCommitsSince(r, upstreamHash, base), ""
	}

	return UpstreamStatusDiverged,
		countCommitsSince(r, localHash, base),
		countCommitsSince(r, upstreamHash, base),
		""
}

func countCommitsSince(r *Repository, start, stop Hash) int {
	if start == "" || stop == "" || start == stop {
		return 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	commits := r.commitsMap()
	visited := map[Hash]struct{}{stop: {}}
	queue := []Hash{start}
	count := 0

	for len(queue) > 0 {
		hash := queue[0]
		queue = queue[1:]
		if _, seen := visited[hash]; seen {
			continue
		}
		visited[hash] = struct{}{}

		commit, ok := commits[hash]
		if !ok {
			continue
		}

		count++
		for _, parent := range commit.Parents {
			if _, seen := visited[parent]; !seen {
				queue = append(queue, parent)
			}
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
