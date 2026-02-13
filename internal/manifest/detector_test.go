// package manifest contains tests for path detection
package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPathExists tests the pathExists helper function
func TestPathExists(t *testing.T) {
	// Create a temp directory
	tmpDir := t.TempDir()

	// Create a file
	filePath := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("couldn't create test file: %v", err)
	}

	// Create a directory
	dirPath := filepath.Join(tmpDir, "testdir")
	if err := os.Mkdir(dirPath, 0755); err != nil {
		t.Fatalf("couldn't create test directory: %v", err)
	}

	// Test file exists
	if !pathExists(filePath) {
		t.Error("pathExists returned false for existing file")
	}

	// Test directory exists
	if !pathExists(dirPath) {
		t.Error("pathExists returned false for existing directory")
	}

	// Test non-existent path
	if pathExists(filepath.Join(tmpDir, "nonexistent")) {
		t.Error("pathExists returned true for non-existent path")
	}
}

// TestResolveFilePathWithDirectory tests that directories are found
func TestResolveFilePathWithDirectory(t *testing.T) {
	// Create a Stow-style repo structure
	tmpDir := t.TempDir()

	// Create multiple package directories (Stow needs >= 2 to be detected)
	// Package 1: nvim/.config/nvim
	nvimPkgDir := filepath.Join(tmpDir, "nvim", ".config", "nvim")
	if err := os.MkdirAll(nvimPkgDir, 0755); err != nil {
		t.Fatalf("couldn't create nvim directory: %v", err)
	}

	// Create a config file inside
	configFile := filepath.Join(nvimPkgDir, "init.lua")
	if err := os.WriteFile(configFile, []byte("-- test"), 0644); err != nil {
		t.Fatalf("couldn't create init.lua: %v", err)
	}

	// Package 2: tmux/.tmux.conf (to meet Stow detection threshold)
	tmuxPkgDir := filepath.Join(tmpDir, "tmux")
	if err := os.MkdirAll(tmuxPkgDir, 0755); err != nil {
		t.Fatalf("couldn't create tmux directory: %v", err)
	}

	tmuxFile := filepath.Join(tmuxPkgDir, ".tmux.conf")
	if err := os.WriteFile(tmuxFile, []byte("# test"), 0644); err != nil {
		t.Fatalf("couldn't create .tmux.conf: %v", err)
	}

	// Detect structure (should be Stow)
	structure := DetectStructure(tmpDir)
	if structure != StructureStow {
		t.Logf("Warning: Expected StructureStow, got %v", structure)
	}

	// Try to resolve .config/nvim (a directory)
	resolved, found := ResolveFilePath(tmpDir, ".config/nvim", structure)

	if !found {
		t.Errorf("ResolveFilePath didn't find .config/nvim directory in Stow layout")
	}

	expectedPath := nvimPkgDir
	if resolved != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, resolved)
	}

	// Verify it's actually a directory
	info, err := os.Stat(resolved)
	if err != nil {
		t.Fatalf("couldn't stat resolved path: %v", err)
	}

	if !info.IsDir() {
		t.Error("Resolved path is not a directory")
	}
}

// TestResolveFilePathWithFile tests that files are still found
func TestResolveFilePathWithFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file at root (flat structure)
	tmuxPath := filepath.Join(tmpDir, ".tmux.conf")
	if err := os.WriteFile(tmuxPath, []byte("# test"), 0644); err != nil {
		t.Fatalf("couldn't create .tmux.conf: %v", err)
	}

	structure := DetectStructure(tmpDir)

	resolved, found := ResolveFilePath(tmpDir, ".tmux.conf", structure)

	if !found {
		t.Error("ResolveFilePath didn't find .tmux.conf file")
	}

	if resolved != tmuxPath {
		t.Errorf("Expected path %s, got %s", tmuxPath, resolved)
	}
}

