package hosted

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rybkr/gitvista/internal/repomanager"
)

const postgresDriverName = "postgres"

type postgresHostedStore struct {
	db             *sql.DB
	repoManager    *repomanager.RepoManager
	systemUserID   string
	defaultAccount HostedAccount
}

func NewPostgresHostedStore(dataSourceName string, rm *repomanager.RepoManager) (HostedStore, error) {
	if strings.TrimSpace(dataSourceName) == "" {
		return nil, errors.New("empty postgres data source name")
	}
	if rm == nil {
		return nil, errors.New("nil repo manager")
	}

	db, err := sql.Open(postgresDriverName, dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	ctx, cancel := hostedStoreContext()
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	store := &postgresHostedStore{
		db:           db,
		repoManager:  rm,
		systemUserID: "usr_system",
		defaultAccount: HostedAccount{
			ID:        "acct_" + DefaultHostedAccountSlug,
			Slug:      DefaultHostedAccountSlug,
			Name:      "Personal",
			CreatedAt: time.Now().UTC(),
			IsDefault: true,
		},
	}

	if err := store.bootstrap(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := store.ensureDefaultAccount(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	account, err := store.lookupAccount(ctx, DefaultHostedAccountSlug)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	store.defaultAccount = account

	return store, nil
}

func (s *postgresHostedStore) DefaultAccount() HostedAccount {
	return s.defaultAccount
}

func (s *postgresHostedStore) CreateAccount(name, slug string) (HostedAccount, error) {
	normalizedSlug, err := normalizeHostedAccountSlug(slug)
	if err != nil {
		return HostedAccount{}, err
	}

	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		trimmedName = normalizedSlug
	}

	existing, err := s.GetAccount(normalizedSlug)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrHostedAccountNotFound) {
		return HostedAccount{}, err
	}

	now := time.Now().UTC()
	account := HostedAccount{
		ID:        "acct_" + normalizedSlug,
		Slug:      normalizedSlug,
		Name:      trimmedName,
		IsDefault: normalizedSlug == s.defaultAccount.Slug,
		CreatedAt: now,
	}

	ctx, cancel := hostedStoreContext()
	defer cancel()

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO hosted_accounts (id, slug, name, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (slug) DO NOTHING
	`, account.ID, account.Slug, account.Name, account.CreatedAt); err != nil {
		return HostedAccount{}, fmt.Errorf("create account: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO account_memberships (account_slug, user_id, role, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (account_slug, user_id) DO NOTHING
	`, account.Slug, s.systemUserID, "owner", now); err != nil {
		return HostedAccount{}, fmt.Errorf("create default account membership: %w", err)
	}

	return s.GetAccount(account.Slug)
}

func (s *postgresHostedStore) GetAccount(slug string) (HostedAccount, error) {
	ctx, cancel := hostedStoreContext()
	defer cancel()
	return s.lookupAccount(ctx, slug)
}

func (s *postgresHostedStore) ListAccounts() ([]HostedAccount, error) {
	ctx, cancel := hostedStoreContext()
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, slug, name, created_at
		FROM hosted_accounts
		ORDER BY created_at ASC, slug ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	var accounts []HostedAccount
	for rows.Next() {
		var account HostedAccount
		if err := rows.Scan(&account.ID, &account.Slug, &account.Name, &account.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		account.IsDefault = account.Slug == s.defaultAccount.Slug
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].IsDefault != accounts[j].IsDefault {
			return accounts[i].IsDefault
		}
		if accounts[i].CreatedAt.Equal(accounts[j].CreatedAt) {
			return accounts[i].Slug < accounts[j].Slug
		}
		return accounts[i].CreatedAt.Before(accounts[j].CreatedAt)
	})
	return accounts, nil
}

func (s *postgresHostedStore) AddRepo(accountSlug, rawURL string) (HostedRepo, error) {
	account, err := s.requireAccount(accountSlug)
	if err != nil {
		return HostedRepo{}, err
	}

	managedRepoID, err := s.repoManager.AddRepo(rawURL)
	if err != nil {
		return HostedRepo{}, err
	}

	info, err := s.repoManager.Info(managedRepoID)
	if err != nil {
		return HostedRepo{}, err
	}

	repoID := deriveHostedRepoID(account.Slug, managedRepoID)

	existing, err := s.GetRepo(account.Slug, repoID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrHostedRepoNotFound) {
		return HostedRepo{}, err
	}

	accessToken, err := newHostedAccessToken()
	if err != nil {
		return HostedRepo{}, err
	}

	now := time.Now().UTC()
	ctx, cancel := hostedStoreContext()
	defer cancel()

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO hosted_repositories (
			id, account_slug, managed_repo_id, url, display_name, access_token, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO NOTHING
	`, repoID, account.Slug, managedRepoID, info.URL, hostedRepoDisplayName(info.URL), accessToken, now)
	if err != nil {
		return HostedRepo{}, fmt.Errorf("insert hosted repository: %w", err)
	}

	return s.GetRepo(account.Slug, repoID)
}

func (s *postgresHostedStore) ListRepos(accountSlug string) ([]HostedRepo, error) {
	account, err := s.requireAccount(accountSlug)
	if err != nil {
		return nil, err
	}

	ctx, cancel := hostedStoreContext()
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, account_slug, managed_repo_id, url, display_name, access_token, created_at
		FROM hosted_repositories
		WHERE account_slug = $1
	`, account.Slug)
	if err != nil {
		return nil, fmt.Errorf("list hosted repositories: %w", err)
	}
	defer rows.Close()

	var repos []HostedRepo
	for rows.Next() {
		repo, err := scanHostedRepo(rows.Scan)
		if err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hosted repositories: %w", err)
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].CreatedAt.After(repos[j].CreatedAt)
	})
	return repos, nil
}

