// package cache handles git operations and repo management
package cache

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

// HasSubmodules checks if a repository contains git submodules
func HasSubmodules(repoDir string) (bool, error) {
	gitmodulesPath := filepath.Join(repoDir, ".gitmodules")
	_, err := os.Stat(gitmodulesPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// InitSubmodules initializes git submodules in a repository
// Note: go-git has limited submodule support, so we fall back to git CLI
func InitSubmodules(ctx context.Context, repoDir string) error {
	// Try using git CLI directly for better submodule support
	// This handles SSH keys, HTTPS auth, and recursive submodules better

	// First check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git command not found (needed for submodule support)")
	}

	// Run: git submodule update --init --recursive
	cmd := exec.CommandContext(ctx, "git", "submodule", "update", "--init", "--recursive")
	cmd.Dir = repoDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)

		// Check for specific error types
		if strings.Contains(outputStr, "Repository not found") {
			return fmt.Errorf("some submodules are private or don't exist - this is normal, skip and continue\n\nDetails: %s", outputStr)
		}

		if strings.Contains(outputStr, "ssh:") || strings.Contains(outputStr, "Permission denied") {
			return fmt.Errorf("submodule uses SSH authentication - please set up SSH keys or skip submodules\n%s", outputStr)
		}

		if strings.Contains(outputStr, "correct access rights") {
			return fmt.Errorf("submodule requires authentication - skip this step to continue\n\nDetails: %s", outputStr)
		}

		return fmt.Errorf("couldn't initialize submodules: %w\n%s", err, output)
	}

	return nil
}

// CountSubmodules returns the number of submodules in a repo
func CountSubmodules(repoDir string) (int, error) {
	gitmodulesPath := filepath.Join(repoDir, ".gitmodules")
	data, err := os.ReadFile(gitmodulesPath)
	if err != nil {
		return 0, err
	}

	// Count [submodule "..."] entries
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "[submodule") {
			count++
		}
	}

	return count, nil
}
