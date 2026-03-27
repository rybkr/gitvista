package main

import (
	"strings"
	"testing"
)

func TestParseBlameArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantPath string
		wantCode int
		wantErr  string
	}{
		{name: "path only", args: []string{"README.md"}, wantPath: "README.md"},
		{name: "explicit porcelain", args: []string{"-p", "cmd/cli"}, wantPath: "cmd/cli"},
		{name: "missing args", args: nil, wantCode: 1, wantErr: "usage: gitvista-cli blame"},
		{name: "missing path", args: []string{""}, wantCode: 1, wantErr: "missing path"},
		{name: "unsupported flag", args: []string{"--bad", "README.md"}, wantCode: 1, wantErr: "unsupported argument"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, code, err := parseBlameArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || code != tt.wantCode {
					t.Fatalf("parseBlameArgs() = (%+v, %d, %v)", opts, code, err)
				}
				return
			}
			if err != nil || code != 0 || opts.path != tt.wantPath {
				t.Fatalf("parseBlameArgs() = (%+v, %d, %v)", opts, code, err)
			}
		})
	}
}

func TestNormalizeBlamePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "cleans nested path", input: "cmd/../README.md", want: "README.md"},
		{name: "cleans current directory", input: "./README.md", want: "README.md"},
		{name: "rejects empty", input: "", wantErr: "empty path"},
		{name: "rejects absolute", input: "/tmp/file", wantErr: "absolute paths are not allowed"},
		{name: "rejects parent escape", input: "../secret", wantErr: "path escapes repository root"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeBlamePath(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("normalizeBlamePath(%q) = (%q, %v)", tt.input, got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("normalizeBlamePath(%q) = (%q, %v), want (%q, nil)", tt.input, got, err, tt.want)
			}
		})
	}
}
