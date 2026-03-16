package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/rybkr/gitvista/gitcore"
)

type catFileMode int

const (
	catFileModeType catFileMode = iota
	catFileModeSize
	catFileModePretty
)

type catFileOptions struct {
	mode     catFileMode
	revision string
}

func runCatFile(repoCtx *repositoryContext, args []string) int {
	opts, exitCode, err := parseCatFileArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitCode
	}

	result, err := repoCtx.repo.CatFile(gitcore.CatFileOptions{Revision: opts.revision})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 128
	}

	switch opts.mode {
	case catFileModeType:
		fmt.Fprintln(os.Stdout, result.Type.String())
	case catFileModeSize:
		fmt.Fprintln(os.Stdout, result.Size)
	case catFileModePretty:
		data, err := formatCatFileOutput(result)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 128
		}
		if _, err := os.Stdout.Write(data); err != nil {
			fmt.Fprintf(os.Stderr, "gitvista-cli cat-file: write output: %v\n", err)
			return 1
		}
	}

	return 0
}

func parseCatFileArgs(args []string) (catFileOptions, int, error) {
	if len(args) != 2 {
		return catFileOptions{}, 1, fmt.Errorf("usage: gitvista-cli cat-file (-t | -s | -p) <object>")
	}

	opts := catFileOptions{revision: args[1]}
	switch args[0] {
	case "-t":
		opts.mode = catFileModeType
	case "-s":
		opts.mode = catFileModeSize
	case "-p":
		opts.mode = catFileModePretty
	default:
		return catFileOptions{}, 1, fmt.Errorf("gitvista-cli cat-file: unsupported argument %q", args[0])
	}

	if opts.revision == "" {
		return catFileOptions{}, 1, fmt.Errorf("gitvista-cli cat-file: missing object")
	}

	return opts, 0, nil
}

func formatCatFileOutput(result *gitcore.CatFileResult) ([]byte, error) {
	switch result.Type {
	case gitcore.ObjectTypeBlob, gitcore.ObjectTypeCommit, gitcore.ObjectTypeTag:
		return append([]byte(nil), result.Data...), nil
	case gitcore.ObjectTypeTree:
		return formatTreeObject(result.Data)
	default:
		return nil, fmt.Errorf("unsupported object type: %s", result.Type.String())
	}
}

func formatTreeObject(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	for len(data) > 0 {
		modeEnd := bytes.IndexByte(data, ' ')
		if modeEnd <= 0 {
			return nil, fmt.Errorf("failed to parse tree object: invalid mode")
		}
		mode := string(data[:modeEnd])
		data = data[modeEnd+1:]

		nameEnd := bytes.IndexByte(data, 0)
		if nameEnd < 0 {
			return nil, fmt.Errorf("failed to parse tree object: invalid name")
		}
		name := string(data[:nameEnd])
		data = data[nameEnd+1:]

		if len(data) < 20 {
			return nil, fmt.Errorf("failed to parse tree object: truncated hash")
		}
		var rawHash [20]byte
		copy(rawHash[:], data[:20])
		data = data[20:]

		hash, err := gitcore.NewHashFromBytes(rawHash)
		if err != nil {
			return nil, fmt.Errorf("failed to parse tree object: invalid hash: %w", err)
		}

		fmt.Fprintf(&buf, "%s %s %s\t%s\n", mode, treeEntryType(mode), hash, name)
	}

	return buf.Bytes(), nil
}

func treeEntryType(mode string) string {
	switch {
	case len(mode) >= 3 && mode[:3] == "100":
		return "blob"
	case mode == "040000" || mode == "40000":
		return "tree"
	case mode == "120000":
		return "blob"
	case mode == "160000":
		return "commit"
	default:
		return "invalid"
	}
}
