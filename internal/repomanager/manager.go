// Package repomanager handles lifecycle management of cloned Git repositories,
// including cloning, periodic fetching, session tracking, and eviction.
package repomanager

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rybkr/gitvista/internal/gitcore"
)

// RepoState represents the lifecycle state of a managed repository.
type RepoState int

const (
	// StatePending indicates the repo is queued for cloning.
	StatePending RepoState = iota
	// StateCloning indicates the repo is currently being cloned.
	StateCloning
	// StateReady indicates the repo has been cloned and is available.
	StateReady
	// StateError indicates a failure during cloning or fetching.
	StateError
)

// String returns a human-readable representation of the state.
func (s RepoState) String() string {
	switch s {
	case StatePending:
		return "pending"
	case StateCloning:
		return "cloning"
	case StateReady:
		return "ready"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// Config holds settings for the RepoManager.
type Config struct {
	DataDir             string
	MaxConcurrentClones int
	FetchInterval       time.Duration
	InactivityTTL       time.Duration
	CloneTimeout        time.Duration
	FetchTimeout        time.Duration
	MaxRepos            int
	Logger              *slog.Logger
}

// defaults fills zero-valued fields with sensible defaults.
func (c *Config) defaults() {
	if c.DataDir == "" {
		c.DataDir = "/data/repos"
	}
	if c.MaxConcurrentClones <= 0 {
		c.MaxConcurrentClones = 3
	}
	if c.FetchInterval <= 0 {
		c.FetchInterval = 30 * time.Second
	}
	if c.InactivityTTL <= 0 {
		c.InactivityTTL = 24 * time.Hour
	}
	if c.CloneTimeout <= 0 {
		c.CloneTimeout = 5 * time.Minute
	}
	if c.FetchTimeout <= 0 {
		c.FetchTimeout = 2 * time.Minute
	}
	if c.MaxRepos <= 0 {
		c.MaxRepos = 100
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

// ManagedRepo tracks a single remote repository through its lifecycle.
type ManagedRepo struct {
	mu         sync.RWMutex
	ID         string
	URL        string // original URL
	NormURL    string // canonicalized URL
	State      RepoState
	Error      string
	DiskPath   string
	Repo       *gitcore.Repository // non-nil when Ready
	CreatedAt  time.Time
	LastAccess time.Time
	LastFetch  time.Time
}

// RepoInfo is a read-only snapshot of a managed repository's state, used by List().
type RepoInfo struct {
	ID         string
	URL        string
	State      RepoState
	Error      string
	CreatedAt  time.Time
	LastAccess time.Time
	LastFetch  time.Time
}

// RepoManager manages the lifecycle of cloned remote repositories.
type RepoManager struct {
	cfg        Config
	logger     *slog.Logger
	mu         sync.RWMutex
	repos      map[string]*ManagedRepo
	cloneQueue chan *ManagedRepo
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// New creates a RepoManager and ensures the data directory exists.
func New(cfg Config) (*RepoManager, error) {
	cfg.defaults()

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create data directory %s: %w", cfg.DataDir, err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &RepoManager{
		cfg:        cfg,
		logger:     cfg.Logger,
		repos:      make(map[string]*ManagedRepo),
		cloneQueue: make(chan *ManagedRepo, cfg.MaxRepos),
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// Start launches clone workers and the fetch/eviction loops.
func (rm *RepoManager) Start() error {
	for range rm.cfg.MaxConcurrentClones {
		rm.wg.Add(1)
		go rm.cloneWorker()
	}

	rm.wg.Add(1)
	go rm.fetchLoop()

	rm.wg.Add(1)
	go rm.evictionLoop()

	rm.logger.Info("repo manager started",
		"workers", rm.cfg.MaxConcurrentClones,
		"data_dir", rm.cfg.DataDir,
	)

	return nil
}

// Close shuts down all goroutines and waits for them to finish.
func (rm *RepoManager) Close() {
	rm.cancel()
	rm.wg.Wait()
	rm.logger.Info("repo manager stopped")
}

// AddRepo normalizes the URL, deduplicates, and enqueues a clone if needed.
// Returns the repo ID (derived from the normalized URL hash).
func (rm *RepoManager) AddRepo(rawURL string) (string, error) {
	normURL, err := normalizeURL(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	id := hashURL(normURL)

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Deduplication: if this repo already exists, return its ID.
	// Allow re-enqueueing repos in error state for retry.
	if existing, exists := rm.repos[id]; exists {
		existing.mu.Lock()
		if existing.State == StateError {
			existing.State = StatePending
			existing.Error = ""
			select {
			case rm.cloneQueue <- existing:
			default:
				existing.State = StateError
				existing.Error = "clone queue full"
			}
			existing.mu.Unlock()
			return id, nil
		}
		existing.mu.Unlock()

		return id, nil
	}

	if len(rm.repos) >= rm.cfg.MaxRepos {
		return "", fmt.Errorf("maximum number of repos (%d) reached", rm.cfg.MaxRepos)
	}

	now := time.Now()
	managed := &ManagedRepo{
		ID:         id,
		URL:        rawURL,
		NormURL:    normURL,
		State:      StatePending,
		DiskPath:   filepath.Join(rm.cfg.DataDir, id),
		CreatedAt:  now,
		LastAccess: now,
	}

	rm.repos[id] = managed

	select {
	case rm.cloneQueue <- managed:
	default:
		managed.State = StateError
		managed.Error = "clone queue full"
		return id, fmt.Errorf("clone queue full")
	}

	return id, nil
}

// GetRepo returns the loaded *gitcore.Repository for the given ID.
// Returns an error if the repo is not ready.
func (rm *RepoManager) GetRepo(id string) (*gitcore.Repository, error) {
	rm.mu.RLock()
	managed, exists := rm.repos[id]
	rm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("repo not found: %s", id)
	}

	managed.mu.Lock()
	defer managed.mu.Unlock()

	switch managed.State {
	case StateReady:
		managed.LastAccess = time.Now()
		return managed.Repo, nil
	case StatePending, StateCloning:
		return nil, fmt.Errorf("repo %s is still %s", id, managed.State)
	case StateError:
		return nil, fmt.Errorf("repo %s has error: %s", id, managed.Error)
	default:
		return nil, fmt.Errorf("repo %s is in unknown state", id)
	}
}

// Status returns the current state and error message for a repo.
func (rm *RepoManager) Status(id string) (RepoState, string, error) {
	rm.mu.RLock()
	managed, exists := rm.repos[id]
	rm.mu.RUnlock()

	if !exists {
		return 0, "", fmt.Errorf("repo not found: %s", id)
	}

	managed.mu.RLock()
	defer managed.mu.RUnlock()
	return managed.State, managed.Error, nil
}

// List returns a snapshot of all managed repositories.
func (rm *RepoManager) List() []RepoInfo {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]RepoInfo, 0, len(rm.repos))
	for _, managed := range rm.repos {
		managed.mu.RLock()
		result = append(result, RepoInfo{
			ID:         managed.ID,
			URL:        managed.URL,
			State:      managed.State,
			Error:      managed.Error,
			CreatedAt:  managed.CreatedAt,
			LastAccess: managed.LastAccess,
			LastFetch:  managed.LastFetch,
		})
		managed.mu.RUnlock()
	}
	return result
}

// Remove deletes a repo from the registry and its data from disk.
func (rm *RepoManager) Remove(id string) error {
	rm.mu.Lock()
	managed, exists := rm.repos[id]
	if !exists {
		rm.mu.Unlock()
		return fmt.Errorf("repo not found: %s", id)
	}
	delete(rm.repos, id)
	rm.mu.Unlock()

	managed.mu.Lock()
	diskPath := managed.DiskPath
	managed.mu.Unlock()

	if err := os.RemoveAll(diskPath); err != nil {
		rm.logger.Warn("failed to remove repo data", "id", id, "path", diskPath, "error", err)
	}

	rm.logger.Info("repo removed", "id", id)
	return nil
}

// cloneWorker pulls repos from the clone queue and processes them.
func (rm *RepoManager) cloneWorker() {
	defer rm.wg.Done()

	for {
		select {
		case <-rm.ctx.Done():
			return
		case managed, ok := <-rm.cloneQueue:
			if !ok {
				return
			}
			rm.processClone(managed)
		}
	}
}

// processClone performs the actual clone and loads the repository.
func (rm *RepoManager) processClone(managed *ManagedRepo) {
	managed.mu.Lock()
	managed.State = StateCloning
	repoURL := managed.URL
	diskPath := managed.DiskPath
	managed.mu.Unlock()

	rm.logger.Info("cloning repo", "id", managed.ID, "url", repoURL)

	// Remove any stale directory from a previous failed or interrupted clone.
	if err := os.RemoveAll(diskPath); err != nil {
		rm.logger.Warn("failed to clean stale directory before clone", "id", managed.ID, "path", diskPath, "error", err)
	}

	if err := cloneRepo(rm.ctx, repoURL, diskPath, rm.cfg.CloneTimeout); err != nil {
		managed.mu.Lock()
		managed.State = StateError
		managed.Error = err.Error()
		managed.mu.Unlock()
		rm.logger.Error("clone failed", "id", managed.ID, "error", err)
		return
	}

	repo, err := gitcore.NewRepository(diskPath)
	if err != nil {
		managed.mu.Lock()
		managed.State = StateError
		managed.Error = fmt.Sprintf("failed to load repository: %v", err)
		managed.mu.Unlock()
		rm.logger.Error("repo load failed after clone", "id", managed.ID, "error", err)
		return
	}

	now := time.Now()
	managed.mu.Lock()
	managed.State = StateReady
	managed.Error = ""
	managed.Repo = repo
	managed.LastFetch = now
	managed.LastAccess = now
	managed.mu.Unlock()

	rm.logger.Info("repo ready", "id", managed.ID)
}
