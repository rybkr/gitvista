package gitcore

import (
	"os"
	"testing"
)

func BenchmarkNewRepository(b *testing.B) {
	repoPath := os.Getenv("GITVISTA_BENCH_REPO")
	if repoPath == "" {
		b.Skip("set GITVISTA_BENCH_REPO to benchmark repository loading")
	}

	b.ReportAllocs()
	for b.Loop() {
		repo, err := NewRepository(repoPath)
		if err != nil {
			b.Fatalf("NewRepository(%q): %v", repoPath, err)
		}
		if err := repo.Close(); err != nil {
			b.Fatalf("Close(): %v", err)
		}
	}
}
