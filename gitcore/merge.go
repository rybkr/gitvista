package gitcore

import "fmt"

// ConflictType classifies how a file is affected in a merge preview.
type ConflictType string

const (
	ConflictNone         ConflictType = "none"
	ConflictConflicting  ConflictType = "conflicting"
	ConflictBothAdded    ConflictType = "both_added"
	ConflictDeleteModify ConflictType = "delete_modify"
	ConflictRenameModify ConflictType = "rename_modify"
	ConflictRenameRename ConflictType = "rename_rename"
)

// MergePreviewEntry represents a single file in the merge preview.
type MergePreviewEntry struct {
	Path         string       `json:"path"`
	ConflictType ConflictType `json:"conflictType"`
	OursStatus   string       `json:"oursStatus"`
	TheirsStatus string       `json:"theirsStatus"`
	IsBinary     bool         `json:"isBinary"`
	BaseHash     Hash         `json:"baseHash,omitempty"`
	OursHash     Hash         `json:"oursHash,omitempty"`
	TheirsHash   Hash         `json:"theirsHash,omitempty"`
}

// MergePreviewStats summarizes the merge preview.
type MergePreviewStats struct {
	TotalFiles int `json:"totalFiles"`
	Conflicts  int `json:"conflicts"`
	CleanMerge int `json:"cleanMerge"`
}

// MergePreviewResult is the full result of a merge preview computation.
type MergePreviewResult struct {
	MergeBaseHash Hash                `json:"mergeBaseHash"`
	OursHash      Hash                `json:"oursHash"`
	TheirsHash    Hash                `json:"theirsHash"`
	Entries       []MergePreviewEntry `json:"entries"`
	Stats         MergePreviewStats   `json:"stats"`
}

// MergeRegionType classifies a region in a three-way diff.
type MergeRegionType string

const (
	MergeRegionContext  MergeRegionType = "context"
	MergeRegionOurs     MergeRegionType = "ours"
	MergeRegionTheirs   MergeRegionType = "theirs"
	MergeRegionConflict MergeRegionType = "conflict"
)

// MergeRegion represents a contiguous region in a three-way diff.
type MergeRegion struct {
	Type        MergeRegionType `json:"type"`
	BaseStart   int             `json:"baseStart"`
	BaseLines   []string        `json:"baseLines"`
	OursLines   []string        `json:"oursLines,omitempty"`
	TheirsLines []string        `json:"theirsLines,omitempty"`
}

// ThreeWayDiffStats summarizes the changes in a three-way diff.
type ThreeWayDiffStats struct {
	OursAdded       int `json:"oursAdded"`
	OursDeleted     int `json:"oursDeleted"`
	TheirsAdded     int `json:"theirsAdded"`
	TheirsDeleted   int `json:"theirsDeleted"`
	ConflictRegions int `json:"conflictRegions"`
}

// ThreeWayFileDiff represents a three-way merge diff for a single file.
type ThreeWayFileDiff struct {
	Path         string            `json:"path"`
	ConflictType ConflictType      `json:"conflictType"`
	IsBinary     bool              `json:"isBinary"`
	Truncated    bool              `json:"truncated"`
	Regions      []MergeRegion     `json:"regions"`
	Stats        ThreeWayDiffStats `json:"stats"`
}

