package main

import "testing"

func TestParseFlagsDefaultsPort(t *testing.T) {
	flags, err := parseFlags(nil, func(key, fallback string) string {
		return fallback
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if flags.port != "8080" {
		t.Fatalf("parseFlags default port = %q, want %q", flags.port, "8080")
	}
}

func TestParseFlagsAcceptsLongPort(t *testing.T) {
	flags, err := parseFlags([]string{"--port", "9090"}, func(key, fallback string) string {
		return fallback
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if flags.port != "9090" {
		t.Fatalf("parseFlags port = %q, want %q", flags.port, "9090")
	}
}

func TestResolveBindHost(t *testing.T) {
	tests := []struct {
		name     string
		repoPath string
		host     string
		want     string
	}{
		{
			name:     "local mode defaults to loopback",
			repoPath: "/tmp/repo",
			host:     "",
			want:     "127.0.0.1",
		},
		{
			name:     "hosted mode keeps all-interfaces default",
			repoPath: "",
			host:     "",
			want:     "",
		},
		{
			name:     "explicit host wins in local mode",
			repoPath: "/tmp/repo",
			host:     "0.0.0.0",
			want:     "0.0.0.0",
		},
		{
			name:     "explicit host wins in hosted mode",
			repoPath: "",
			host:     "0.0.0.0",
			want:     "0.0.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveBindHost(tt.repoPath, tt.host)
			if got != tt.want {
				t.Errorf("resolveBindHost(%q, %q) = %q, want %q", tt.repoPath, tt.host, got, tt.want)
			}
		})
	}
}

func TestApplyInvocationDefaults(t *testing.T) {
	tests := []struct {
		name  string
		argv0 string
		flags appFlags
		want  string
	}{
		{
			name:  "git-vista defaults to current directory",
			argv0: "git-vista",
			flags: appFlags{},
			want:  ".",
		},
		{
			name:  "gitvista keeps hosted default",
			argv0: "gitvista",
			flags: appFlags{},
			want:  "",
		},
		{
			name:  "explicit repo path wins",
			argv0: "git-vista",
			flags: appFlags{repoPath: "/tmp/repo"},
			want:  "/tmp/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := tt.flags
			applyInvocationDefaults(&flags, tt.argv0)
			if flags.repoPath != tt.want {
				t.Fatalf("repoPath = %q, want %q", flags.repoPath, tt.want)
			}
		})
	}
}
