package server

import (
	"net/http/httptest"
	"testing"
)

func TestParseBulkDiffStatsLimit(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  int
	}{
		{name: "default", query: "", want: 3000},
		{name: "valid custom limit", query: "?limit=25", want: 25},
		{name: "negative limit falls back", query: "?limit=-10", want: 3000},
		{name: "invalid limit falls back", query: "?limit=abc", want: 3000},
		{name: "limit is capped", query: "?limit=50000", want: 20000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/commits/diffstats"+tt.query, nil)
			if got := parseBulkDiffStatsLimit(req); got != tt.want {
				t.Fatalf("parseBulkDiffStatsLimit(%q) = %d, want %d", tt.query, got, tt.want)
			}
		})
	}
}
