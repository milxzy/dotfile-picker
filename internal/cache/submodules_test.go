package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConvertSSHToHTTPS(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "git@github.com:ThePrimeagen/init.lua.git",
			expected: "https://github.com/ThePrimeagen/init.lua",
		},
		{
			input:    "git@github.com:user/repo.git",
			expected: "https://github.com/user/repo",
		},
		{
			input:    "https://github.com/user/repo",
			expected: "https://github.com/user/repo",
		},
		{
			input:    "https://github.com/user/repo.git",
			expected: "https://github.com/user/repo.git", // Already HTTPS
		},
	}

	for _, tt := range tests {
		result := ConvertSSHToHTTPS(tt.input)
		if result != tt.expected {
			t.Errorf("ConvertSSHToHTTPS(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestParseGitmodules(t *testing.T) {
	// Create a temporary .gitmodules file
	tmpDir := t.TempDir()
	gitmodulesPath := filepath.Join(tmpDir, ".gitmodules")

	content := `[submodule "nvim/.config/nvim"]
	path = nvim/.config/nvim
	url = git@github.com:ThePrimeagen/init.lua.git
[submodule "personal"]
	path = personal
	url = git@github.com:ThePrimeagen/.dotfiles-personal.git
`

	err := os.WriteFile(gitmodulesPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test .gitmodules: %v", err)
	}

	// Parse the file
	submodules, err := ParseGitmodules(tmpDir)
	if err != nil {
		t.Fatalf("ParseGitmodules failed: %v", err)
	}

	// Verify results
	if len(submodules) != 2 {
		t.Errorf("Expected 2 submodules, got %d", len(submodules))
	}

	// Check first submodule
	if submodules[0].Path != "nvim/.config/nvim" {
		t.Errorf("Expected path 'nvim/.config/nvim', got %q", submodules[0].Path)
	}
	if submodules[0].URL != "git@github.com:ThePrimeagen/init.lua.git" {
		t.Errorf("Expected URL 'git@github.com:ThePrimeagen/init.lua.git', got %q", submodules[0].URL)
	}

	// Check second submodule
	if submodules[1].Path != "personal" {
		t.Errorf("Expected path 'personal', got %q", submodules[1].Path)
	}
}

func TestParseGitmodulesNoFile(t *testing.T) {
	tmpDir := t.TempDir()

	submodules, err := ParseGitmodules(tmpDir)
	if err != nil {
		t.Errorf("Expected no error when .gitmodules doesn't exist, got: %v", err)
	}
	if submodules != nil {
		t.Errorf("Expected nil submodules when .gitmodules doesn't exist, got %v", submodules)
	}
}

func TestFindSubmoduleByPath(t *testing.T) {
	submodules := []SubmoduleConfig{
		{Path: "nvim/.config/nvim", URL: "git@github.com:user/nvim.git"},
		{Path: "tmux", URL: "git@github.com:user/tmux.git"},
	}

	// Test found
	found := FindSubmoduleByPath(submodules, "nvim/.config/nvim")
	if found == nil {
		t.Error("Expected to find submodule, got nil")
	} else if found.URL != "git@github.com:user/nvim.git" {
		t.Errorf("Expected URL 'git@github.com:user/nvim.git', got %q", found.URL)
	}

	// Test not found
	notFound := FindSubmoduleByPath(submodules, "zsh")
	if notFound != nil {
		t.Errorf("Expected nil for non-existent submodule, got %v", notFound)
	}
}

func TestIsEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Test empty directory
	emptyDir := filepath.Join(tmpDir, "empty")
	os.Mkdir(emptyDir, 0755)

	isEmpty, err := IsEmptyDirectory(emptyDir)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !isEmpty {
		t.Error("Expected empty directory to return true")
	}

	// Test directory with files
	nonEmptyDir := filepath.Join(tmpDir, "nonempty")
	os.Mkdir(nonEmptyDir, 0755)
	os.WriteFile(filepath.Join(nonEmptyDir, "file.txt"), []byte("test"), 0644)

	isEmpty, err = IsEmptyDirectory(nonEmptyDir)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if isEmpty {
		t.Error("Expected non-empty directory to return false")
	}

	// Test directory with only .git file (submodule placeholder)
	gitOnlyDir := filepath.Join(tmpDir, "gitonly")
	os.Mkdir(gitOnlyDir, 0755)
	os.WriteFile(filepath.Join(gitOnlyDir, ".git"), []byte("gitdir: ../.git/modules/something"), 0644)

	isEmpty, err = IsEmptyDirectory(gitOnlyDir)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !isEmpty {
		t.Error("Expected directory with only .git file to be considered empty")
	}
}
