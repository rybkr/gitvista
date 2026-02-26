package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rybkr/gitvista/internal/gitcore"
	"github.com/rybkr/gitvista/internal/termcolor"
)

func runShow(repo *gitcore.Repository, args []string, cw *termcolor.Writer) int {
	stat := false
	rev := "HEAD"

	for _, arg := range args {
		if arg == "--stat" {
			stat = true
		} else {
			rev = arg
		}
	}

	hash, err := resolveHash(repo, rev)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}

	commit, err := repo.GetCommit(hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}

	// Header (same format as runLog full output)
	branches := repo.Branches()
	tags := repo.Tags()
	headRef := repo.HeadRef()
	decorations := buildDecorations(repo, branches, tags, headRef, cw)

	decor := ""
	if d, ok := decorations[commit.ID]; ok {
		decor = " " + cw.Yellow("(") + d + cw.Yellow(")")
	}

	fmt.Printf("%s %s%s\n", cw.Yellow("commit"), cw.Yellow(string(commit.ID)), decor)
	if len(commit.Parents) > 1 {
		parentStrs := make([]string, len(commit.Parents))
		for j, p := range commit.Parents {
			parentStrs[j] = p.Short()
		}
		fmt.Printf("Merge: %s\n", strings.Join(parentStrs, " "))
	}
	fmt.Printf("Author: %s <%s>\n", commit.Author.Name, commit.Author.Email)
	fmt.Printf("Date:   %s\n", gitDateFormat(commit.Author.When))
	fmt.Println()
	for _, line := range strings.Split(commit.Message, "\n") {
		fmt.Printf("    %s\n", line)
	}

	// Skip diff for merge commits (combined diff out of scope)
	if len(commit.Parents) > 1 {
		return 0
	}

	// Diff section
	var oldTreeHash gitcore.Hash
	if len(commit.Parents) == 1 {
		parent, parentErr := repo.GetCommit(commit.Parents[0])
		if parentErr != nil {
			fmt.Fprintf(os.Stderr, "fatal: %v\n", parentErr)
			return 128
		}
		oldTreeHash = parent.Tree
	}
	// Root commits: oldTreeHash stays "" — TreeDiff handles this

	entries, err := gitcore.TreeDiff(repo, oldTreeHash, commit.Tree, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}

	if stat {
		return printDiffStat(entries)
	}

	fmt.Println()
	return printUnifiedDiff(repo, entries, cw)
}