// TestResolveFilePathWithAliases tests path alias resolution (xdg_config, home, etc)
func TestResolveFilePathWithAliases(t *testing.T) {
	tmpDir := t.TempDir()

	// Test case 1: xdg_config as alias for .config
	xdgNvimDir := filepath.Join(tmpDir, "xdg_config", "nvim")
	if err := os.MkdirAll(xdgNvimDir, 0755); err != nil {
		t.Fatalf("couldn't create xdg_config/nvim: %v", err)
	}

	// Create a file to make it valid
	if err := os.WriteFile(filepath.Join(xdgNvimDir, "init.lua"), []byte("test"), 0644); err != nil {
		t.Fatalf("couldn't create init.lua: %v", err)
	}

	// Try to resolve .config/nvim (should find xdg_config/nvim)
	resolved, found := ResolveFilePath(tmpDir, ".config/nvim", StructureUnknown)
	if !found {
		t.Error("ResolveFilePath didn't find .config/nvim via xdg_config alias")
	}

	if resolved != xdgNvimDir {
		t.Errorf("Expected path %s, got %s", xdgNvimDir, resolved)
	}

	// Test case 2: home as alias for ~
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.Mkdir(homeDir, 0755); err != nil {
		t.Fatalf("couldn't create home directory: %v", err)
	}

	bashrcPath := filepath.Join(homeDir, ".bashrc")
	if err := os.WriteFile(bashrcPath, []byte("test"), 0644); err != nil {
		t.Fatalf("couldn't create .bashrc: %v", err)
	}

	// Try to resolve ~/.bashrc (should find home/.bashrc)
	resolved, found = ResolveFilePath(tmpDir, "~/.bashrc", StructureUnknown)
	if !found {
		t.Error("ResolveFilePath didn't find ~/.bashrc via home alias")
	}

	if resolved != bashrcPath {
		t.Errorf("Expected path %s, got %s", bashrcPath, resolved)
	}

	// Test case 3: config as another alias for .config
	configTmuxDir := filepath.Join(tmpDir, "config", "tmux")
	if err := os.MkdirAll(configTmuxDir, 0755); err != nil {
		t.Fatalf("couldn't create config/tmux: %v", err)
	}

	if err := os.WriteFile(filepath.Join(configTmuxDir, "tmux.conf"), []byte("test"), 0644); err != nil {
		t.Fatalf("couldn't create tmux.conf: %v", err)
	}

	resolved, found = ResolveFilePath(tmpDir, ".config/tmux", StructureUnknown)
	if !found {
		t.Error("ResolveFilePath didn't find .config/tmux via config alias")
	}

	if resolved != configTmuxDir {
		t.Errorf("Expected path %s, got %s", configTmuxDir, resolved)
	}
}

// TestResolveFilePathWithEmptyDirectory tests that empty directories are still found
// (the caller will need to check if they contain files)
func TestResolveFilePathWithEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an empty xdg_config/nvim directory (like an uninitialized submodule)
	emptyNvimDir := filepath.Join(tmpDir, "xdg_config", "nvim")
	if err := os.MkdirAll(emptyNvimDir, 0755); err != nil {
		t.Fatalf("couldn't create empty nvim directory: %v", err)
	}

	// Try to resolve .config/nvim (should find the empty xdg_config/nvim via alias)
	resolved, found := ResolveFilePath(tmpDir, ".config/nvim", StructureUnknown)

	// Should find the directory even if it's empty
	if !found {
		t.Error("ResolveFilePath didn't find .config/nvim via xdg_config alias (empty dir)")
	}

	if resolved != emptyNvimDir {
		t.Errorf("Expected path %s, got %s", emptyNvimDir, resolved)
	}

	// Verify it's actually a directory
	info, err := os.Stat(resolved)
	if err != nil {
		t.Fatalf("couldn't stat resolved path: %v", err)
	}

	if !info.IsDir() {
		t.Error("Resolved path is not a directory")
	}

	// Check that the directory is empty
	entries, err := os.ReadDir(resolved)
	if err != nil {
		t.Fatalf("couldn't read directory: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("Expected empty directory, but found %d entries", len(entries))
	}
}
