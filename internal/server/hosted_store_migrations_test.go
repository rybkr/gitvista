package server

import "testing"

func TestHostedStoreMigrations_StrictlyIncrease(t *testing.T) {
	migrations := hostedStoreMigrations()
	if len(migrations) == 0 {
		t.Fatal("expected hosted store migrations")
	}

	lastVersion := 0
	for _, migration := range migrations {
		if migration.version <= lastVersion {
			t.Fatalf("migration version %d is not greater than previous version %d", migration.version, lastVersion)
		}
		if migration.name == "" {
			t.Fatalf("migration %d has empty name", migration.version)
		}
		if len(migration.upSQL) == 0 {
			t.Fatalf("migration %d has no statements", migration.version)
		}
		lastVersion = migration.version
	}
}

func TestHostedStoreMigrations_CurrentVersionMatchesLastMigration(t *testing.T) {
	migrations := hostedStoreMigrations()
	got := migrations[len(migrations)-1].version
	if got != 4 {
		t.Fatalf("latest hosted store schema version = %d, want 4", got)
	}
}
