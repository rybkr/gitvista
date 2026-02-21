package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rybkr/gitvista/internal/gitcore"
)

func runDiff(repo *gitcore.Repository, args []string) int {
	stat := false
	var revs []string

	for _, arg := range args {
		if arg == "--stat" {
			stat = true
		} else {
			revs = append(revs, arg)
		}
	}

	if len(revs) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gitvista-cli diff [--stat] <commit1> <commit2>")
		return 1
	}

	hash1, err := resolveHash(repo, revs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}
	hash2, err := resolveHash(repo, revs[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}

	commit1, err := repo.GetCommit(hash1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}
	commit2, err := repo.GetCommit(hash2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}

	entries, err := gitcore.TreeDiff(repo, commit1.Tree, commit2.Tree, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}

	if stat {
		return printDiffStat(entries)
	}

	return printUnifiedDiff(repo, entries)
}

func printUnifiedDiff(repo *gitcore.Repository, entries []gitcore.DiffEntry) int {
	for _, entry := range entries {
		path := entry.Path
		oldPath := entry.OldPath
		if oldPath == "" {
			oldPath = path
		}

		fmt.Printf("diff --git a/%s b/%s\n", oldPath, path)

		// index line
		oldHash := entry.OldHash.Short()
		newHash := entry.NewHash.Short()
		if oldHash == "" {
			oldHash = "0000000"
		}
		if newHash == "" {
			newHash = "0000000"
		}

		switch entry.Status {
		case gitcore.DiffStatusAdded:
			fmt.Printf("new file mode %s\n", normalizeMode(entry.NewMode))
			fmt.Printf("index %s..%s\n", oldHash, newHash)
		case gitcore.DiffStatusDeleted:
			fmt.Printf("deleted file mode %s\n", normalizeMode(entry.OldMode))
			fmt.Printf("index %s..%s\n", oldHash, newHash)
		case gitcore.DiffStatusRenamed:
			fmt.Printf("similarity index 100%%\n")
			fmt.Printf("rename from %s\n", oldPath)
			fmt.Printf("rename to %s\n", path)
			continue // No content diff for exact renames
		default:
			fmt.Printf("index %s..%s\n", oldHash, newHash)
		}

		if entry.IsBinary {
			fmt.Printf("Binary files differ\n")
			continue
		}

		fileDiff, err := gitcore.ComputeFileDiff(repo, entry.OldHash, entry.NewHash, path, gitcore.DefaultContextLines)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			continue
		}

		if fileDiff.IsBinary {
			fmt.Printf("Binary files differ\n")
			continue
		}

		// --- / +++ headers
		if entry.Status == gitcore.DiffStatusAdded {
			fmt.Printf("--- /dev/null\n")
		} else {
			fmt.Printf("--- a/%s\n", oldPath)
		}
		if entry.Status == gitcore.DiffStatusDeleted {
			fmt.Printf("+++ /dev/null\n")
		} else {
			fmt.Printf("+++ b/%s\n", path)
		}

		for _, hunk := range fileDiff.Hunks {
			fmt.Printf("@@ -%d,%d +%d,%d @@\n", hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)
			for _, line := range hunk.Lines {
				switch line.Type {
				case gitcore.LineTypeContext:
					fmt.Printf(" %s\n", line.Content)
				case gitcore.LineTypeAddition:
					fmt.Printf("+%s\n", line.Content)
				case gitcore.LineTypeDeletion:
					fmt.Printf("-%s\n", line.Content)
				}
			}
		}
	}
	return 0
}

func printDiffStat(entries []gitcore.DiffEntry) int {
	if len(entries) == 0 {
		return 0
	}

	maxNameLen := 0
	for _, e := range entries {
		name := statDisplayName(e)
		if len(name) > maxNameLen {
			maxNameLen = len(name)
		}
	}

	for _, e := range entries {
		name := statDisplayName(e)

		if e.IsBinary {
			fmt.Printf(" %-*s | Bin\n", maxNameLen, name)
			continue
		}

		switch e.Status {
		case gitcore.DiffStatusAdded:
			fmt.Printf(" %-*s | (new)\n", maxNameLen, name)
		case gitcore.DiffStatusDeleted:
			fmt.Printf(" %-*s | (gone)\n", maxNameLen, name)
		case gitcore.DiffStatusRenamed:
			fmt.Printf(" %-*s | 0\n", maxNameLen, name)
		default:
			fmt.Printf(" %-*s | (modified)\n", maxNameLen, name)
		}
	}

	fmt.Printf(" %d file(s) changed\n", len(entries))

	return 0
}

func statDisplayName(e gitcore.DiffEntry) string {
	if e.Status == gitcore.DiffStatusRenamed {
		return e.OldPath + " => " + e.Path
	}
	return e.Path
}

func normalizeMode(mode string) string {
	if mode == "" {
		return "100644"
	}
	if len(mode) < 6 {
		return strings.Repeat("0", 6-len(mode)) + mode
	}
	return mode
}
