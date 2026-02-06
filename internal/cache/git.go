// package cache handles git operations and repo management
package cache

import (
	"context"
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// CloneRepo clones a git repository to the target directory
// supports context cancellation for long-running operations
func CloneRepo(ctx context.Context, url, targetDir string) error {
	// check if the directory already exists
	if _, err := os.Stat(targetDir); err == nil {
		// repo already exists, try to pull instead
		return PullRepo(ctx, targetDir)
	}

	// clone the repo
	_, err := git.PlainCloneContext(ctx, targetDir, false, &git.CloneOptions{
		URL:      url,
		Progress: nil, // we'll add progress later
		Depth:    1,   // shallow clone for speed
	})

	if err != nil {
		return fmt.Errorf("couldn't clone repo: %w", err)
	}

	return nil
}

// PullRepo updates an existing git repository
// pulls latest changes from the default remote
func PullRepo(ctx context.Context, repoDir string) error {
	// open the existing repo
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return fmt.Errorf("couldn't open repo: %w", err)
	}

	// get the working tree
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("couldn't get worktree: %w", err)
	}

	// pull latest changes
	err = worktree.PullContext(ctx, &git.PullOptions{
		RemoteName: "origin",
	})

	// if already up to date, that's fine
	if err == git.NoErrAlreadyUpToDate {
		return nil
	}

	if err != nil {
		return fmt.Errorf("couldn't pull updates: %w", err)
	}

	return nil
}

// GetLatestCommit returns the hash of the latest commit
// useful for checking if repo has been updated
func GetLatestCommit(repoDir string) (string, error) {
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return "", fmt.Errorf("couldn't open repo: %w", err)
	}

	ref, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("couldn't get head: %w", err)
	}

	return ref.Hash().String(), nil
}

// RepoExists checks if a git repo exists at the given path
func RepoExists(repoDir string) bool {
	_, err := git.PlainOpen(repoDir)
	return err == nil
}

// CheckoutBranch switches to a specific branch
// useful if we want to support pinning to specific versions later
func CheckoutBranch(repoDir, branch string) error {
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return fmt.Errorf("couldn't open repo: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("couldn't get worktree: %w", err)
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branch),
	})

	if err != nil {
		return fmt.Errorf("couldn't checkout branch %s: %w", branch, err)
	}

	return nil
}
