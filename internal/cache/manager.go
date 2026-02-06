// package cache manages downloaded dotfile repositories
package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/milxzy/dot-generator/internal/manifest"
)

// Manager handles caching and syncing of dotfile repos
type Manager struct {
	cacheDir string
	mu       sync.RWMutex
}

// NewManager creates a cache manager
// cacheDir is where all repos will be stored
func NewManager(cacheDir string) *Manager {
	return &Manager{
		cacheDir: cacheDir,
	}
}

// EnsureRepo makes sure a creator's repo is downloaded and up to date
// clones if missing, pulls if stale
func (m *Manager) EnsureRepo(ctx context.Context, creator *manifest.Creator) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	repoPath := m.getRepoPath(creator.ID)

	// check if repo exists
	if !RepoExists(repoPath) {
		// clone it
		if err := CloneRepo(ctx, creator.Repo, repoPath); err != nil {
			return fmt.Errorf("couldn't download %s's dotfiles: %w", creator.Name, err)
		}
		return nil
	}

	// repo exists, check if we should update it
	// for now, just try to pull
	if err := PullRepo(ctx, repoPath); err != nil {
		// not critical if pull fails - we have the cached version
		// just continue with what we have
		return nil
	}

	return nil
}

// EnsureRepos downloads multiple repos concurrently
// uses a worker pool to limit concurrent git operations
func (m *Manager) EnsureRepos(ctx context.Context, creators []manifest.Creator) error {
	// use a worker pool to limit concurrency
	maxWorkers := 5
	sem := make(chan struct{}, maxWorkers)
	errChan := make(chan error, len(creators))
	var wg sync.WaitGroup

	for _, creator := range creators {
		wg.Add(1)
		go func(c manifest.Creator) {
			defer wg.Done()

			// acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// download the repo
			if err := m.EnsureRepo(ctx, &c); err != nil {
				errChan <- err
			}
		}(creator)
	}

	// wait for all downloads to complete
	wg.Wait()
	close(errChan)

	// check if any errors occurred
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		// return the first error for now
		// (we could combine them later)
		return errs[0]
	}

	return nil
}

// GetRepoPath returns the local path to a creator's repo
func (m *Manager) GetRepoPath(creatorID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getRepoPath(creatorID)
}

// getRepoPath is the internal version without locking
func (m *Manager) getRepoPath(creatorID string) string {
	return filepath.Join(m.cacheDir, creatorID)
}

// IsRepoCached checks if a creator's repo exists locally
func (m *Manager) IsRepoCached(creatorID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	repoPath := m.getRepoPath(creatorID)
	return RepoExists(repoPath)
}

// GetRepoAge returns how long ago the repo was last updated
func (m *Manager) GetRepoAge(creatorID string) (time.Duration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	repoPath := m.getRepoPath(creatorID)

	// check the .git directory's modification time
	gitDir := filepath.Join(repoPath, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return 0, fmt.Errorf("couldn't stat repo: %w", err)
	}

	return time.Since(info.ModTime()), nil
}

// ClearCache removes all cached repos
// useful for testing or forcing a fresh download
func (m *Manager) ClearCache() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return os.RemoveAll(m.cacheDir)
}

// ClearCreatorCache removes a specific creator's cached repo
func (m *Manager) ClearCreatorCache(creatorID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	repoPath := m.getRepoPath(creatorID)
	return os.RemoveAll(repoPath)
}

// ListCachedCreators returns a list of all cached creator IDs
func (m *Manager) ListCachedCreators() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, err := os.ReadDir(m.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var creators []string
	for _, entry := range entries {
		if entry.IsDir() {
			creators = append(creators, entry.Name())
		}
	}

	return creators, nil
}
