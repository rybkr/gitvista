package hosted

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rybkr/gitvista/internal/repomanager"
)

const DefaultHostedAccountSlug = "personal"

var ErrHostedRepoNotFound = errors.New("hosted repo not found")
var ErrHostedAccountNotFound = errors.New("hosted account not found")

type HostedAccount struct {
	ID        string
	Slug      string
	Name      string
	IsDefault bool
	CreatedAt time.Time
}

type HostedRepo struct {
	ID            string
	AccountSlug   string
	ManagedRepoID string
	URL           string
	DisplayName   string
	CreatedAt     time.Time
	AccessToken   string
}

type HostedStore interface {
	DefaultAccount() HostedAccount
	CreateAccount(name, slug string) (HostedAccount, error)
	GetAccount(slug string) (HostedAccount, error)
	ListAccounts() ([]HostedAccount, error)
	AddRepo(accountSlug, rawURL string) (HostedRepo, error)
	ListRepos(accountSlug string) ([]HostedRepo, error)
	GetRepo(accountSlug, repoID string) (HostedRepo, error)
	AuthorizeRepo(accountSlug, repoID, accessToken string) (HostedRepo, error)
	RemoveRepo(accountSlug, repoID string) error
	Close() error
}

type memoryHostedStore struct {
	mu             sync.RWMutex
	repoManager    *repomanager.RepoManager
	defaultAccount HostedAccount
	accounts       map[string]HostedAccount
	repos          map[string]map[string]HostedRepo
}

func NewMemoryHostedStore(rm *repomanager.RepoManager) HostedStore {
	now := time.Now()
	defaultAccount := HostedAccount{
		ID:        "acct_" + DefaultHostedAccountSlug,
		Slug:      DefaultHostedAccountSlug,
		Name:      "Personal",
		IsDefault: true,
		CreatedAt: now,
	}
	return &memoryHostedStore{
		repoManager:    rm,
		defaultAccount: defaultAccount,
		accounts: map[string]HostedAccount{
			defaultAccount.Slug: defaultAccount,
		},
		repos: make(map[string]map[string]HostedRepo),
	}
}

func (s *memoryHostedStore) DefaultAccount() HostedAccount {
	return s.defaultAccount
}

func (s *memoryHostedStore) CreateAccount(name, slug string) (HostedAccount, error) {
	normalizedSlug, err := normalizeHostedAccountSlug(slug)
	if err != nil {
		return HostedAccount{}, err
	}

	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		trimmedName = normalizedSlug
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if account, ok := s.accounts[normalizedSlug]; ok {
		return account, nil
	}

	account := HostedAccount{
		ID:        "acct_" + normalizedSlug,
		Slug:      normalizedSlug,
		Name:      trimmedName,
		IsDefault: normalizedSlug == s.defaultAccount.Slug,
		CreatedAt: time.Now(),
	}
	s.accounts[normalizedSlug] = account
	return account, nil
}

func (s *memoryHostedStore) GetAccount(slug string) (HostedAccount, error) {
	return s.requireAccount(slug)
}

func (s *memoryHostedStore) ListAccounts() ([]HostedAccount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	accounts := make([]HostedAccount, 0, len(s.accounts))
	for _, account := range s.accounts {
		accounts = append(accounts, account)
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

func (s *memoryHostedStore) AddRepo(accountSlug, rawURL string) (HostedRepo, error) {
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

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.repos[account.Slug]; !ok {
		s.repos[account.Slug] = make(map[string]HostedRepo)
	}

	if existing, ok := s.repos[account.Slug][repoID]; ok {
		return existing, nil
	}

	accessToken, err := newHostedAccessToken()
	if err != nil {
		return HostedRepo{}, err
	}

	repo := HostedRepo{
		ID:            repoID,
		AccountSlug:   account.Slug,
		ManagedRepoID: managedRepoID,
		URL:           info.URL,
		DisplayName:   hostedRepoDisplayName(info.URL),
		CreatedAt:     time.Now(),
		AccessToken:   accessToken,
	}
	s.repos[account.Slug][repoID] = repo
	return repo, nil
}

func (s *memoryHostedStore) ListRepos(accountSlug string) ([]HostedRepo, error) {
	account, err := s.requireAccount(accountSlug)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	accountRepos := s.repos[account.Slug]
	repos := make([]HostedRepo, 0, len(accountRepos))
	for _, repo := range accountRepos {
		repos = append(repos, repo)
	}
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].CreatedAt.After(repos[j].CreatedAt)
	})
	return repos, nil
}

func (s *memoryHostedStore) GetRepo(accountSlug, repoID string) (HostedRepo, error) {
	account, err := s.requireAccount(accountSlug)
	if err != nil {
		return HostedRepo{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	accountRepos := s.repos[account.Slug]
	repo, ok := accountRepos[repoID]
	if !ok {
		return HostedRepo{}, ErrHostedRepoNotFound
	}
	return repo, nil
}

func (s *memoryHostedStore) AuthorizeRepo(accountSlug, repoID, accessToken string) (HostedRepo, error) {
	repo, err := s.GetRepo(accountSlug, repoID)
	if err != nil {
		return HostedRepo{}, err
	}
	if accessToken == "" || repo.AccessToken != accessToken {
		return HostedRepo{}, ErrHostedRepoNotFound
	}
	return repo, nil
}

func (s *memoryHostedStore) RemoveRepo(accountSlug, repoID string) error {
	repo, err := s.GetRepo(accountSlug, repoID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	delete(s.repos[repo.AccountSlug], repo.ID)
	shouldRemoveManaged := true
	for slug, repos := range s.repos {
		for _, existing := range repos {
			if slug == repo.AccountSlug && existing.ID == repo.ID {
				continue
			}
			if existing.ManagedRepoID == repo.ManagedRepoID {
				shouldRemoveManaged = false
				break
			}
		}
		if !shouldRemoveManaged {
			break
		}
	}
	s.mu.Unlock()

	if shouldRemoveManaged {
		if err := s.repoManager.Remove(repo.ManagedRepoID); err != nil {
			return err
		}
	}
	return nil
}

func (s *memoryHostedStore) Close() error {
	return nil
}

func (s *memoryHostedStore) requireAccount(accountSlug string) (HostedAccount, error) {
	slug, err := normalizeHostedAccountSlug(accountSlug)
	if err != nil {
		return HostedAccount{}, err
	}

	s.mu.RLock()
	account, ok := s.accounts[slug]
	s.mu.RUnlock()
	if !ok {
		return HostedAccount{}, fmt.Errorf("%w: %s", ErrHostedAccountNotFound, slug)
	}
	return account, nil
}

func normalizeHostedAccountSlug(slug string) (string, error) {
	slug = strings.TrimSpace(strings.ToLower(slug))
	if slug == "" {
		slug = DefaultHostedAccountSlug
	}
	for i, r := range slug {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return "", fmt.Errorf("invalid account slug %q", slug)
		}
		if r == '-' && (i == 0 || i == len(slug)-1) {
			return "", fmt.Errorf("invalid account slug %q", slug)
		}
	}
	return slug, nil
}

func deriveHostedRepoID(accountSlug, managedRepoID string) string {
	sum := sha256.Sum256([]byte(accountSlug + ":" + managedRepoID))
	return hex.EncodeToString(sum[:8])
}

func newHostedAccessToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate access token: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

func hostedRepoDisplayName(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	path := strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
	if path != "" {
		return path
	}

	return rawURL
}

func hostedStoreContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}
