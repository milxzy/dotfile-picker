// package backup handles creating and managing config backups
package backup

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Manager handles backup operations
type Manager struct {
	backupDir string
}

// BackupMetadata tracks what was backed up and when
type BackupMetadata struct {
	OriginalPath string    `json:"original_path"`
	BackupPath   string    `json:"backup_path"`
	Timestamp    time.Time `json:"timestamp"`
	CreatorID    string    `json:"creator_id"`
	DotfileID    string    `json:"dotfile_id"`
}

// NewManager creates a backup manager
// backupDir is where all backups will be stored
func NewManager(backupDir string) *Manager {
	return &Manager{
		backupDir: backupDir,
	}
}

// Backup creates a backup of a file
// returns the backup path and metadata
func (m *Manager) Backup(originalPath, creatorID, dotfileID string) (*BackupMetadata, error) {
	// check if the original file exists
	if _, err := os.Stat(originalPath); os.IsNotExist(err) {
		// nothing to backup
		return nil, nil
	}

	// create a timestamped backup filename
	timestamp := time.Now()
	backupName := fmt.Sprintf(
		"%s_%s_%s.bak",
		filepath.Base(originalPath),
		timestamp.Format("20060102_150405"),
		dotfileID,
	)

	// preserve directory structure in backup
	relativePath, err := filepath.Rel(os.Getenv("HOME"), originalPath)
	if err != nil {
		// if not under home, just use the filename
		relativePath = filepath.Base(originalPath)
	}

	backupPath := filepath.Join(m.backupDir, filepath.Dir(relativePath), backupName)

	// ensure backup directory exists
	if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
		return nil, fmt.Errorf("couldn't create backup directory: %w", err)
	}

	// copy the file
	if err := copyFile(originalPath, backupPath); err != nil {
		return nil, fmt.Errorf("couldn't backup file: %w", err)
	}

	metadata := &BackupMetadata{
		OriginalPath: originalPath,
		BackupPath:   backupPath,
		Timestamp:    timestamp,
		CreatorID:    creatorID,
		DotfileID:    dotfileID,
	}

	// save metadata
	if err := m.saveMetadata(metadata); err != nil {
		// not critical, but log it
		_ = err
	}

	return metadata, nil
}

// ListBackups returns all backups for a specific file
func (m *Manager) ListBackups(originalPath string) ([]*BackupMetadata, error) {
	metadataPath := m.getMetadataPath()

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*BackupMetadata{}, nil
		}
		return nil, err
	}

	var allBackups []*BackupMetadata
	if err := json.Unmarshal(data, &allBackups); err != nil {
		return nil, err
	}

	// filter by original path
	var backups []*BackupMetadata
	for _, backup := range allBackups {
		if backup.OriginalPath == originalPath {
			backups = append(backups, backup)
		}
	}

	return backups, nil
}

// Restore restores a file from backup
func (m *Manager) Restore(backupPath, originalPath string) error {
	// check if backup exists
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup doesn't exist: %w", err)
	}

	// copy backup to original location
	if err := copyFile(backupPath, originalPath); err != nil {
		return fmt.Errorf("couldn't restore file: %w", err)
	}

	return nil
}

// saveMetadata appends backup metadata to the manifest
func (m *Manager) saveMetadata(metadata *BackupMetadata) error {
	metadataPath := m.getMetadataPath()

	// load existing metadata
	var allBackups []*BackupMetadata
	if data, err := os.ReadFile(metadataPath); err == nil {
		_ = json.Unmarshal(data, &allBackups)
	}

	// append new metadata
	allBackups = append(allBackups, metadata)

	// save back
	data, err := json.MarshalIndent(allBackups, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metadataPath, data, 0644)
}

// getMetadataPath returns the path to the backup manifest
func (m *Manager) getMetadataPath() string {
	return filepath.Join(m.backupDir, "backup_manifest.json")
}

// copyFile copies a file from src to dst
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
