// package manifest handles detecting different dotfile repo structures
package manifest

import (
	"os"
	"path/filepath"
	"strings"
)

// RepoStructure represents the type of dotfile organization
type RepoStructure int

const (
	// StructureUnknown means we couldn't detect the layout
	StructureUnknown RepoStructure = iota

	// StructureFlat means dotfiles are at repo root (.vimrc, .bashrc, etc)
	StructureFlat

	// StructureStow means GNU Stow layout (package/file structure)
	StructureStow

	// StructureChezmoi means chezmoi-managed dotfiles
	StructureChezmoi

	// StructureBareRepo means a bare git repo with worktree
	StructureBareRepo

	// StructureConfig means all files in a single config directory
	StructureConfig
)

// DetectStructure tries to figure out how the repo is organized
// this helps us find files when paths aren't specified in manifest
func DetectStructure(repoPath string) RepoStructure {
	// check for chezmoi - has .chezmoi.toml or .chezmoiroot
	if fileExists(filepath.Join(repoPath, ".chezmoi.toml")) ||
		fileExists(filepath.Join(repoPath, ".chezmoiroot")) {
		return StructureChezmoi
	}

	// check for stow layout - multiple top-level dirs with config structure
	if looksLikeStow(repoPath) {
		return StructureStow
	}

	// check if there's a single config directory
	configDir := filepath.Join(repoPath, "config")
	if dirExists(configDir) {
		return StructureConfig
	}

	// check for flat layout - dotfiles at root
	if hasDotfilesAtRoot(repoPath) {
		return StructureFlat
	}

	// couldn't detect
	return StructureUnknown
}

// ResolveFilePath tries to find a dotfile in the repo
// uses the detected structure to search intelligently
// accepts both files and directories
func ResolveFilePath(repoPath, relativePath string, structure RepoStructure) (string, bool) {
	// first try the exact path from manifest
	exactPath := filepath.Join(repoPath, relativePath)
	if pathExists(exactPath) {
		return exactPath, true
	}

	// try without leading dot if it has one
	if strings.HasPrefix(filepath.Base(relativePath), ".") {
		noDotPath := filepath.Join(repoPath, strings.TrimPrefix(filepath.Base(relativePath), "."))
		if pathExists(noDotPath) {
			return noDotPath, true
		}
	}

	// try common directory aliases (e.g., .config -> xdg_config, ~ -> home)
	pathAliases := map[string][]string{
		".config": {"xdg_config", "config"},
		"~":       {"home"},
	}

	for pattern, replacements := range pathAliases {
		if strings.Contains(relativePath, pattern) {
			for _, replacement := range replacements {
				aliasPath := strings.Replace(relativePath, pattern, replacement, 1)
				fullPath := filepath.Join(repoPath, aliasPath)
				if pathExists(fullPath) {
					return fullPath, true
				}
			}
		}
	}

	// search based on structure
	switch structure {
	case StructureStow:
		return findInStow(repoPath, relativePath)
	case StructureChezmoi:
		return findInChezmoi(repoPath, relativePath)
	case StructureConfig:
		configPath := filepath.Join(repoPath, "config", relativePath)
		if pathExists(configPath) {
			return configPath, true
		}
	case StructureFlat:
		// already tried at root
		return "", false
	}

	return "", false
}

// looksLikeStow checks if the repo uses GNU Stow layout
// stow repos have multiple top-level dirs like "vim", "tmux", etc
func looksLikeStow(repoPath string) bool {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return false
	}

	// count directories that look like package names
	packageDirs := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		// skip hidden dirs and git dirs
		if strings.HasPrefix(name, ".") || name == "scripts" || name == "bin" {
			continue
		}

		// check if this dir has config-like files
		pkgPath := filepath.Join(repoPath, name)
		if hasConfigFiles(pkgPath) {
			packageDirs++
		}
	}

	// if we found multiple package-like directories, it's probably stow
	return packageDirs >= 2
}

// findInStow searches for a file or directory in stow-style layout
func findInStow(repoPath, relativePath string) (string, bool) {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return "", false
	}

	// search in each package directory
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		pkgPath := filepath.Join(repoPath, entry.Name(), relativePath)
		if pathExists(pkgPath) {
			return pkgPath, true
		}
	}

	return "", false
}

// findInChezmoi searches for a file in chezmoi layout
func findInChezmoi(repoPath, relativePath string) (string, bool) {
	// chezmoi uses "dot_" prefix for dotfiles
	// so .vimrc becomes dot_vimrc
	base := filepath.Base(relativePath)
	dir := filepath.Dir(relativePath)

	if strings.HasPrefix(base, ".") {
		chezmoiName := "dot_" + strings.TrimPrefix(base, ".")
		chezmoiPath := filepath.Join(repoPath, dir, chezmoiName)
		if fileExists(chezmoiPath) {
			return chezmoiPath, true
		}
	}

	return "", false
}

// hasDotfilesAtRoot checks if there are dotfiles at repo root
func hasDotfilesAtRoot(repoPath string) bool {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return false
	}

	dotfileCount := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), ".") {
			// skip .git, .gitignore, etc
			if entry.Name() == ".git" || entry.Name() == ".gitignore" || entry.Name() == ".github" {
				continue
			}
			dotfileCount++
		}
	}

	return dotfileCount > 0
}

// hasConfigFiles checks if a directory contains config-like files
func hasConfigFiles(dirPath string) bool {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false
	}

	// look for common config patterns
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || strings.Contains(name, "config") {
			return true
		}
	}

	return false
}

// pathExists checks if a path exists (file or directory)
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists checks if a directory exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
