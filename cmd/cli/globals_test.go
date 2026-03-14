package main

import "testing"

func TestParseGlobalFlagsExtractsProfiles(t *testing.T) {
	flags, remaining := parseGlobalFlags([]string{
		"--repo", "/tmp/repo",
		"--cpuprofile", "/tmp/cli.cpu.prof",
		"--memprofile", "/tmp/cli.mem.prof",
		"repo",
	})

	if flags.repoPath != "/tmp/repo" {
		t.Fatalf("repoPath = %q, want %q", flags.repoPath, "/tmp/repo")
	}
	if flags.cpuProfilePath != "/tmp/cli.cpu.prof" {
		t.Fatalf("cpuProfilePath = %q, want %q", flags.cpuProfilePath, "/tmp/cli.cpu.prof")
	}
	if flags.memProfilePath != "/tmp/cli.mem.prof" {
		t.Fatalf("memProfilePath = %q, want %q", flags.memProfilePath, "/tmp/cli.mem.prof")
	}
	if len(remaining) != 1 || remaining[0] != "repo" {
		t.Fatalf("remaining = %v, want [repo]", remaining)
	}
}
