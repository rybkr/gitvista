package server

import (
	"io/fs"
	"testing"
)

func TestHostedGooseMigrationsEmbedded(t *testing.T) {
	entries, err := fs.Glob(hostedGooseMigrationsFS, hostedGooseMigrationsDir+"/*.sql")
	if err != nil {
		t.Fatalf("failed to glob embedded migrations: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("embedded migration count = %d, want 4", len(entries))
	}
}
