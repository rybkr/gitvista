package server

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type hostedStoreMigration struct {
	version int
	name    string
	upSQL   []string
}

func hostedStoreMigrations() []hostedStoreMigration {
	return []hostedStoreMigration{
		{
			version: 1,
			name:    "create_hosted_accounts",
			upSQL: []string{
				`CREATE TABLE hosted_accounts (
					id TEXT PRIMARY KEY,
					slug TEXT NOT NULL UNIQUE,
					name TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
			},
		},
		{
			version: 2,
			name:    "create_hosted_repositories",
			upSQL: []string{
				`CREATE TABLE hosted_repositories (
					id TEXT PRIMARY KEY,
					account_slug TEXT NOT NULL REFERENCES hosted_accounts(slug) ON DELETE CASCADE,
					managed_repo_id TEXT NOT NULL,
					url TEXT NOT NULL,
					display_name TEXT NOT NULL,
					access_token TEXT NOT NULL UNIQUE,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX hosted_repositories_account_slug_created_at_idx
					ON hosted_repositories (account_slug, created_at DESC)`,
				`CREATE INDEX hosted_repositories_managed_repo_id_idx
					ON hosted_repositories (managed_repo_id)`,
			},
		},
		{
			version: 3,
			name:    "create_hosted_users",
			upSQL: []string{
				`CREATE TABLE hosted_users (
					id TEXT PRIMARY KEY,
					email TEXT NOT NULL UNIQUE,
					display_name TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
			},
		},
		{
			version: 4,
			name:    "create_account_memberships",
			upSQL: []string{
				`CREATE TABLE account_memberships (
					account_slug TEXT NOT NULL REFERENCES hosted_accounts(slug) ON DELETE CASCADE,
					user_id TEXT NOT NULL REFERENCES hosted_users(id) ON DELETE CASCADE,
					role TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					PRIMARY KEY (account_slug, user_id)
				)`,
				`CREATE INDEX account_memberships_user_id_idx
					ON account_memberships (user_id)`,
			},
		},
	}
}

func applyHostedStoreMigrations(ctx context.Context, db *sql.DB) error {
	if err := ensureHostedStoreMigrationTable(ctx, db); err != nil {
		return err
	}

	currentVersion, err := currentHostedStoreSchemaVersion(ctx, db)
	if err != nil {
		return err
	}

	for _, migration := range hostedStoreMigrations() {
		if migration.version <= currentVersion {
			continue
		}
		if err := applyHostedStoreMigration(ctx, db, migration); err != nil {
			return err
		}
		currentVersion = migration.version
	}

	return nil
}

func ensureHostedStoreMigrationTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS hosted_schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("ensure hosted schema migrations table: %w", err)
	}
	return nil
}

func currentHostedStoreSchemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	row := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM hosted_schema_migrations`)
	if err := row.Scan(&version); err != nil {
		return 0, fmt.Errorf("read hosted schema version: %w", err)
	}
	return version, nil
}

func applyHostedStoreMigration(ctx context.Context, db *sql.DB, migration hostedStoreMigration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin hosted migration %d: %w", migration.version, err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, stmt := range migration.upSQL {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apply hosted migration %d (%s): %w", migration.version, migration.name, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO hosted_schema_migrations (version, name, applied_at)
		VALUES ($1, $2, $3)
	`, migration.version, migration.name, time.Now().UTC()); err != nil {
		return fmt.Errorf("record hosted migration %d (%s): %w", migration.version, migration.name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit hosted migration %d (%s): %w", migration.version, migration.name, err)
	}
	return nil
}
