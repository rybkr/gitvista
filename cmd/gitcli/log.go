package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rybkr/gitvista/internal/gitcore"
)

func runLog(repo *gitcore.Repository, args []string) int {
	maxCount := 0
	oneline := false

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--oneline":
			oneline = true
		case args[i] == "-n" && i+1 < len(args):
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: invalid -n value: %q\n", args[i]) //nolint:gosec // G705: CLI stderr, not web; %q quotes safely
				return 1
			}
			maxCount = n
		case strings.HasPrefix(args[i], "-n"):
			// Handle -n5 style
			n, err := strconv.Atoi(args[i][2:])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: invalid -n value: %q\n", args[i][2:]) //nolint:gosec // G705: CLI stderr, not web; %q quotes safely
				return 1
			}
			maxCount = n
		default:
			fmt.Fprintf(os.Stderr, "error: unknown option: %q\n", args[i]) //nolint:gosec // G705: CLI stderr, not web; %q quotes safely
			return 1
		}
	}

	commits := repo.CommitLog(maxCount)
	if len(commits) == 0 {
		return 0
	}

	// Build decoration maps
	branches := repo.Branches()
	tags := repo.Tags()
	headRef := repo.HeadRef()

	// Build commit hash -> decoration strings
	decorations := buildDecorations(repo, branches, tags, headRef)

	for i, c := range commits {
		decor := ""
		if d, ok := decorations[c.ID]; ok {
			decor = " (" + d + ")"
		}

		if oneline {
			fmt.Printf("%s%s %s\n", c.ID.Short(), decor, firstLine(c.Message))
		} else {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("commit %s%s\n", c.ID, decor)
			if len(c.Parents) > 1 {
				parentStrs := make([]string, len(c.Parents))
				for j, p := range c.Parents {
					parentStrs[j] = p.Short()
				}
				fmt.Printf("Merge: %s\n", strings.Join(parentStrs, " "))
			}
			fmt.Printf("Author: %s <%s>\n", c.Author.Name, c.Author.Email)
			fmt.Printf("Date:   %s\n", gitDateFormat(c.Author.When))
			fmt.Println()
			for _, line := range strings.Split(c.Message, "\n") {
				fmt.Printf("    %s\n", line)
			}
		}
	}

	return 0
}

func buildDecorations(repo *gitcore.Repository, branches map[string]gitcore.Hash, tags map[string]string, headRef string) map[gitcore.Hash]string {
	result := make(map[gitcore.Hash]string)

	// Determine the branch name HEAD points to
	headBranch := ""
	if headRef != "" {
		headBranch = strings.TrimPrefix(headRef, "refs/heads/")
	}

	// Group branch and tag names by commit hash
	type decoInfo struct {
		headArrow string
		branches  []string
		tags      []string
	}
	byHash := make(map[gitcore.Hash]*decoInfo)

	getInfo := func(h gitcore.Hash) *decoInfo {
		if info, ok := byHash[h]; ok {
			return info
		}
		info := &decoInfo{}
		byHash[h] = info
		return info
	}

	for name, hash := range branches {
		info := getInfo(hash)
		if name == headBranch {
			info.headArrow = "HEAD -> " + name
		} else {
			info.branches = append(info.branches, name)
		}
	}

	for name, hashStr := range tags {
		info := getInfo(gitcore.Hash(hashStr))
		info.tags = append(info.tags, "tag: "+name)
	}

	if headBranch == "" && repo.HeadDetached() {
		info := getInfo(repo.Head())
		info.headArrow = "HEAD"
	}

	for hash, info := range byHash {
		var parts []string
		if info.headArrow != "" {
			parts = append(parts, info.headArrow)
		}
		parts = append(parts, info.branches...)
		parts = append(parts, info.tags...)
		if len(parts) > 0 {
			result[hash] = strings.Join(parts, ", ")
		}
	}

	return result
}

func firstLine(msg string) string {
	if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
		return msg[:idx]
	}
	return msg
}
