package repositoryview

import (
	"sort"
	"strings"

	"github.com/rybkr/gitvista/gitcore"
)

const (
	branchLabelSourceHeadRef      = "head_ref"
	branchLabelSourceLocalRef     = "local_ref"
	branchLabelSourceRemoteRef    = "remote_ref"
	branchLabelSourceMergeMessage = "merge_message"
)

type branchAttribution struct {
	Label    string
	Source   string
	priority int
}

type branchSeed struct {
	Label    string
	Source   string
	Tip      gitcore.Hash
	priority int
}

func commitBranchAttribution(repo *gitcore.Repository) map[gitcore.Hash]branchAttribution {
	if repo == nil {
		return map[gitcore.Hash]branchAttribution{}
	}
	return buildCommitBranchAttribution(repo.Commits(), repo.GraphBranches(), repo.HeadRef())
}

func buildCommitBranchAttribution(
	commits map[gitcore.Hash]*gitcore.Commit,
	refs map[string]gitcore.Hash,
	headRef string,
) map[gitcore.Hash]branchAttribution {
	if len(commits) == 0 {
		return map[gitcore.Hash]branchAttribution{}
	}

	out := make(map[gitcore.Hash]branchAttribution)
	seeds := collectBranchSeeds(refs, headRef)

	for _, seed := range seeds {
		walk := seed.Tip
		for walk != "" {
			commit := commits[walk]
			if commit == nil {
				break
			}
			if existing, ok := out[walk]; ok && existing.priority <= seed.priority {
				break
			}
			out[walk] = branchAttribution{
				Label:    seed.Label,
				Source:   seed.Source,
				priority: seed.priority,
			}
			if len(commit.Parents) == 0 {
				break
			}
			walk = commit.Parents[0]
		}
	}

	mergeCommits := make([]*gitcore.Commit, 0)
	for _, commit := range commits {
		if commit != nil && len(commit.Parents) > 1 {
			mergeCommits = append(mergeCommits, commit)
		}
	}
	sort.Slice(mergeCommits, func(i, j int) bool {
		if mergeCommits[i].Committer.When.Equal(mergeCommits[j].Committer.When) {
			return mergeCommits[i].ID < mergeCommits[j].ID
		}
		return mergeCommits[i].Committer.When.After(mergeCommits[j].Committer.When)
	})

	for _, mergeCommit := range mergeCommits {
		label := normalizeBranchLabel(parseMergeBranchName(mergeCommit.Message))
		if label == "" {
			continue
		}

		mainline := make(map[gitcore.Hash]struct{})
		walk := mergeCommit.ID
		for walk != "" {
			mainline[walk] = struct{}{}
			commit := commits[walk]
			if commit == nil || len(commit.Parents) == 0 {
				break
			}
			walk = commit.Parents[0]
		}

		for _, parent := range mergeCommit.Parents[1:] {
			walk = parent
			for walk != "" {
				if _, ok := mainline[walk]; ok {
					break
				}
				commit := commits[walk]
				if commit == nil {
					break
				}
				if _, ok := out[walk]; ok {
					break
				}
				out[walk] = branchAttribution{
					Label:    label,
					Source:   branchLabelSourceMergeMessage,
					priority: 3,
				}
				if len(commit.Parents) == 0 {
					break
				}
				walk = commit.Parents[0]
			}
		}
	}

	return out
}