func (s *postgresHostedStore) GetRepo(accountSlug, repoID string) (HostedRepo, error) {
	account, err := s.requireAccount(accountSlug)
	if err != nil {
		return HostedRepo{}, err
	}

	ctx, cancel := hostedStoreContext()
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, account_slug, managed_repo_id, url, display_name, access_token, created_at
		FROM hosted_repositories
		WHERE account_slug = $1 AND id = $2
	`, account.Slug, repoID)
	repo, err := scanHostedRepo(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return HostedRepo{}, ErrHostedRepoNotFound
		}
		return HostedRepo{}, err
	}
	return repo, nil
}

func (s *postgresHostedStore) AuthorizeRepo(accountSlug, repoID, accessToken string) (HostedRepo, error) {
	if strings.TrimSpace(accessToken) == "" {
		return HostedRepo{}, ErrHostedRepoNotFound
	}

	account, err := s.requireAccount(accountSlug)
	if err != nil {
		return HostedRepo{}, err
	}

	ctx, cancel := hostedStoreContext()
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, account_slug, managed_repo_id, url, display_name, access_token, created_at
		FROM hosted_repositories
		WHERE account_slug = $1 AND id = $2 AND access_token = $3
	`, account.Slug, repoID, accessToken)
	repo, err := scanHostedRepo(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return HostedRepo{}, ErrHostedRepoNotFound
		}
		return HostedRepo{}, err
	}
	return repo, nil
}

func (s *postgresHostedStore) RemoveRepo(accountSlug, repoID string) error {
	repo, err := s.GetRepo(accountSlug, repoID)
	if err != nil {
		return err
	}

	ctx, cancel := hostedStoreContext()
	defer cancel()

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM hosted_repositories
		WHERE account_slug = $1 AND id = $2
	`, repo.AccountSlug, repo.ID)
	if err != nil {
		return fmt.Errorf("delete hosted repository: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrHostedRepoNotFound
	}

	var remaining int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM hosted_repositories
		WHERE managed_repo_id = $1
	`, repo.ManagedRepoID).Scan(&remaining); err != nil {
		return fmt.Errorf("count managed repository references: %w", err)
	}
	if remaining == 0 {
		if err := s.repoManager.Remove(repo.ManagedRepoID); err != nil {
			return err
		}
	}
	return nil
}

func (s *postgresHostedStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *postgresHostedStore) bootstrap(ctx context.Context) error {
	if err := applyHostedStoreMigrations(ctx, s.db); err != nil {
		return fmt.Errorf("bootstrap hosted store schema: %w", err)
	}
	return nil
}

func (s *postgresHostedStore) ensureDefaultAccount(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO hosted_users (id, email, display_name, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO NOTHING
	`, s.systemUserID, "system@gitvista.local", "System", time.Now().UTC()); err != nil {
		return fmt.Errorf("ensure system user: %w", err)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO hosted_accounts (id, slug, name, created_at)
		VALUES ($1, $2, $3, $4)
	        ON CONFLICT (slug) DO NOTHING
	`, s.defaultAccount.ID, s.defaultAccount.Slug, s.defaultAccount.Name, s.defaultAccount.CreatedAt)
	if err != nil {
		return fmt.Errorf("ensure default account: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO account_memberships (account_slug, user_id, role, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (account_slug, user_id) DO NOTHING
	`, s.defaultAccount.Slug, s.systemUserID, "owner", time.Now().UTC()); err != nil {
		return fmt.Errorf("ensure default account membership: %w", err)
	}
	return nil
}

func (s *postgresHostedStore) lookupAccount(ctx context.Context, accountSlug string) (HostedAccount, error) {
	slug, err := normalizeHostedAccountSlug(accountSlug)
	if err != nil {
		return HostedAccount{}, err
	}

	var account HostedAccount
	row := s.db.QueryRowContext(ctx, `
		SELECT id, slug, name, created_at
		FROM hosted_accounts
		WHERE slug = $1
	`, slug)
	if err := row.Scan(&account.ID, &account.Slug, &account.Name, &account.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return HostedAccount{}, fmt.Errorf("%w: %s", ErrHostedAccountNotFound, slug)
		}
		return HostedAccount{}, fmt.Errorf("lookup account: %w", err)
	}
	account.IsDefault = account.Slug == s.defaultAccount.Slug
	return account, nil
}

func (s *postgresHostedStore) requireAccount(accountSlug string) (HostedAccount, error) {
	ctx, cancel := hostedStoreContext()
	defer cancel()
	return s.lookupAccount(ctx, accountSlug)
}

type hostedRepoScanner func(dest ...any) error

func scanHostedRepo(scan hostedRepoScanner) (HostedRepo, error) {
	var repo HostedRepo
	if err := scan(
		&repo.ID,
		&repo.AccountSlug,
		&repo.ManagedRepoID,
		&repo.URL,
		&repo.DisplayName,
		&repo.AccessToken,
		&repo.CreatedAt,
	); err != nil {
		return HostedRepo{}, err
	}
	return repo, nil
}
