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
	}, nil
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
