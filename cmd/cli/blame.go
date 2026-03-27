package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rybkr/gitvista/gitcore"
)

type blameOptions struct {
	path string
}

func runBlame(repoCtx *repositoryContext, args []string) int {
	opts, exitCode, err := parseBlameArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitCode
	}

	hash, err := repoCtx.repo.ResolveRevision("HEAD")
	if err != nil {
		fmt.Fprintf(os.Stderr, "gitvista-cli blame: %v\n", err)
		return 128
	}

	records, err := collectBlameRecords(repoCtx.repo, hash, opts.path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gitvista-cli blame: %v\n", err)
		return 128
	}

	commits := repoCtx.repo.Commits()
	for _, record := range records {
		commit := commits[record.entry.CommitHash]
		if commit == nil {
			fmt.Fprintf(os.Stderr, "gitvista-cli blame: commit not found: %s\n", record.entry.CommitHash)
			return 128
		}
		printBlamePorcelain(commit, record.path)
	}

	return 0
}

func parseBlameArgs(args []string) (blameOptions, int, error) {
	switch len(args) {
	case 1:
		if args[0] == "" {
			return blameOptions{}, 1, fmt.Errorf("gitvista-cli blame: missing path")
		}
		return blameOptions{path: args[0]}, 0, nil
	case 2:
		if args[0] != "-p" {
			return blameOptions{}, 1, fmt.Errorf("gitvista-cli blame: unsupported argument %q", args[0])
		}
		if args[1] == "" {
			return blameOptions{}, 1, fmt.Errorf("gitvista-cli blame: missing path")
		}
		return blameOptions{path: args[1]}, 0, nil
	default:
		return blameOptions{}, 1, fmt.Errorf("usage: gitvista-cli blame [-p] <path>")
	}
}

type blameRecord struct {
	path  string
	entry *gitcore.BlameEntry
}

func collectBlameRecords(repo *gitcore.Repository, revision gitcore.Hash, targetPath string) ([]blameRecord, error) {
	normalizedPath, err := normalizeBlamePath(targetPath)
	if err != nil {
		return nil, err
	}

	commits := repo.Commits()
	commit, ok := commits[revision]
	if !ok {
		return nil, fmt.Errorf("commit not found: %s", revision)
	}

	entry, err := resolveCommitEntryAtPath(repo, commit.Tree, normalizedPath)
	if err == nil {
		if !isDirectoryEntry(entry) {
			dirPath, fileName := splitBlameTargetPath(normalizedPath)
			blame, blameErr := repo.GetFileBlame(revision, dirPath)
			if blameErr != nil {
				return nil, blameErr
			}
			blameEntry := blame[fileName]
			if blameEntry == nil {
				return nil, fmt.Errorf("path %q not found", normalizedPath)
			}
			return []blameRecord{{
				path:  normalizedPath,
				entry: blameEntry,
			}}, nil
		}
	}

	blame, blameErr := repo.GetFileBlame(revision, normalizedPath)
	if blameErr != nil {
		if err != nil {
			return nil, fmt.Errorf("resolve path %q: %w", normalizedPath, err)
		}
		return nil, blameErr
	}

	names := make([]string, 0, len(blame))
	for name := range blame {
		names = append(names, name)
	}
	sort.Strings(names)

	records := make([]blameRecord, 0, len(names))
	for _, name := range names {
		records = append(records, blameRecord{
			path:  joinBlamePath(normalizedPath, name),
			entry: blame[name],
		})
	}
	return records, nil
}

func printBlamePorcelain(commit *gitcore.Commit, filename string) {
	fmt.Fprintf(os.Stdout, "%s 1 1 1\n", commit.ID)
	fmt.Fprintf(os.Stdout, "author %s\n", commit.Author.Name)
	fmt.Fprintf(os.Stdout, "author-mail <%s>\n", commit.Author.Email)
	fmt.Fprintf(os.Stdout, "author-time %d\n", commit.Author.When.Unix())
	fmt.Fprintf(os.Stdout, "author-tz %s\n", commit.Author.When.Format("-0700"))
	fmt.Fprintf(os.Stdout, "summary %s\n", strings.TrimSpace(blameSummary(commit.Message)))
	fmt.Fprintf(os.Stdout, "filename %s\n", filename)
}

func normalizeBlamePath(targetPath string) (string, error) {
	if targetPath == "" {
		return "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(targetPath) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}

	normalizedPath := path.Clean(filepath.ToSlash(targetPath))
	if normalizedPath == "." || normalizedPath == "" {
		return "", fmt.Errorf("empty path")
	}
	if normalizedPath == ".." || strings.HasPrefix(normalizedPath, "../") {
		return "", fmt.Errorf("path escapes repository root")
	}
	return normalizedPath, nil
}

func resolveCommitEntryAtPath(repo *gitcore.Repository, rootTreeHash gitcore.Hash, targetPath string) (gitcore.TreeEntry, error) {
	components := strings.Split(targetPath, "/")
	currentTreeHash := rootTreeHash

	for _, component := range components[:len(components)-1] {
		tree, err := repo.GetTree(currentTreeHash)
		if err != nil {
			return gitcore.TreeEntry{}, fmt.Errorf("failed to read tree %s: %w", currentTreeHash, err)
		}

		found := false
		for _, entry := range tree.Entries {
			if entry.Name == component {
				if !isDirectoryEntry(entry) {
					return gitcore.TreeEntry{}, fmt.Errorf("path component %q is not a directory", component)
				}
				currentTreeHash = entry.ID
				found = true
				break
			}
		}
		if !found {
			return gitcore.TreeEntry{}, fmt.Errorf("path component %q not found", component)
		}
	}

	tree, err := repo.GetTree(currentTreeHash)
	if err != nil {
		return gitcore.TreeEntry{}, fmt.Errorf("failed to read tree %s: %w", currentTreeHash, err)
	}

	leafName := components[len(components)-1]
	for _, entry := range tree.Entries {
		if entry.Name == leafName {
			return entry, nil
		}
	}

	return gitcore.TreeEntry{}, fmt.Errorf("path component %q not found", leafName)
}

func isDirectoryEntry(entry gitcore.TreeEntry) bool {
	return entry.Mode == "040000" || entry.Mode == "40000" || entry.Type == gitcore.ObjectTypeTree
}

func splitBlameTargetPath(targetPath string) (string, string) {
	dirPath, fileName := path.Split(targetPath)
	return strings.TrimSuffix(dirPath, "/"), fileName
}

func joinBlamePath(basePath, name string) string {
	if basePath == "" {
		return name
	}
	return basePath + "/" + name
}

func blameSummary(message string) string {
	for i, c := range message {
		if c == '\n' {
			return message[:i]
		}
	}
	return message
}