// MergePreview computes a preview of merging theirs into ours without modifying the repository.
func MergePreview(repo *Repository, oursHash, theirsHash Hash) (*MergePreviewResult, error) {
	baseHash, err := MergeBase(repo, oursHash, theirsHash)
	if err != nil {
		return nil, err
	}

	oursCommit, err := repo.GetCommit(oursHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get ours commit: %w", err)
	}
	theirsCommit, err := repo.GetCommit(theirsHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get theirs commit: %w", err)
	}

	var baseTree Hash
	if baseHash != "" {
		baseCommit, baseErr := repo.GetCommit(baseHash)
		if baseErr != nil {
			return nil, fmt.Errorf("failed to get base commit: %w", baseErr)
		}
		baseTree = baseCommit.Tree
	}

	oursDiff, err := TreeDiff(repo, baseTree, oursCommit.Tree, "")
	if err != nil {
		return nil, fmt.Errorf("failed to diff ours against base: %w", err)
	}
	theirsDiff, err := TreeDiff(repo, baseTree, theirsCommit.Tree, "")
	if err != nil {
		return nil, fmt.Errorf("failed to diff theirs against base: %w", err)
	}

	oursMap := make(map[string]DiffEntry, len(oursDiff))
	for _, e := range oursDiff {
		oursMap[e.Path] = e
	}
	theirsMap := make(map[string]DiffEntry, len(theirsDiff))
	for _, e := range theirsDiff {
		theirsMap[e.Path] = e
	}

	entries := make([]MergePreviewEntry, 0, len(oursMap)+len(theirsMap))
	conflicts := 0

	oursOldPaths := make(map[string]DiffEntry)
	for _, e := range oursDiff {
		if e.Status == DiffStatusRenamed {
			oursOldPaths[e.OldPath] = e
		}
	}
	theirsOldPaths := make(map[string]DiffEntry)
	for _, e := range theirsDiff {
		if e.Status == DiffStatusRenamed {
			theirsOldPaths[e.OldPath] = e
		}
	}

	for oldPath, oursEntry := range oursOldPaths {
		theirsEntry, ok := theirsOldPaths[oldPath]
		if !ok {
			continue
		}
		if oursEntry.Path == theirsEntry.Path && oursEntry.NewHash == theirsEntry.NewHash {
			continue
		}
		entries = append(entries, MergePreviewEntry{
			Path:         oursEntry.Path,
			ConflictType: ConflictRenameRename,
			OursStatus:   oursEntry.Status.String(),
			TheirsStatus: theirsEntry.Status.String(),
			OursHash:     oursEntry.NewHash,
			TheirsHash:   theirsEntry.NewHash,
			BaseHash:     oursEntry.OldHash,
		})
		conflicts++
		delete(oursMap, oursEntry.Path)
		delete(theirsMap, theirsEntry.Path)
	}

	for oldPath, oursEntry := range oursOldPaths {
		if _, gone := oursMap[oursEntry.Path]; !gone {
			continue
		}
		theirsEntry, ok := theirsMap[oldPath]
		if !ok || theirsEntry.Status == DiffStatusRenamed {
			continue
		}
		entries = append(entries, MergePreviewEntry{
			Path:         oursEntry.Path,
			ConflictType: ConflictRenameModify,
			OursStatus:   oursEntry.Status.String(),
			TheirsStatus: theirsEntry.Status.String(),
			OursHash:     oursEntry.NewHash,
			TheirsHash:   theirsEntry.NewHash,
			BaseHash:     oursEntry.OldHash,
		})
		conflicts++
		delete(oursMap, oursEntry.Path)
		delete(theirsMap, oldPath)
	}

	for oldPath, theirsEntry := range theirsOldPaths {
		if _, gone := theirsMap[theirsEntry.Path]; !gone {
			continue
		}
		oursEntry, ok := oursMap[oldPath]
		if !ok || oursEntry.Status == DiffStatusRenamed {
			continue
		}
		entries = append(entries, MergePreviewEntry{
			Path:         theirsEntry.Path,
			ConflictType: ConflictRenameModify,
			OursStatus:   oursEntry.Status.String(),
			TheirsStatus: theirsEntry.Status.String(),
			OursHash:     oursEntry.NewHash,
			TheirsHash:   theirsEntry.NewHash,
			BaseHash:     theirsEntry.OldHash,
		})
		conflicts++
		delete(oursMap, oldPath)
		delete(theirsMap, theirsEntry.Path)
	}

	allPaths := make(map[string]struct{})
	for p := range oursMap {
		allPaths[p] = struct{}{}
	}
	for p := range theirsMap {
		allPaths[p] = struct{}{}
	}

	for path := range allPaths {
		oursEntry, inOurs := oursMap[path]
		theirsEntry, inTheirs := theirsMap[path]

		entry := MergePreviewEntry{
			Path:     path,
			IsBinary: (inOurs && oursEntry.IsBinary) || (inTheirs && theirsEntry.IsBinary),
		}

		if inOurs {
			entry.OursStatus = oursEntry.Status.String()
			entry.OursHash = oursEntry.NewHash
			entry.BaseHash = oursEntry.OldHash
		}
		if inTheirs {
			entry.TheirsStatus = theirsEntry.Status.String()
			entry.TheirsHash = theirsEntry.NewHash
			if entry.BaseHash == "" {
				entry.BaseHash = theirsEntry.OldHash
			}
		}

		switch {
		case inOurs && !inTheirs:
			entry.ConflictType = ConflictNone
		case !inOurs && inTheirs:
			entry.ConflictType = ConflictNone
		case inOurs && inTheirs:
			entry.ConflictType = classifyConflict(oursEntry, theirsEntry)
		}

		if entry.ConflictType != ConflictNone {
			conflicts++
		}
		entries = append(entries, entry)
	}

	return &MergePreviewResult{
		MergeBaseHash: baseHash,
		OursHash:      oursHash,
		TheirsHash:    theirsHash,
		Entries:       entries,
		Stats: MergePreviewStats{
			TotalFiles: len(entries),
			Conflicts:  conflicts,
			CleanMerge: len(entries) - conflicts,
		},
	}, nil
}

func classifyConflict(ours, theirs DiffEntry) ConflictType {
	if ours.NewHash != "" && ours.NewHash == theirs.NewHash {
		return ConflictNone
	}
	if ours.Status == DiffStatusAdded && theirs.Status == DiffStatusAdded {
		return ConflictBothAdded
	}
	if (ours.Status == DiffStatusDeleted && theirs.Status != DiffStatusDeleted) ||
		(ours.Status != DiffStatusDeleted && theirs.Status == DiffStatusDeleted) {
		return ConflictDeleteModify
	}
	if ours.Status == DiffStatusDeleted && theirs.Status == DiffStatusDeleted {
		return ConflictNone
	}
	return ConflictConflicting
}
