package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/rybkr/gitvista/internal/gitcore"
)

const (
	statusModified = "modified"
	statusDeleted  = "deleted"
)

func runStatus(repo *gitcore.Repository, args []string) int {
	porcelain := false
	for _, arg := range args {
		if arg == "-s" || arg == "--porcelain" {
			porcelain = true
		}
	}

	status, err := gitcore.ComputeWorkingTreeStatus(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}

	// Sort files for deterministic output
	sort.Slice(status.Files, func(i, j int) bool {
		return status.Files[i].Path < status.Files[j].Path
	})

	if porcelain {
		return printPorcelain(status)
	}
	return printLongStatus(repo, status)
}

func printPorcelain(status *gitcore.WorkingTreeStatus) int {
	for _, f := range status.Files {
		x, y := statusCodes(f)
		fmt.Printf("%c%c %s\n", x, y, f.Path)
	}
	return 0
}

func statusCodes(f gitcore.FileStatus) (x, y byte) {
	x = ' '
	y = ' '

	if f.IsUntracked {
		return '?', '?'
	}

	switch f.IndexStatus {
	case "added":
		x = 'A'
	case statusModified:
		x = 'M'
	case statusDeleted:
		x = 'D'
	}

	switch f.WorkStatus {
	case statusModified:
		y = 'M'
	case statusDeleted:
		y = 'D'
	}

	return x, y
}

func printLongStatus(repo *gitcore.Repository, status *gitcore.WorkingTreeStatus) int {
	headRef := repo.HeadRef()
	if headRef != "" {
		branch := headRef
		if idx := len("refs/heads/"); len(headRef) > idx {
			branch = headRef[idx:]
		}
		fmt.Printf("On branch %s\n", branch)
	} else {
		fmt.Printf("HEAD detached at %s\n", repo.Head().Short())
	}

	var staged, unstaged, untracked []gitcore.FileStatus
	for _, f := range status.Files {
		if f.IsUntracked {
			untracked = append(untracked, f)
			continue
		}
		if f.IndexStatus != "" {
			staged = append(staged, f)
		}
		if f.WorkStatus != "" {
			unstaged = append(unstaged, f)
		}
	}

	if len(staged) > 0 {
		fmt.Println("Changes to be committed:")
		fmt.Println("  (use \"git restore --staged <file>...\" to unstage)")
		for _, f := range staged {
			prefix := ""
			switch f.IndexStatus {
			case "added":
				prefix = "new file:   "
			case statusModified:
				prefix = "modified:   "
			case statusDeleted:
				prefix = "deleted:    "
			}
			fmt.Printf("\t%s%s\n", prefix, f.Path)
		}
		fmt.Println()
	}

	if len(unstaged) > 0 {
		fmt.Println("Changes not staged for commit:")
		fmt.Println("  (use \"git add <file>...\" to update what will be committed)")
		for _, f := range unstaged {
			prefix := ""
			switch f.WorkStatus {
			case statusModified:
				prefix = "modified:   "
			case statusDeleted:
				prefix = "deleted:    "
			}
			fmt.Printf("\t%s%s\n", prefix, f.Path)
		}
		fmt.Println()
	}

	if len(untracked) > 0 {
		fmt.Println("Untracked files:")
		fmt.Println("  (use \"git add <file>...\" to include in what will be committed)")
		for _, f := range untracked {
			fmt.Printf("\t%s\n", f.Path)
		}
		fmt.Println()
	}

	if len(staged) == 0 && len(unstaged) == 0 && len(untracked) == 0 {
		fmt.Println("nothing to commit, working tree clean")
	}

	return 0
}
