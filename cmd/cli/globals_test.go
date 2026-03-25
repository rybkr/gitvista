package main

import (
	"testing"

	"github.com/rybkr/gitvista/internal/cli"
)

func TestParseGlobalFlagsDefaults(t *testing.T) {
	flags, remaining, err := parseGlobalFlags(nil)
	if err != nil {
		t.Fatalf("parseGlobalFlags() error = %v", err)
	}

	if flags.colorMode != cli.ColorAuto {
		t.Fatalf("colorMode = %v, want %v", flags.colorMode, cli.ColorAuto)
	}
	if flags.repoPath != "." {
		t.Fatalf("repoPath = %q, want %q", flags.repoPath, ".")
	}
	if flags.cpuProfilePath != "" {
		t.Fatalf("cpuProfilePath = %q, want empty", flags.cpuProfilePath)
	}
	if flags.memProfilePath != "" {
		t.Fatalf("memProfilePath = %q, want empty", flags.memProfilePath)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining = %v, want empty", remaining)
	}
}

func TestParseGlobalFlagsParsesSupportedFlags(t *testing.T) {
	flags, remaining, err := parseGlobalFlags([]string{
		"--no-color",
		"--repo", "/tmp/repo",
		"--cpuprofile=cpu.pprof",
		"--memprofile", "mem.pprof",
		"rev-list",
		"HEAD",
	})
	if err != nil {
		t.Fatalf("parseGlobalFlags() error = %v", err)
	}

	if flags.colorMode != cli.ColorNever {
		t.Fatalf("colorMode = %v, want %v", flags.colorMode, cli.ColorNever)
	}
	if flags.repoPath != "/tmp/repo" {
		t.Fatalf("repoPath = %q, want %q", flags.repoPath, "/tmp/repo")
	}
	if flags.cpuProfilePath != "cpu.pprof" {
		t.Fatalf("cpuProfilePath = %q, want %q", flags.cpuProfilePath, "cpu.pprof")
	}
	if flags.memProfilePath != "mem.pprof" {
		t.Fatalf("memProfilePath = %q, want %q", flags.memProfilePath, "mem.pprof")
	}
	if len(remaining) != 2 || remaining[0] != "rev-list" || remaining[1] != "HEAD" {
		t.Fatalf("remaining = %v, want [rev-list HEAD]", remaining)
	}
}

func TestParseGlobalFlagsParsesColorForms(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want cli.ColorMode
	}{
		{
			name: "separate argument",
			args: []string{"--color", "always"},
			want: cli.ColorAlways,
		},
		{
			name: "equals argument",
			args: []string{"--color=never"},
			want: cli.ColorNever,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, remaining, err := parseGlobalFlags(tt.args)
			if err != nil {
				t.Fatalf("parseGlobalFlags() error = %v", err)
			}
			if flags.colorMode != tt.want {
				t.Fatalf("colorMode = %v, want %v", flags.colorMode, tt.want)
			}
			if len(remaining) != 0 {
				t.Fatalf("remaining = %v, want empty", remaining)
			}
		})
	}
}

func TestParseStringFlag(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		index     int
		flag      string
		wantValue string
		wantSkip  int
		wantOK    bool
		wantErr   string
	}{
		{
			name:      "separate value",
			args:      []string{"--repo", "/tmp/repo"},
			index:     0,
			flag:      "--repo",
			wantValue: "/tmp/repo",
			wantSkip:  1,
			wantOK:    true,
		},
		{
			name:      "inline value",
			args:      []string{"--repo=/tmp/repo"},
			index:     0,
			flag:      "--repo",
			wantValue: "/tmp/repo",
			wantSkip:  0,
			wantOK:    true,
		},
		{
			name:   "different flag",
			args:   []string{"rev-list"},
			index:  0,
			flag:   "--repo",
			wantOK: false,
		},
		{
			name:    "missing separate value",
			args:    []string{"--repo"},
			index:   0,
			flag:    "--repo",
			wantErr: "missing value",
		},
		{
			name:    "missing inline value",
			args:    []string{"--repo="},
			index:   0,
			flag:    "--repo",
			wantErr: "missing value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotSkip, gotOK, err := parseStringFlag(tt.args, tt.index, tt.flag)
			if tt.wantErr != "" {
				if err == nil || err.Error() == "" || gotOK {
					t.Fatalf("parseStringFlag() = (%q, %d, %t, %v)", gotValue, gotSkip, gotOK, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseStringFlag() error = %v", err)
			}
			if gotValue != tt.wantValue || gotSkip != tt.wantSkip || gotOK != tt.wantOK {
				t.Fatalf("parseStringFlag() = (%q, %d, %t), want (%q, %d, %t)", gotValue, gotSkip, gotOK, tt.wantValue, tt.wantSkip, tt.wantOK)
			}
		})
	}
}

func TestParseGlobalFlagsRejectsMissingValues(t *testing.T) {
	tests := [][]string{
		{"--repo"},
		{"--cpuprofile"},
		{"--memprofile="},
		{"--color"},
	}

	for _, args := range tests {
		if _, _, err := parseGlobalFlags(args); err == nil {
			t.Fatalf("parseGlobalFlags(%v) unexpectedly succeeded", args)
		}
	}
}
