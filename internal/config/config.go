// package config handles app-wide configuration
// provides sane defaults but allows user customization
package config

import (
	"os"
	"path/filepath"
	"time"
)

// Config holds all application settings
type Config struct {
	// ManifestURL is where we fetch the creator registry
	ManifestURL string

	// CacheDir is where we store downloaded repos
	CacheDir string

	// BackupDir is where we store config backups
	BackupDir string

	// ConfigDir is the base config directory
	ConfigDir string

	// ManifestCachePath is where we cache the manifest json
	ManifestCachePath string

	// RefreshInterval is how often to check for manifest updates
	RefreshInterval time.Duration

	// LogDir is where we write debug logs
	LogDir string

	// DotfilesRoot is where XDG config directories go (default: ~/.config)
	DotfilesRoot string

	// AutoXDGDetection enables smart detection of XDG config directories
	// When true, paths like "nvim" → "$DotfilesRoot/nvim"
	// When false, paths are used as-is: "nvim" → "~/nvim"
	AutoXDGDetection bool

	// XDGDirectories is the list of directory names to auto-detect
	// Only used when AutoXDGDetection is true
	XDGDirectories []string
}

// Default returns a config with sane defaults
// uses standard xdg directories
func Default() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	// use xdg config dir if available, otherwise ~/.config
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(homeDir, ".config")
	}

	baseDir := filepath.Join(configHome, "dotfile-picker")

	return &Config{
		ManifestURL:       "https://raw.githubusercontent.com/milxzy/dotfile-registry/main/manifest.json",
		CacheDir:          filepath.Join(baseDir, "cache"),
		BackupDir:         filepath.Join(baseDir, "backups"),
		ConfigDir:         baseDir,
		ManifestCachePath: filepath.Join(baseDir, "manifest.json"),
		RefreshInterval:   7 * 24 * time.Hour, // weekly refresh
		LogDir:            filepath.Join(baseDir, "logs"),
		DotfilesRoot:      configHome, // defaults to ~/.config or $XDG_CONFIG_HOME
		AutoXDGDetection:  true,       // enabled by default for smart behavior
		XDGDirectories:    defaultXDGDirs(),
	}, nil
}

// defaultXDGDirs returns the default list of known XDG config directories
// These directories will be automatically placed in DotfilesRoot when AutoXDGDetection is enabled
func defaultXDGDirs() []string {
	return []string{
		"nvim", "vim", // editors
		"kitty", "alacritty", // terminals
		"tmux",        // multiplexer (when in dir form)
		"fish", "zsh", // shells (when in dir form)
		"git",      // vcs
		"starship", // prompt
		"helix",    // editor
		"wezterm",  // terminal
	}
}

// EnsureDirectories creates all necessary directories
// safe to call multiple times
func (c *Config) EnsureDirectories() error {
	dirs := []string{
		c.CacheDir,
		c.BackupDir,
		c.ConfigDir,
		c.LogDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

// CreatorCacheDir returns the cache directory for a specific creator
func (c *Config) CreatorCacheDir(creatorID string) string {
	return filepath.Join(c.CacheDir, creatorID)
}
