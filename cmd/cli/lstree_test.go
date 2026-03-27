package main

import (
	"strings"
	"testing"
)

func TestParseLsTreeArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantRev  string
		wantCode int
		wantErr  string
	}{
		{name: "revision", args: []string{"HEAD"}, wantRev: "HEAD"},
		{name: "missing args", args: nil, wantCode: 1, wantErr: "usage: gitvista-cli ls-tree"},
		{name: "too many args", args: []string{"HEAD", "extra"}, wantCode: 1, wantErr: "usage: gitvista-cli ls-tree"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, code, err := parseLsTreeArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || code != tt.wantCode {
					t.Fatalf("parseLsTreeArgs() = (%+v, %d, %v)", opts, code, err)
				}
				return
			}
			if err != nil || code != 0 || opts.revision != tt.wantRev {
				t.Fatalf("parseLsTreeArgs() = (%+v, %d, %v)", opts, code, err)
			}
		})
	}
}
