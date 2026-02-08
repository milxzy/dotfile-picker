// package tui contains workflow tests
package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/milxzy/dot-generator/internal/applier"
	"github.com/milxzy/dot-generator/internal/backup"
	"github.com/milxzy/dot-generator/internal/cache"
	"github.com/milxzy/dot-generator/internal/config"
	"github.com/milxzy/dot-generator/internal/manifest"
)

// TestWorkflowComponents tests that all workflow components integrate properly
func TestWorkflowComponents(t *testing.T) {
	// create temp directories for testing
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	backupDir := filepath.Join(tmpDir, "backups")
	configDir := filepath.Join(tmpDir, "config")
	logDir := filepath.Join(tmpDir, "logs")

	// create test config
	cfg := &config.Config{
		CacheDir:  cacheDir,
		BackupDir: backupDir,
		ConfigDir: configDir,
		LogDir:    logDir,
	}

	if err := cfg.EnsureDirectories(); err != nil {
		t.Fatalf("couldn't create directories: %v", err)
	}

	// create managers
	cacheManager := cache.NewManager(cacheDir)
	backupManager := backup.NewManager(backupDir)
	applierInstance, err := applier.NewApplier(backupManager)
	if err != nil {
		t.Fatalf("couldn't create applier: %v", err)
	}

	// test that we can resolve a simple file path
	homeDir, _ := os.UserHomeDir()
	targetPath := applierInstance.ResolveTargetPath(".bashrc", homeDir)
	expectedPath := filepath.Join(homeDir, ".bashrc")

	if targetPath != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, targetPath)
	}

	// test that structure detection doesn't panic
	structure := manifest.DetectStructure(tmpDir)
	if structure != manifest.StructureUnknown {
		t.Logf("detected structure: %v (expected unknown for empty dir)", structure)
	}

	// verify cache manager is properly initialized
	if cacheManager == nil {
		t.Error("cache manager is nil")
	}

	t.Log("workflow components integrated successfully")
}

// TestManifestLoading tests that the manifest can be loaded
func TestManifestLoading(t *testing.T) {
	// try to load from local file first
	manifestPath := "../../configs/manifest.json"
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Skipf("skipping test, local manifest not found: %v", err)
	}

	var m manifest.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("couldn't parse manifest: %v", err)
	}

	if len(m.Creators) == 0 {
		t.Error("manifest has no creators")
	}

	if len(m.Categories) == 0 {
		t.Error("manifest has no categories")
	}

	t.Logf("loaded %d creators in %d categories", len(m.Creators), len(m.Categories))
}
