-- +goose Up
CREATE TABLE account_memberships (
    account_slug TEXT NOT NULL REFERENCES hosted_accounts(slug) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES hosted_users(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (account_slug, user_id)
);

CREATE INDEX account_memberships_user_id_idx
    ON account_memberships (user_id);

-- +goose Down
DROP INDEX IF EXISTS account_memberships_user_id_idx;
DROP TABLE IF EXISTS account_memberships;
