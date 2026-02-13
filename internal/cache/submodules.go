// package cache handles submodule detection and resolution
package cache

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SubmoduleConfig represents a git submodule entry
type SubmoduleConfig struct {
	Path string // relative path in repo (e.g., "nvim/.config/nvim")
	URL  string // git URL (e.g., "git@github.com:user/repo.git")
}

// ParseGitmodules reads and parses a .gitmodules file
// Returns a list of submodule configurations
func ParseGitmodules(repoDir string) ([]SubmoduleConfig, error) {
	gitmodulesPath := filepath.Join(repoDir, ".gitmodules")

	// Check if .gitmodules exists
	if _, err := os.Stat(gitmodulesPath); os.IsNotExist(err) {
		return nil, nil // No submodules
	}

	file, err := os.Open(gitmodulesPath)
	if err != nil {
		return nil, fmt.Errorf("couldn't open .gitmodules: %w", err)
	}
	defer file.Close()

	var submodules []SubmoduleConfig
	var current *SubmoduleConfig

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// New submodule section
		if strings.HasPrefix(line, "[submodule") {
			if current != nil {
				submodules = append(submodules, *current)
			}
			current = &SubmoduleConfig{}
			continue
		}

		if current == nil {
			continue
		}

		// Parse path
		if strings.HasPrefix(line, "path =") {
			current.Path = strings.TrimSpace(strings.TrimPrefix(line, "path ="))
		}

		// Parse URL
		if strings.HasPrefix(line, "url =") {
			current.URL = strings.TrimSpace(strings.TrimPrefix(line, "url ="))
		}
	}

	// Add the last submodule
	if current != nil && current.Path != "" && current.URL != "" {
		submodules = append(submodules, *current)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading .gitmodules: %w", err)
	}

	return submodules, nil
}

// ConvertSSHToHTTPS converts git SSH URLs to HTTPS URLs
// Example: git@github.com:user/repo.git -> https://github.com/user/repo
func ConvertSSHToHTTPS(sshURL string) string {
	// Handle SSH format: git@host:user/repo.git
	if strings.HasPrefix(sshURL, "git@") {
		// Remove git@ prefix
		url := strings.TrimPrefix(sshURL, "git@")

		// Replace : with /
		url = strings.Replace(url, ":", "/", 1)

		// Remove .git suffix
		url = strings.TrimSuffix(url, ".git")

		// Add https://
		return "https://" + url
	}

	// Already HTTPS or other format
	return sshURL
}

// FindSubmoduleByPath finds a submodule config by its path
func FindSubmoduleByPath(submodules []SubmoduleConfig, path string) *SubmoduleConfig {
	for i := range submodules {
		if submodules[i].Path == path {
			return &submodules[i]
		}
	}
	return nil
}

// ResolveSubmodule attempts to resolve an empty directory as a submodule
// by reading .gitmodules and cloning the submodule directly
func ResolveSubmodule(ctx context.Context, repoDir, submodulePath string) error {
	// Parse .gitmodules
	submodules, err := ParseGitmodules(repoDir)
	if err != nil {
		return fmt.Errorf("couldn't parse .gitmodules: %w", err)
	}

	if len(submodules) == 0 {
		return fmt.Errorf("no submodules found in .gitmodules")
	}

	// Find the submodule config for this path
	// submodulePath is absolute, we need to make it relative to repoDir
	relPath, err := filepath.Rel(repoDir, submodulePath)
	if err != nil {
		return fmt.Errorf("couldn't determine relative path: %w", err)
	}

	submodule := FindSubmoduleByPath(submodules, relPath)
	if submodule == nil {
		return fmt.Errorf("no submodule config found for path: %s", relPath)
	}

	// Convert SSH to HTTPS
	cloneURL := ConvertSSHToHTTPS(submodule.URL)

	// Try to clone the submodule directly
	err = CloneRepo(ctx, cloneURL, submodulePath)
	if err != nil {
		// If HTTPS fails and original was SSH, try SSH
		if cloneURL != submodule.URL {
			err = CloneRepo(ctx, submodule.URL, submodulePath)
			if err != nil {
				return fmt.Errorf("couldn't clone submodule (tried HTTPS and SSH): %w", err)
			}
		} else {
			return fmt.Errorf("couldn't clone submodule: %w", err)
		}
	}

	return nil
}

// IsEmptyDirectory checks if a directory is empty or only contains .git metadata
func IsEmptyDirectory(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}

	// Count non-.git entries
	count := 0
	for _, entry := range entries {
		// Skip .git file (submodule placeholder)
		if entry.Name() == ".git" {
			continue
		}
		count++
	}

	return count == 0, nil
}

// ResolveSubmodulesRecursive recursively resolves submodules up to maxDepth
// This handles nested submodules (submodules that themselves have submodules)
func ResolveSubmodulesRecursive(ctx context.Context, repoDir string, maxDepth int) error {
	if maxDepth <= 0 {
		return nil // Depth limit reached
	}

	// Parse .gitmodules in the current repo
	submodules, err := ParseGitmodules(repoDir)
	if err != nil {
		return fmt.Errorf("couldn't parse .gitmodules: %w", err)
	}

	if len(submodules) == 0 {
		return nil // No submodules to resolve
	}

	// Only show message at the top level (maxDepth == 3 is the initial call)
	if maxDepth == 3 {
		fmt.Fprintf(os.Stderr, "Resolving %d submodule(s)...\n", len(submodules))
	}

	// Resolve each submodule
	// Track at least one successful resolution
	successCount := 0

	for _, submodule := range submodules {
		submodulePath := filepath.Join(repoDir, submodule.Path)

		// Check if submodule directory is empty (needs resolution)
		isEmpty, err := IsEmptyDirectory(submodulePath)
		if err != nil {
			// Directory doesn't exist yet, create it
			if os.IsNotExist(err) {
				if err := os.MkdirAll(submodulePath, 0755); err != nil {
					// Warn but continue with other submodules
					continue
				}
				isEmpty = true
			} else {
				// Warn but continue with other submodules
				continue
			}
		}

		// If directory is not empty, it's already resolved
		if !isEmpty {
			// Still check for nested submodules
			if err := ResolveSubmodulesRecursive(ctx, submodulePath, maxDepth-1); err != nil {
				// Log but don't fail - nested submodules might not be critical
			}
			successCount++ // Already resolved counts as success
			continue
		}

		// Convert SSH to HTTPS
		cloneURL := ConvertSSHToHTTPS(submodule.URL)

		// Try to clone the submodule
		err = CloneRepo(ctx, cloneURL, submodulePath)
		if err != nil {
			// If HTTPS fails and original was SSH, try SSH
			if cloneURL != submodule.URL {
				err = CloneRepo(ctx, submodule.URL, submodulePath)
				if err != nil {
					// Warn but continue - submodule might be private (skip logging to keep UI clean)
					continue
				}
			} else {
				// Warn but continue - submodule might be private (skip logging to keep UI clean)
				continue
			}
		}

		successCount++

		// Recursively resolve nested submodules
		if err := ResolveSubmodulesRecursive(ctx, submodulePath, maxDepth-1); err != nil {
			// Log but don't fail - nested submodules might not be critical
		}
	}

	// Only fail if we couldn't resolve ANY submodules
	// This is lenient because some submodules might be private
	if len(submodules) > 0 && successCount == 0 {
		return fmt.Errorf("couldn't resolve any submodules (%d total)", len(submodules))
	}

	return nil
}
