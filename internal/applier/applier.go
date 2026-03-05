// package applier handles applying dotfiles to the user's system
package applier

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/milxzy/dotfile-picker/internal/backup"
	"github.com/milxzy/dotfile-picker/internal/config"
	"github.com/milxzy/dotfile-picker/internal/fsutil"
	"github.com/milxzy/dotfile-picker/internal/logger"
	"github.com/milxzy/dotfile-picker/internal/manifest"
)

// Applier handles copying dotfiles from cache to user's home
type Applier struct {
	backupManager *backup.Manager
	homeDir       string
	config        *config.Config
}

// NewApplier creates a new applier
func NewApplier(backupManager *backup.Manager, cfg *config.Config) (*Applier, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("couldn't get home directory: %w", err)
	}

	return &Applier{
		backupManager: backupManager,
		homeDir:       homeDir,
		config:        cfg,
	}, nil
}

// ApplyResult represents the result of applying a dotfile
type ApplyResult struct {
	Success    bool
	BackupPath string
	TargetPath string
	Error      error
	Skipped    bool
}

// Apply copies a dotfile from the cached repo to the target location
// creates backup of existing file first
func (a *Applier) Apply(sourcePath, targetRelPath string, creator *manifest.Creator, dotfile *manifest.Dotfile) *ApplyResult {
	result := &ApplyResult{}

	// resolve target path (expand to full path)
	targetPath := a.ResolveTargetPath(targetRelPath, a.homeDir)
	result.TargetPath = targetPath

	logger.Debug("Applying file:")
	logger.Debug("  Source: %s", sourcePath)
	logger.Debug("  Target: %s", targetPath)

	// check if target exists
	if info, err := os.Stat(targetPath); err == nil {
		logger.Debug("  Existing file: YES (size: %d bytes, mode: %v)", info.Size(), info.Mode())
	} else {
		logger.Debug("  Existing file: NO (new file)")
	}

	// create backup if file exists
	backupMetadata, err := a.backupManager.Backup(targetPath, creator.ID, dotfile.ID)
	if err != nil {
		logger.Error("  Backup failed: %v", err)
		result.Error = fmt.Errorf("couldn't create backup: %w", err)
		return result
	}

	if backupMetadata != nil {
		logger.Info("  Backup created: %s", backupMetadata.BackupPath)
		result.BackupPath = backupMetadata.BackupPath
	}

	// ensure target directory exists
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		logger.Error("  Failed to create target directory: %v", err)
		result.Error = fmt.Errorf("couldn't create target directory: %w", err)
		return result
	}

	// copy the file
	logger.Debug("  Copying file...")
	if err := copyFile(sourcePath, targetPath); err != nil {
		logger.Error("  Copy failed: %v", err)
		result.Error = fmt.Errorf("couldn't copy file: %w", err)
		// try to restore backup if copy failed
		if backupMetadata != nil {
			logger.Info("  Attempting to restore backup...")
			_ = a.backupManager.Restore(backupMetadata.BackupPath, targetPath)
		}
		return result
	}

	logger.Info("  ✓ File applied successfully")
	result.Success = true
	return result
}

// ApplyMultiple applies multiple dotfiles
// returns results for each file
func (a *Applier) ApplyMultiple(files map[string]string, creator *manifest.Creator, dotfile *manifest.Dotfile) []*ApplyResult {
	logger.Section("Applying Files")
	logger.Info("Total files to apply: %d", len(files))

	var results []*ApplyResult
	successCount := 0

	fileNum := 1
	for sourcePath, targetRelPath := range files {
		logger.Debug("--- File %d/%d ---", fileNum, len(files))
		result := a.Apply(sourcePath, targetRelPath, creator, dotfile)
		results = append(results, result)
		if result.Success {
			successCount++
		}
		fileNum++
	}

	logger.Section("Application Summary")
	logger.Info("Successfully applied: %d/%d files", successCount, len(files))
	if successCount < len(files) {
		logger.Error("Failed to apply: %d files", len(files)-successCount)
		for _, result := range results {
			if !result.Success && result.Error != nil {
				logger.Error("  %s: %v", result.TargetPath, result.Error)
			}
		}
	}

	return results
}

// ResolveTargetPath converts a relative path to an absolute path
// handles both ~/.config/... and .config/... formats
// also handles XDG config directory auto-detection
func (a *Applier) ResolveTargetPath(relPath, homeDir string) string {
	logger.Debug("  Resolving target path: %s", relPath)

	// if path starts with ~, expand it
	if len(relPath) > 0 && relPath[0] == '~' {
		result := filepath.Join(homeDir, relPath[1:])
		logger.Debug("    → Tilde expansion: %s", result)
		return result
	}

	// if path starts with ., treat it relative to home (explicit dotfile)
	if len(relPath) > 0 && relPath[0] == '.' {
		result := filepath.Join(homeDir, relPath)
		logger.Debug("    → Explicit dotfile (starts with dot): %s", result)
		return result
	}

	// if path starts with /, use it as-is (absolute path)
	if filepath.IsAbs(relPath) {
		logger.Debug("    → Absolute path: %s", relPath)
		return relPath
	}

	// Check if this is a known XDG config directory
	if a.isXDGConfigDir(relPath) {
		result := filepath.Join(a.config.DotfilesRoot, relPath)
		logger.Debug("    → XDG config dir detected, using: %s", result)
		return result
	}

	// otherwise, treat as relative to home
	result := filepath.Join(homeDir, relPath)
	logger.Debug("    → Using home-relative path: %s", result)
	return result
}

// isXDGConfigDir checks if a path should go in the config root
func (a *Applier) isXDGConfigDir(relPath string) bool {
	if !a.config.AutoXDGDetection {
		return false
	}

	// Extract the first component of the path
	parts := strings.Split(filepath.Clean(relPath), string(filepath.Separator))
	if len(parts) == 0 {
		return false
	}

	firstDir := parts[0]

	// Check against known XDG directories
	for _, xdgDir := range a.config.XDGDirectories {
		if firstDir == xdgDir {
			logger.Debug("    → Matched XDG directory: %s", xdgDir)
			return true
		}
	}

	return false
}

// Rollback attempts to restore from backup
func (a *Applier) Rollback(results []*ApplyResult) error {
	var errs []error

	for _, result := range results {
		if result.BackupPath != "" {
			if err := a.backupManager.Restore(result.BackupPath, result.TargetPath); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("rollback encountered %d errors", len(errs))
	}

	return nil
}

// copyFile copies a file from src to dst preserving permissions.
func copyFile(src, dst string) error {
	return fsutil.CopyFile(src, dst)
}
