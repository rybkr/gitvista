package server

import (
	"math"
	"slices"
	"strings"
	"time"

	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/repositoryview"
)

func (rs *RepoSession) broadcastInitialBootstrap(
	delta *repositoryview.RepositoryDelta,
	status *WorkingTreeStatus,
	headInfo *HeadInfo,
) {
	batches := planInitialCommitBatches(delta)
	if len(batches) == 0 {
		rs.broadcastUpdate(UpdateMessage{Delta: delta, Status: status, Head: headInfo})
		return
	}

	rs.logger.Info("Streaming initial bootstrap deltas",
		"batches", len(batches),
		"commits", len(delta.AddedCommits),
	)

	for i, batch := range batches {
		if len(batch) == 0 {
			continue
		}

		sent := make(map[gitcore.Hash]struct{}, len(batch))
		for _, c := range batch {
			if c != nil {
				sent[c.ID] = struct{}{}
			}
		}

		batchDelta := repositoryview.NewRepositoryDelta()
		batchDelta.AddedCommits = batch
		batchDelta.AddedBranches = filterBranchesBySentHashes(delta.AddedBranches, sent)
		batchDelta.HeadHash = delta.HeadHash
		batchDelta.Bootstrap = true
		batchDelta.BootstrapComplete = i == len(batches)-1

		msg := UpdateMessage{Delta: batchDelta}
		if batchDelta.BootstrapComplete {
			batchDelta.Tags = delta.Tags
			batchDelta.Stashes = delta.Stashes
			msg.Status = status
			msg.Head = headInfo
		}
		rs.broadcastUpdate(msg)
		if !batchDelta.BootstrapComplete {
			time.Sleep(bootstrapBatchPause)
		}
	}
}

func planInitialCommitBatches(delta *repositoryview.RepositoryDelta) [][]*gitcore.Commit {
	if delta == nil || len(delta.AddedCommits) == 0 {
		return nil
	}

	ordered := slices.Clone(delta.AddedCommits)
	slices.SortFunc(ordered, func(a, b *gitcore.Commit) int {
		if a == nil || b == nil {
			if a == nil && b == nil {
				return 0
			}
			if a == nil {
				return 1
			}
			return -1
		}
		if a.Committer.When.Equal(b.Committer.When) {
			return strings.Compare(string(a.ID), string(b.ID))
		}
		if a.Committer.When.After(b.Committer.When) {
			return -1
		}
		return 1
	})

	commitByHash := make(map[gitcore.Hash]*gitcore.Commit, len(ordered))
	for _, c := range ordered {
		if c != nil {
			commitByHash[c.ID] = c
		}
	}

	priority := make(map[gitcore.Hash]struct{})
	for _, h := range collectRefTipHashes(delta) {
		if _, ok := commitByHash[h]; ok {
			priority[h] = struct{}{}
		}
	}

	targetHash := gitcore.Hash(delta.HeadHash)
	targetCommit, ok := commitByHash[targetHash]
	if !ok && len(ordered) > 0 {
		targetCommit = ordered[0]
		targetHash = targetCommit.ID
	}
	if targetHash != "" {
		priority[targetHash] = struct{}{}
	}

	if targetCommit != nil {
		targetSec := targetCommit.Committer.When.Unix()
		windowSec := int64(initialBootstrapWindowDays * 24 * 60 * 60)
		for _, c := range ordered {
			if c != nil && absInt64(c.Committer.When.Unix()-targetSec) <= windowSec {
				priority[c.ID] = struct{}{}
			}
		}
	}

	priorityOrdered := make([]*gitcore.Commit, 0, len(priority))
	remaining := make([]*gitcore.Commit, 0, len(ordered)-len(priority))
	lightweightRemaining := len(ordered) > forceModeMaxCommits
	for _, c := range ordered {
		if c == nil {
			continue
		}
		if _, ok := priority[c.ID]; ok {
			priorityOrdered = append(priorityOrdered, makeBootstrapCommit(c, false))
			continue
		}
		remaining = append(remaining, makeBootstrapCommit(c, lightweightRemaining))
	}

	batches := make([][]*gitcore.Commit, 0, int(math.Ceil(float64(len(ordered))/float64(bootstrapMaxCommitsPerBatch)))+1)
	appendBatches := func(commits []*gitcore.Commit, firstTarget int, defaultTarget int) {
		if len(commits) == 0 {
			return
		}
		target := firstTarget
		batch := make([]*gitcore.Commit, 0, bootstrapMaxCommitsPerBatch)
		estimated := 0
		for _, c := range commits {
			size := estimateBootstrapCommitSize(c)
			if len(batch) > 0 && (estimated+size > target || len(batch) >= bootstrapMaxCommitsPerBatch) {
				batches = append(batches, batch)
				batch = make([]*gitcore.Commit, 0, bootstrapMaxCommitsPerBatch)
				estimated = 0
				target = defaultTarget
			}
			batch = append(batch, c)
			estimated += size
		}
		if len(batch) > 0 {
			batches = append(batches, batch)
		}
	}

	appendBatches(priorityOrdered, bootstrapFirstBatchTarget, bootstrapBatchTarget)
	appendBatches(remaining, bootstrapBatchTarget, bootstrapBatchTarget)
	return batches
}

