package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupManager creates a Manager backed by a temp directory.
func setupManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	return NewManager(filepath.Join(dir, "backups")), dir
}

// writeFile writes content to a file at path, creating parent directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestBackup_NonExistentFile(t *testing.T) {
	mgr, dir := setupManager(t)
	meta, err := mgr.Backup(filepath.Join(dir, "no_such_file"), "creator", "dotfile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta != nil {
		t.Error("expected nil metadata for non-existent file")
	}
}

func TestBackup_ExistingFile(t *testing.T) {
	mgr, dir := setupManager(t)

	original := filepath.Join(dir, "config", ".vimrc")
	writeFile(t, original, "set number\n")

	meta, err := mgr.Backup(original, "creator1", "vim")
	if err != nil {
		t.Fatalf("Backup returned error: %v", err)
	}
	if meta == nil {
		t.Fatal("expected non-nil metadata for existing file")
	}

	// backup file should exist and have the same content
	data, err := os.ReadFile(meta.BackupPath)
	if err != nil {
		t.Fatalf("couldn't read backup file: %v", err)
	}
	if string(data) != "set number\n" {
		t.Errorf("backup content mismatch: got %q", string(data))
	}

	// metadata fields
	if meta.OriginalPath != original {
		t.Errorf("OriginalPath mismatch: got %q", meta.OriginalPath)
	}
	if meta.CreatorID != "creator1" {
		t.Errorf("CreatorID mismatch: got %q", meta.CreatorID)
	}
	if meta.DotfileID != "vim" {
		t.Errorf("DotfileID mismatch: got %q", meta.DotfileID)
	}
	if meta.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestBackup_TimestampOrder(t *testing.T) {
	mgr, dir := setupManager(t)

	original := filepath.Join(dir, ".bashrc")
	writeFile(t, original, "export PATH=$PATH:/usr/local/bin\n")

	meta1, err := mgr.Backup(original, "c", "bash")
	if err != nil {
		t.Fatalf("first backup: %v", err)
	}

	// tiny sleep so timestamps differ
	time.Sleep(2 * time.Millisecond)

	writeFile(t, original, "export PATH=$PATH:/opt/bin\n")
	meta2, err := mgr.Backup(original, "c", "bash")
	if err != nil {
		t.Fatalf("second backup: %v", err)
	}

	if !meta2.Timestamp.After(meta1.Timestamp) {
		t.Error("expected second backup timestamp to be after first")
	}
}

func TestRestore(t *testing.T) {
	mgr, dir := setupManager(t)

	original := filepath.Join(dir, ".zshrc")
	writeFile(t, original, "original content\n")

	meta, err := mgr.Backup(original, "c", "zsh")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	// overwrite original
	writeFile(t, original, "overwritten content\n")

	// restore from backup
	if err := mgr.Restore(meta.BackupPath, original); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	data, err := os.ReadFile(original)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "original content\n" {
		t.Errorf("restore content mismatch: got %q", string(data))
	}
}

func TestRestore_MissingBackup(t *testing.T) {
	mgr, dir := setupManager(t)
	err := mgr.Restore(filepath.Join(dir, "does_not_exist.bak"), filepath.Join(dir, "target"))
	if err == nil {
		t.Error("expected error restoring from missing backup, got nil")
	}
}

func TestListBackups_Empty(t *testing.T) {
	mgr, dir := setupManager(t)
	backups, err := mgr.ListBackups(filepath.Join(dir, ".vimrc"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("expected empty list, got %d entries", len(backups))
	}
}

func TestListBackups_AfterBackup(t *testing.T) {
	mgr, dir := setupManager(t)

	original := filepath.Join(dir, ".tmux.conf")
	writeFile(t, original, "set -g mouse on\n")
	if _, err := mgr.Backup(original, "c", "tmux"); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	backups, err := mgr.ListBackups(original)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 1 {
		t.Errorf("expected 1 backup entry, got %d", len(backups))
	}
	if backups[0].DotfileID != "tmux" {
		t.Errorf("unexpected DotfileID: %q", backups[0].DotfileID)
	}
}
