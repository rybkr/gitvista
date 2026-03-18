package hosted

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

const hostedGooseMigrationsDir = "migrations"

//go:embed migrations/*.sql
var hostedGooseMigrationsFS embed.FS

func applyHostedStoreMigrations(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(hostedGooseMigrationsFS)
	if err := goose.SetDialect(postgresDriverName); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, hostedGooseMigrationsDir); err != nil {
		return fmt.Errorf("apply goose migrations: %w", err)
	}
	return nil
}