func makeBootstrapCommit(c *gitcore.Commit, lightweight bool) *gitcore.Commit {
	if c == nil {
		return nil
	}
	parents := append([]gitcore.Hash(nil), c.Parents...)
	if !lightweight {
		return &gitcore.Commit{
			ID:                c.ID,
			Tree:              c.Tree,
			Parents:           parents,
			Author:            c.Author,
			Committer:         c.Committer,
			Message:           c.Message,
			BranchLabel:       c.BranchLabel,
			BranchLabelSource: c.BranchLabelSource,
		}
	}
	return &gitcore.Commit{
		ID:      c.ID,
		Parents: parents,
		Author: gitcore.Signature{
			When: c.Author.When,
		},
		Committer: gitcore.Signature{
			When: c.Committer.When,
		},
		BranchLabel:       c.BranchLabel,
		BranchLabelSource: c.BranchLabelSource,
	}
}

func estimateBootstrapCommitSize(c *gitcore.Commit) int {
	if c == nil {
		return 0
	}
	size := 180
	size += len(c.Message)
	size += len(c.Author.Name) + len(c.Author.Email)
	size += len(c.Committer.Name) + len(c.Committer.Email)
	size += len(c.Parents) * 44
	return size
}

func collectRefTipHashes(delta *repositoryview.RepositoryDelta) []gitcore.Hash {
	if delta == nil {
		return nil
	}
	out := make([]gitcore.Hash, 0, len(delta.AddedBranches)+len(delta.Tags)+len(delta.Stashes)+1)
	if delta.HeadHash != "" {
		if h, err := gitcore.NewHash(delta.HeadHash); err == nil {
			out = append(out, h)
		}
	}
	for _, h := range delta.AddedBranches {
		out = append(out, h)
	}
	for _, h := range delta.Tags {
		if parsed, err := gitcore.NewHash(h); err == nil {
			out = append(out, parsed)
		}
	}
	for _, s := range delta.Stashes {
		if s != nil && s.Hash != "" {
			out = append(out, s.Hash)
		}
	}
	return out
}

func filterBranchesBySentHashes(branches map[string]gitcore.Hash, sent map[gitcore.Hash]struct{}) map[string]gitcore.Hash {
	filtered := make(map[string]gitcore.Hash)
	for name, hash := range branches {
		if _, ok := sent[hash]; ok {
			filtered[name] = hash
		}
	}
	return filtered
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func (rs *RepoSession) scheduleAnalyticsPrewarm(repo *gitcore.Repository) {
	if repo == nil {
		return
	}

	rs.analyticsMu.Lock()
	rs.analyticsGen++
	gen := rs.analyticsGen
	rs.analyticsMu.Unlock()

	rs.wg.Add(1)
	go func(repo *gitcore.Repository, gen uint64) {
		defer rs.wg.Done()
		periods := []string{"all", "3m", "6m", "1y"}
		for _, period := range periods {
			select {
			case <-rs.ctx.Done():
				return
			default:
			}
			if !rs.analyticsGenCurrent(gen) {
				return
			}

			key := analyticsCacheKey(repo, period)
			if _, ok := rs.diffCache.Get(key); ok {
				continue
			}

			analytics, err := buildAnalytics(repo, analyticsQuery{
				period:   period,
				cacheKey: period,
			})
			if err != nil {
				rs.logger.Warn("Analytics prewarm failed", "period", period, "err", err)
				continue
			}
			if !rs.analyticsGenCurrent(gen) {
				return
			}
			rs.diffCache.Put(key, analytics)
		}
	}(repo, gen)
}

func (rs *RepoSession) analyticsGenCurrent(gen uint64) bool {
	rs.analyticsMu.Lock()
	defer rs.analyticsMu.Unlock()
	return rs.analyticsGen == gen
}