func collectBranchSeeds(refs map[string]gitcore.Hash, headRef string) []branchSeed {
	seeds := make([]branchSeed, 0, len(refs)+1)

	if headRef != "" {
		if hash, ok := refs[headRef]; ok {
			label := normalizeBranchLabel(headRef)
			if label != "" {
				seeds = append(seeds, branchSeed{
					Label:    label,
					Source:   branchLabelSourceHeadRef,
					Tip:      hash,
					priority: 0,
				})
			}
		}
	}

	for ref, hash := range refs {
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			label := normalizeBranchLabel(ref)
			if label != "" {
				seeds = append(seeds, branchSeed{
					Label:    label,
					Source:   branchLabelSourceLocalRef,
					Tip:      hash,
					priority: 1,
				})
			}
		case strings.HasPrefix(ref, "refs/remotes/") && !strings.HasSuffix(ref, "/HEAD"):
			label := normalizeBranchLabel(ref)
			if label != "" {
				seeds = append(seeds, branchSeed{
					Label:    label,
					Source:   branchLabelSourceRemoteRef,
					Tip:      hash,
					priority: 2,
				})
			}
		}
	}

	sort.Slice(seeds, func(i, j int) bool {
		if seeds[i].priority != seeds[j].priority {
			return seeds[i].priority < seeds[j].priority
		}
		if branchMainRank(seeds[i].Label) != branchMainRank(seeds[j].Label) {
			return branchMainRank(seeds[i].Label) < branchMainRank(seeds[j].Label)
		}
		if seeds[i].Label != seeds[j].Label {
			return seeds[i].Label < seeds[j].Label
		}
		return seeds[i].Tip < seeds[j].Tip
	})

	return seeds
}

func branchMainRank(label string) int {
	switch label {
	case "main", "master", "trunk":
		return 0
	default:
		return 1
	}
}

func normalizeBranchLabel(name string) string {
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "refs/heads/") {
		return strings.TrimPrefix(name, "refs/heads/")
	}
	if strings.HasPrefix(name, "refs/remotes/") {
		rest := strings.TrimPrefix(name, "refs/remotes/")
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			return rest[slash+1:]
		}
		return rest
	}
	return name
}

func parseMergeBranchName(message string) string {
	first := message
	if newline := strings.IndexByte(message, '\n'); newline >= 0 {
		first = message[:newline]
	}

	if strings.HasPrefix(first, "Merge remote-tracking branch '") {
		rest := strings.TrimPrefix(first, "Merge remote-tracking branch '")
		if end := strings.IndexByte(rest, '\''); end >= 0 {
			raw := rest[:end]
			if slash := strings.IndexByte(raw, '/'); slash >= 0 {
				return raw[slash+1:]
			}
			return raw
		}
	}

	if strings.HasPrefix(first, "Merge branch '") {
		rest := strings.TrimPrefix(first, "Merge branch '")
		if end := strings.IndexByte(rest, '\''); end >= 0 {
			return rest[:end]
		}
	}

	if strings.HasPrefix(first, "Merge pull request #") {
		if from := strings.Index(first, " from "); from >= 0 {
			rest := first[from+len(" from "):]
			if to := strings.Index(rest, " to "); to >= 0 {
				rest = rest[:to]
			}
			if slash := strings.IndexByte(rest, '/'); slash >= 0 && slash+1 < len(rest) {
				return strings.TrimSpace(rest[slash+1:])
			}
		}
		if from := strings.Index(first, " from "); from >= 0 {
			rest := first[from+len(" from "):]
			if slash := strings.IndexByte(rest, '/'); slash >= 0 && slash+1 < len(rest) {
				return strings.TrimSpace(rest[slash+1:])
			}
		}
	}

	if strings.HasPrefix(first, "Merged in ") {
		rest := strings.TrimPrefix(first, "Merged in ")
		if end := strings.Index(rest, " (pull request #"); end >= 0 {
			return strings.TrimSpace(rest[:end])
		}
	}

	if strings.HasPrefix(first, "Merge ") {
		rest := strings.TrimPrefix(first, "Merge ")
		if into := strings.Index(rest, " into "); into > 0 {
			return strings.TrimSpace(rest[:into])
		}
		if colon := strings.Index(rest, ":"); colon > 0 {
			return strings.TrimSpace(rest[:colon])
		}
		return strings.TrimSpace(rest)
	}

	return ""
}

func cloneCommitWithBranchAttribution(commit *gitcore.Commit, attribution branchAttribution) *gitcore.Commit {
	if commit == nil {
		return nil
	}

	cloned := *commit
	if commit.Parents != nil {
		cloned.Parents = append([]gitcore.Hash(nil), commit.Parents...)
	}
	cloned.BranchLabel = attribution.Label
	cloned.BranchLabelSource = attribution.Source
	return &cloned
}
