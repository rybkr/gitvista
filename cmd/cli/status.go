package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/cli"
)

type statusOptions struct {
	short bool
}

func runStatus(repoCtx *repositoryContext, args []string, cw *cli.Writer) int {
	opts, exitCode, err := parseStatusArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitCode
	}

	repo := repoCtx.repo
	if repo.IsBare() {
		fmt.Fprintln(os.Stderr, "gitvista-cli status: bare repositories do not have a working tree")
		return 128
	}

	status, err := gitcore.ComputeWorkingTreeStatus(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gitvista-cli status: %v\n", err)
		return 128
	}

	if opts.short {
		printShortStatus(status)
		return 0
	}

	printLongStatus(repo, status, cw)
	return 0
}

func parseStatusArgs(args []string) (statusOptions, int, error) {
	var opts statusOptions
	for _, arg := range args {
		switch arg {
		case "--short":
			opts.short = true
		default:
			return statusOptions{}, 1, fmt.Errorf("gitvista-cli status: unsupported argument %q", arg)
		}
	}
	return opts, 0, nil
}

func printShortStatus(status *gitcore.WorkingTreeStatus) {
	for _, file := range status.Files {
		fmt.Printf("%s%s %s\n", shortIndexStatus(file), shortWorktreeStatus(file), quoteStatusPath(file.Path))
	}
}

func printLongStatus(repo *gitcore.Repository, status *gitcore.WorkingTreeStatus, cw *cli.Writer) {
	fmt.Println(formatStatusHeader(repo, cw))

	staged := collectStatusEntries(status.Files, func(file gitcore.FileStatus) (string, bool) {
		label := longIndexStatusLabel(file)
		return label, label != ""
	})
	modified := collectStatusEntries(status.Files, func(file gitcore.FileStatus) (string, bool) {
		label := longWorktreeStatusLabel(file)
		return label, label != ""
	})
	untracked := collectStatusEntries(status.Files, func(file gitcore.FileStatus) (string, bool) {
		if file.IsUntracked {
			return "", true
		}
		return "", false
	})

	if len(staged) == 0 && len(modified) == 0 && len(untracked) == 0 {
		fmt.Println("nothing to commit, working tree clean")
		return
	}

	if len(staged) > 0 {
		fmt.Println()
		fmt.Println(cw.Command("Changes to be committed:"))
		for _, entry := range staged {
			fmt.Printf("  %-10s %s\n", entry.label+":", entry.path)
		}
	}

	if len(modified) > 0 {
		fmt.Println()
		fmt.Println(cw.Command("Changes not staged for commit:"))
		for _, entry := range modified {
			fmt.Printf("  %-10s %s\n", entry.label+":", entry.path)
		}
	}

	if len(untracked) > 0 {
		fmt.Println()
		fmt.Println(cw.Command("Untracked files:"))
		for _, entry := range untracked {
			fmt.Printf("  %s\n", entry.path)
		}
	}
}

func formatStatusHeader(repo *gitcore.Repository, cw *cli.Writer) string {
	headRef := repo.HeadRef()
	if headRef != "" {
		return fmt.Sprintf("%s %s", cw.Cyan("On branch"), strings.TrimPrefix(headRef, "refs/heads/"))
	}
	if repo.HeadDetached() && repo.Head() != "" {
		return fmt.Sprintf("%s %s", cw.Cyan("HEAD detached at"), repo.Head().Short())
	}
	return cw.Cyan("No commits yet")
}

type statusEntry struct {
	label string
	path  string
}

func collectStatusEntries(files []gitcore.FileStatus, mapFn func(file gitcore.FileStatus) (string, bool)) []statusEntry {
	entries := make([]statusEntry, 0, len(files))
	for _, file := range files {
		label, ok := mapFn(file)
		if !ok {
			continue
		}
		entries = append(entries, statusEntry{label: label, path: file.Path})
	}
	return entries
}

func shortIndexStatus(file gitcore.FileStatus) string {
	if file.IsUntracked {
		return "?"
	}
	switch file.IndexStatus {
	case gitcore.StatusAdded:
		return "A"
	case gitcore.StatusModified:
		return "M"
	case gitcore.StatusDeleted:
		return "D"
	case gitcore.StatusRenamed:
		return "R"
	case gitcore.StatusCopied:
		return "C"
	case gitcore.StatusTypeChanged:
		return "T"
	default:
		return " "
	}
}

func shortWorktreeStatus(file gitcore.FileStatus) string {
	if file.IsUntracked {
		return "?"
	}
	switch file.WorkStatus {
	case gitcore.StatusModified:
		return "M"
	case gitcore.StatusDeleted:
		return "D"
	case gitcore.StatusTypeChanged:
		return "T"
	default:
		return " "
	}
}

func quoteStatusPath(path string) string {
	if strings.ContainsAny(path, " \t\n\"\\") {
		return strconv.Quote(path)
	}
	return path
}

func longIndexStatusLabel(file gitcore.FileStatus) string {
	switch file.IndexStatus {
	case gitcore.StatusAdded:
		return "new file"
	case gitcore.StatusModified:
		return "modified"
	case gitcore.StatusDeleted:
		return "deleted"
	case gitcore.StatusRenamed:
		return "renamed"
	case gitcore.StatusCopied:
		return "copied"
	default:
		return ""
	}
}

func longWorktreeStatusLabel(file gitcore.FileStatus) string {
	switch file.WorkStatus {
	case gitcore.StatusModified:
		return "modified"
	case gitcore.StatusDeleted:
		return "deleted"
	default:
		return ""
	}
}
