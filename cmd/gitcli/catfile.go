package main

import (
	"fmt"
	"os"

	"github.com/rybkr/gitvista/internal/gitcore"
)

func runCatFile(repo *gitcore.Repository, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: gitvista-cli cat-file (-t|-s|-p) <object>")
		return 1
	}

	flag := args[0]
	rev := args[1]

	hash, err := resolveHash(repo, rev)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}

	switch flag {
	case "-t":
		return catFileType(repo, hash)
	case "-s":
		return catFileSize(repo, hash)
	case "-p":
		return catFilePretty(repo, hash)
	default:
		fmt.Fprintf(os.Stderr, "error: unknown flag: %q\n", flag)
		return 1
	}
}

func catFileType(repo *gitcore.Repository, hash gitcore.Hash) int {
	typeName, _, err := repo.GetObjectInfo(hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}
	fmt.Println(typeName)
	return 0
}

func catFileSize(repo *gitcore.Repository, hash gitcore.Hash) int {
	_, size, err := repo.GetObjectInfo(hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}
	fmt.Println(size)
	return 0
}

func catFilePretty(repo *gitcore.Repository, hash gitcore.Hash) int {
	typeName, _, err := repo.GetObjectInfo(hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}

	switch typeName {
	case "commit":
		return prettyPrintCommit(repo, hash)
	case "tree":
		return prettyPrintTree(repo, hash)
	case "blob":
		return prettyPrintBlob(repo, hash)
	case "tag":
		return prettyPrintTag(repo, hash)
	default:
		fmt.Fprintf(os.Stderr, "fatal: unknown object type: %q\n", typeName)
		return 128
	}
}

func prettyPrintCommit(repo *gitcore.Repository, hash gitcore.Hash) int {
	commit, err := repo.GetCommit(hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}

	fmt.Printf("tree %s\n", commit.Tree)
	for _, p := range commit.Parents {
		fmt.Printf("parent %s\n", p)
	}
	fmt.Printf("author %s <%s> %d %s\n",
		commit.Author.Name, commit.Author.Email,
		commit.Author.When.Unix(), commit.Author.When.Format("-0700"))
	fmt.Printf("committer %s <%s> %d %s\n",
		commit.Committer.Name, commit.Committer.Email,
		commit.Committer.When.Unix(), commit.Committer.When.Format("-0700"))
	fmt.Println()
	fmt.Println(commit.Message)
	return 0
}

func prettyPrintTree(repo *gitcore.Repository, hash gitcore.Hash) int {
	tree, err := repo.GetTree(hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}

	for _, entry := range tree.Entries {
		fmt.Printf("%s %s %s\t%s\n", normalizeMode(entry.Mode), entry.Type, entry.ID, entry.Name)
	}
	return 0
}

func prettyPrintBlob(repo *gitcore.Repository, hash gitcore.Hash) int {
	data, err := repo.GetBlob(hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}
	_, _ = os.Stdout.Write(data)
	return 0
}

func prettyPrintTag(repo *gitcore.Repository, hash gitcore.Hash) int {
	tag, err := repo.GetTag(hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		return 128
	}

	fmt.Printf("object %s\n", tag.Object)
	fmt.Printf("type %s\n", tag.ObjType)
	fmt.Printf("tag %s\n", tag.Name)
	fmt.Printf("tagger %s <%s> %d %s\n",
		tag.Tagger.Name, tag.Tagger.Email,
		tag.Tagger.When.Unix(), tag.Tagger.When.Format("-0700"))
	fmt.Println()
	fmt.Println(tag.Message)
	return 0
}
