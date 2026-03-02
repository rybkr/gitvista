package main

import "testing"

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
			name:     "saas mode keeps all-interfaces default",
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
			name:     "explicit host wins in saas mode",
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
