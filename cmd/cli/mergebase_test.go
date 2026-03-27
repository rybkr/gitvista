package main

import (
	"strings"
	"testing"
)

func TestParseMergeBaseArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantOurs   string
		wantTheirs string
		wantCode   int
		wantErr    string
	}{
		{name: "two revisions", args: []string{"HEAD", "main"}, wantOurs: "HEAD", wantTheirs: "main"},
		{name: "missing args", args: nil, wantCode: 1, wantErr: "usage: gitvista-cli merge-base"},
		{name: "one arg", args: []string{"HEAD"}, wantCode: 1, wantErr: "usage: gitvista-cli merge-base"},
		{name: "too many args", args: []string{"HEAD", "main", "extra"}, wantCode: 1, wantErr: "usage: gitvista-cli merge-base"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, code, err := parseMergeBaseArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || code != tt.wantCode {
					t.Fatalf("parseMergeBaseArgs() = (%+v, %d, %v)", opts, code, err)
				}
				return
			}
			if err != nil || code != 0 || opts.ours != tt.wantOurs || opts.theirs != tt.wantTheirs {
				t.Fatalf("parseMergeBaseArgs() = (%+v, %d, %v)", opts, code, err)
			}
		})
	}
}
