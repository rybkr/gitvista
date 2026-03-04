package server

import (
	"testing"
	"time"
)

func TestAnalyticsModuleKey(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "web/analyticsView.js", want: "web/"},
		{path: "internal/server/handlers.go", want: "internal/server/"},
		{path: "cmd/main.go", want: "cmd/"},
		{path: "go.mod", want: "go.mod"},
	}

	for _, tt := range tests {
		if got := analyticsModuleKey(tt.path); got != tt.want {
			t.Fatalf("analyticsModuleKey(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestAnalyticsRiskScoreAndStatus(t *testing.T) {
	score := analyticsRiskScore(90, 100, 30, 35, 60)
	if score < 50 {
		t.Fatalf("score too low: got %d", score)
	}
	if status := analyticsRiskStatus(score); status != "risk" && status != "watch" {
		t.Fatalf("unexpected status: got %q", status)
	}

	low := analyticsRiskScore(2, 100, 1, 1, 5)
	if low >= 40 {
		t.Fatalf("expected low score, got %d", low)
	}
	if status := analyticsRiskStatus(low); status != "ok" {
		t.Fatalf("low score status = %q, want ok", status)
	}
}

func TestAnalyticsWindowing(t *testing.T) {
	now := time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC)
	entries := []analyticsCommitEntry{
		{TS: now.AddDate(0, -8, 0)},
		{TS: now.AddDate(0, -2, 0)},
	}

	start, end := analyticsCurrentWindow(analyticsQuery{period: "6m"}, 6, entries, now)
	if !start.Equal(now.AddDate(0, -6, 0)) {
		t.Fatalf("start = %s, want %s", start, now.AddDate(0, -6, 0))
	}
	if !end.Equal(now) {
		t.Fatalf("end = %s, want %s", end, now)
	}

	ps, pe := analyticsPreviousWindow(start, end)
	if !pe.Before(start) {
		t.Fatalf("previous end must be before current start: prevEnd=%s start=%s", pe, start)
	}
	if !ps.Before(pe) {
		t.Fatalf("previous start must be before previous end: prevStart=%s prevEnd=%s", ps, pe)
	}
}

func TestFilterNonMergeEntries(t *testing.T) {
	entries := []analyticsCommitEntry{
		{Parents: 0},
		{Parents: 1},
		{Parents: 2},
		{Parents: 3},
	}

	filtered := filterNonMergeEntries(entries)
	if len(filtered) != 2 {
		t.Fatalf("filtered length = %d, want 2", len(filtered))
	}
	for _, e := range filtered {
		if e.Parents > 1 {
			t.Fatalf("unexpected merge entry retained: parents=%d", e.Parents)
		}
	}
}
