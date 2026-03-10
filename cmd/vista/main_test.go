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
		name string
		host string
		want string
	}{
		{
			name: "defaults to loopback",
			host: "",
			want: "127.0.0.1",
		},
		{
			name: "explicit host wins",
			host: "0.0.0.0",
			want: "0.0.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveBindHost(tt.host)
			if got != tt.want {
				t.Errorf("resolveBindHost(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}
}

func TestApplyInvocationDefaults(t *testing.T) {
	t.Run("defaults to current directory", func(t *testing.T) {
		flags := appFlags{}
		applyInvocationDefaults(&flags)
		if flags.repoPath != "." {
			t.Fatalf("repoPath = %q, want %q", flags.repoPath, ".")
		}
	})

	t.Run("explicit repo path wins", func(t *testing.T) {
		flags := appFlags{repoPath: "/tmp/repo"}
		applyInvocationDefaults(&flags)
		if flags.repoPath != "/tmp/repo" {
			t.Fatalf("repoPath = %q, want %q", flags.repoPath, "/tmp/repo")
		}
	})
}
