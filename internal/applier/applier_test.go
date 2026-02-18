package applier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/milxzy/dotfile-picker/internal/backup"
	"github.com/milxzy/dotfile-picker/internal/manifest"
)

// setupApplier creates a fresh Applier and backup manager in temp dirs.
func setupApplier(t *testing.T) (*Applier, string) {
	t.Helper()
	dir := t.TempDir()
	mgr := backup.NewManager(filepath.Join(dir, "backups"))

	a := &Applier{
		backupManager: mgr,
		homeDir:       dir,
	}
	return a, dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

var (
	fakeCreator = &manifest.Creator{ID: "testcreator"}
	fakeDotfile = &manifest.Dotfile{ID: "testdotfile"}
)

func TestApply_NewFile(t *testing.T) {
	a, dir := setupApplier(t)

	src := filepath.Join(dir, "source", "init.lua")
	writeFile(t, src, "-- config\n")

	result := a.Apply(src, ".config/nvim/init.lua", fakeCreator, fakeDotfile)

	if !result.Success {
		t.Fatalf("Apply failed: %v", result.Error)
	}
	if result.BackupPath != "" {
		t.Error("expected no backup for a new file")
	}

	target := filepath.Join(dir, ".config/nvim/init.lua")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "-- config\n" {
		t.Errorf("unexpected target content: %q", string(data))
	}
}

func TestApply_OverwriteWithBackup(t *testing.T) {
	a, dir := setupApplier(t)

	// pre-existing target file
	target := filepath.Join(dir, ".vimrc")
	writeFile(t, target, "old content\n")

	src := filepath.Join(dir, "source", ".vimrc")
	writeFile(t, src, "new content\n")

	result := a.Apply(src, ".vimrc", fakeCreator, fakeDotfile)

	if !result.Success {
		t.Fatalf("Apply failed: %v", result.Error)
	}
	if result.BackupPath == "" {
		t.Error("expected a backup path for overwritten file")
	}

	// verify target was updated
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "new content\n" {
		t.Errorf("unexpected target content: %q", string(data))
	}

	// verify backup preserved original
	backupData, err := os.ReadFile(result.BackupPath)
	if err != nil {
		t.Fatalf("ReadFile backup: %v", err)
	}
	if string(backupData) != "old content\n" {
		t.Errorf("unexpected backup content: %q", string(backupData))
	}
}

func TestApply_MissingSource(t *testing.T) {
	a, dir := setupApplier(t)
	result := a.Apply(filepath.Join(dir, "missing"), ".config/missing", fakeCreator, fakeDotfile)
	if result.Success {
		t.Error("expected failure for missing source file")
	}
	if result.Error == nil {
		t.Error("expected non-nil error")
	}
}

func TestResolveTargetPath(t *testing.T) {
	a := &Applier{homeDir: "/home/user"}

	cases := []struct {
		input    string
		expected string
	}{
		{"~/.vimrc", "/home/user/.vimrc"},
		{".vimrc", "/home/user/.vimrc"},
		{".config/nvim/init.lua", "/home/user/.config/nvim/init.lua"},
		{"/etc/hosts", "/etc/hosts"},
		{"relative/path", "/home/user/relative/path"},
	}

	for _, tc := range cases {
		got := a.ResolveTargetPath(tc.input, a.homeDir)
		if got != tc.expected {
			t.Errorf("ResolveTargetPath(%q): got %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestApplyMultiple(t *testing.T) {
	a, dir := setupApplier(t)

	src1 := filepath.Join(dir, "src", "init.lua")
	src2 := filepath.Join(dir, "src", ".zshrc")
	writeFile(t, src1, "nvim\n")
	writeFile(t, src2, "zsh\n")

	files := map[string]string{
		src1: ".config/nvim/init.lua",
		src2: ".zshrc",
	}

	results := a.ApplyMultiple(files, fakeCreator, fakeDotfile)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("Apply failed for %s: %v", r.TargetPath, r.Error)
		}
	}
}

func TestRollback(t *testing.T) {
	a, dir := setupApplier(t)

	// put an existing file so a backup is created
	original := filepath.Join(dir, ".bashrc")
	writeFile(t, original, "original\n")

	src := filepath.Join(dir, "src", ".bashrc")
	writeFile(t, src, "new\n")

	result := a.Apply(src, ".bashrc", fakeCreator, fakeDotfile)
	if !result.Success {
		t.Fatalf("Apply: %v", result.Error)
	}

	// overwrite to confirm something changed
	data, _ := os.ReadFile(original)
	if string(data) != "new\n" {
		t.Fatalf("pre-rollback content wrong: %q", string(data))
	}

	// rollback
	if err := a.Rollback([]*ApplyResult{result}); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	data, err := os.ReadFile(original)
	if err != nil {
		t.Fatalf("ReadFile after rollback: %v", err)
	}
	if string(data) != "original\n" {
		t.Errorf("post-rollback content wrong: %q", string(data))
	}
}
