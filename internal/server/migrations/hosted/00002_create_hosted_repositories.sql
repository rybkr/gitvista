-- +goose Up
CREATE TABLE hosted_repositories (
    id TEXT PRIMARY KEY,
    account_slug TEXT NOT NULL REFERENCES hosted_accounts(slug) ON DELETE CASCADE,
    managed_repo_id TEXT NOT NULL,
    url TEXT NOT NULL,
    display_name TEXT NOT NULL,
    access_token TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX hosted_repositories_account_slug_created_at_idx
    ON hosted_repositories (account_slug, created_at DESC);

CREATE INDEX hosted_repositories_managed_repo_id_idx
    ON hosted_repositories (managed_repo_id);

-- +goose Down
DROP INDEX IF EXISTS hosted_repositories_managed_repo_id_idx;
DROP INDEX IF EXISTS hosted_repositories_account_slug_created_at_idx;
DROP TABLE IF EXISTS hosted_repositories;
