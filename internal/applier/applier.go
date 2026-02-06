// package applier handles applying dotfiles to the user's system
package applier

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/milxzy/dot-generator/internal/backup"
	"github.com/milxzy/dot-generator/internal/manifest"
)

// Applier handles copying dotfiles from cache to user's home
type Applier struct {
	backupManager *backup.Manager
	homeDir       string
}

// NewApplier creates a new applier
func NewApplier(backupManager *backup.Manager) (*Applier, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("couldn't get home directory: %w", err)
	}

	return &Applier{
		backupManager: backupManager,
		homeDir:       homeDir,
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
	targetPath := a.resolveTargetPath(targetRelPath)
	result.TargetPath = targetPath

	// create backup if file exists
	backupMetadata, err := a.backupManager.Backup(targetPath, creator.ID, dotfile.ID)
	if err != nil {
		result.Error = fmt.Errorf("couldn't create backup: %w", err)
		return result
	}

	if backupMetadata != nil {
		result.BackupPath = backupMetadata.BackupPath
	}

	// ensure target directory exists
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		result.Error = fmt.Errorf("couldn't create target directory: %w", err)
		return result
	}

	// copy the file
	if err := copyFile(sourcePath, targetPath); err != nil {
		result.Error = fmt.Errorf("couldn't copy file: %w", err)
		// try to restore backup if copy failed
		if backupMetadata != nil {
			_ = a.backupManager.Restore(backupMetadata.BackupPath, targetPath)
		}
		return result
	}

	result.Success = true
	return result
}

// ApplyMultiple applies multiple dotfiles
// returns results for each file
func (a *Applier) ApplyMultiple(files map[string]string, creator *manifest.Creator, dotfile *manifest.Dotfile) []*ApplyResult {
	var results []*ApplyResult

	for sourcePath, targetRelPath := range files {
		result := a.Apply(sourcePath, targetRelPath, creator, dotfile)
		results = append(results, result)
	}

	return results
}

// resolveTargetPath converts a relative path to an absolute path
// handles both ~/.config/... and .config/... formats
func (a *Applier) resolveTargetPath(relPath string) string {
	// if path starts with ~, expand it
	if len(relPath) > 0 && relPath[0] == '~' {
		return filepath.Join(a.homeDir, relPath[1:])
	}

	// if path starts with ., treat it relative to home
	if len(relPath) > 0 && relPath[0] == '.' {
		return filepath.Join(a.homeDir, relPath)
	}

	// if path starts with /, use it as-is
	if filepath.IsAbs(relPath) {
		return relPath
	}

	// otherwise, treat as relative to home
	return filepath.Join(a.homeDir, relPath)
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

// copyFile copies a file from src to dst preserving permissions
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return err
	}

	// copy permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, sourceInfo.Mode())
}
