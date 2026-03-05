// package manifest handles detecting different dotfile repo structures
package manifest

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/milxzy/dotfile-picker/internal/logger"
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
	logger.Debug("Detecting repository structure for: %s", repoPath)
	logger.DirListing(repoPath, "  ")

	// check for chezmoi - has .chezmoi.toml or .chezmoiroot
	logger.Debug("Checking for chezmoi indicators...")
	if fileExists(filepath.Join(repoPath, ".chezmoi.toml")) ||
		fileExists(filepath.Join(repoPath, ".chezmoiroot")) {
		logger.Info("Detected structure: CHEZMOI")
		return StructureChezmoi
	}
	logger.Debug("  No chezmoi indicators found")

	// check for stow layout - multiple top-level dirs with config structure
	logger.Debug("Checking for stow layout...")
	if looksLikeStow(repoPath) {
		logger.Info("Detected structure: STOW")
		return StructureStow
	}
	logger.Debug("  Not a stow layout")

	// check if there's a single config directory
	logger.Debug("Checking for config directory...")
	configDir := filepath.Join(repoPath, "config")
	if dirExists(configDir) {
		logger.Info("Detected structure: CONFIG (single config directory)")
		logger.DirListing(configDir, "  ")
		return StructureConfig
	}
	logger.Debug("  No config directory found")

	// check for flat layout - dotfiles at root
	logger.Debug("Checking for flat layout (dotfiles at root)...")
	if hasDotfilesAtRoot(repoPath) {
		logger.Info("Detected structure: FLAT (dotfiles at root)")
		return StructureFlat
	}
	logger.Debug("  No dotfiles at root")

	// couldn't detect
	logger.Warn("Could not detect repository structure - will use manual browsing")
	return StructureUnknown
}

// ResolveFilePath tries to find a dotfile in the repo
// uses the detected structure to search intelligently
// accepts both files and directories
func ResolveFilePath(repoPath, relativePath string, structure RepoStructure) (string, bool) {
	logger.Debug("Resolving file path: %s (structure: %v)", relativePath, structure)

	// first try the exact path from manifest
	exactPath := filepath.Join(repoPath, relativePath)
	logger.Debug("  Trying exact path: %s", exactPath)
	if pathExists(exactPath) {
		logger.Info("  ✓ FOUND at exact path: %s", exactPath)
		return exactPath, true
	}
	logger.Debug("    Not found")

	// try without leading dot if it has one
	if strings.HasPrefix(filepath.Base(relativePath), ".") {
		noDotPath := filepath.Join(repoPath, strings.TrimPrefix(filepath.Base(relativePath), "."))
		logger.Debug("  Trying without leading dot: %s", noDotPath)
		if pathExists(noDotPath) {
			logger.Info("  ✓ FOUND without leading dot: %s", noDotPath)
			return noDotPath, true
		}
		logger.Debug("    Not found")
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
				logger.Debug("  Trying alias: %s -> %s", pattern, fullPath)
				if pathExists(fullPath) {
					logger.Info("  ✓ FOUND via alias: %s", fullPath)
					return fullPath, true
				}
				logger.Debug("    Not found")
			}
		}
	}

	// search based on structure
	logger.Debug("  Searching using structure-specific logic...")
	var result string
	var found bool

	switch structure {
	case StructureStow:
		result, found = findInStow(repoPath, relativePath)
	case StructureChezmoi:
		result, found = findInChezmoi(repoPath, relativePath)
	case StructureConfig:
		configPath := filepath.Join(repoPath, "config", relativePath)
		logger.Debug("    Trying config directory: %s", configPath)
		if pathExists(configPath) {
			logger.Info("  ✓ FOUND in config directory: %s", configPath)
			return configPath, true
		}
		logger.Debug("      Not found")
		return "", false
	case StructureFlat:
		// already tried at root
		logger.Debug("    Flat structure - already checked at root")
		return "", false
	}

	if found {
		logger.Info("  ✓ FOUND via structure search: %s", result)
	} else {
		logger.Warn("  ✗ NOT FOUND: %s", relativePath)
	}

	return result, found
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
	var foundPackages []string

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
			foundPackages = append(foundPackages, name)
		}
	}

	// if we found multiple package-like directories, it's probably stow
	isStow := packageDirs >= 2
	if isStow {
		logger.Debug("  Found %d package directories: %v", packageDirs, foundPackages)
	}
	return isStow
}

// findInStow searches for a file or directory in stow-style layout
func findInStow(repoPath, relativePath string) (string, bool) {
	logger.Debug("    Searching in stow packages...")
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		logger.Debug("      Failed to read directory: %v", err)
		return "", false
	}

	// search in each package directory
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		pkgPath := filepath.Join(repoPath, entry.Name(), relativePath)
		logger.Debug("      Checking package '%s': %s", entry.Name(), pkgPath)
		if pathExists(pkgPath) {
			logger.Debug("      ✓ Found in package: %s", entry.Name())
			return pkgPath, true
		}
	}

	logger.Debug("      Not found in any stow package")
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
