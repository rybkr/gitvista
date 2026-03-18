package main

import (
	"testing"

	"github.com/rybkr/gitvista/gitcore"
)

func TestParseRevListArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		want     revListOptions
		wantCode int
		wantErr  string
	}{
		{
			name: "all with count and topo order",
			args: []string{"--all", "--count", "--topo-order"},
			want: revListOptions{
				all:       true,
				count:     true,
				orderMode: revListOrderTopo,
			},
		},
		{
			name: "revision with filters",
			args: []string{"HEAD~2", "--no-merges", "--date-order"},
			want: revListOptions{
				revision:  "HEAD~2",
				noMerges:  true,
				orderMode: revListOrderDate,
			},
		},
		{
			name:     "missing selector",
			args:     []string{"--count"},
			wantCode: 1,
			wantErr:  "missing revision",
		},
		{
			name:     "unsupported flag",
			args:     []string{"--bad", "HEAD"},
			wantCode: 1,
			wantErr:  "unsupported argument",
		},
		{
			name:     "too many revisions",
			args:     []string{"HEAD", "main"},
			wantCode: 1,
			wantErr:  "accepts at most one revision argument",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, code, err := parseRevListArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || code != tt.wantCode || !contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseRevListArgs() = (%+v, %d, %v)", got, code, err)
				}
				return
			}
			if err != nil || code != 0 {
				t.Fatalf("parseRevListArgs() = (%+v, %d, %v)", got, code, err)
			}
			if got != tt.want {
				t.Fatalf("parseRevListArgs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMapRevListOrder(t *testing.T) {
	tests := []struct {
		name string
		in   revListOrder
		want gitcore.RevListOrder
	}{
		{name: "chronological", in: revListOrderChronological, want: gitcore.RevListOrderChronological},
		{name: "topo", in: revListOrderTopo, want: gitcore.RevListOrderTopo},
		{name: "date", in: revListOrderDate, want: gitcore.RevListOrderDate},
		{name: "unknown defaults", in: revListOrder(99), want: gitcore.RevListOrderChronological},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapRevListOrder(tt.in); got != tt.want {
				t.Fatalf("mapRevListOrder(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}())
}
