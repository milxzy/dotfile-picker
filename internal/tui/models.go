// package tui defines shared types and messages for the TUI
package tui

import (
	"github.com/milxzy/dot-generator/internal/applier"
	"github.com/milxzy/dot-generator/internal/diff"
	"github.com/milxzy/dot-generator/internal/manifest"
)

// Screen represents different views in the app
type Screen int

const (
	ScreenLoading Screen = iota
	ScreenCategory
	ScreenCreator
	ScreenDotfile
	ScreenDownloading
	ScreenTreeConfirm
	ScreenDiff
	ScreenComplete
	ScreenError
	ScreenDependencyCheck
	ScreenPluginManagerDetect
	ScreenDirectoryBrowser
)

// messages for bubble tea
type (
	// manifestLoadedMsg is sent when the manifest is loaded
	manifestLoadedMsg struct {
		manifest *manifest.Manifest
	}

	// errorMsg is sent when an error occurs
	errorMsg struct {
		err error
	}

	// categorySelectedMsg is sent when user selects a category
	categorySelectedMsg struct {
		category *manifest.Category
	}

	// creatorSelectedMsg is sent when user selects a creator
	creatorSelectedMsg struct {
		creator *manifest.Creator
	}

	// dotfilesSelectedMsg is sent when user selects dotfiles to apply
	dotfilesSelectedMsg struct {
		dotfiles []*manifest.Dotfile
	}

	// repoDownloadedMsg is sent when a repo finishes downloading
	repoDownloadedMsg struct {
		creatorID string
	}

	// filesResolvedMsg is sent when file paths are resolved
	filesResolvedMsg struct {
		structure manifest.RepoStructure
		fileMap   map[string]string
	}

	// diffGeneratedMsg is sent when diffs are generated
	diffGeneratedMsg struct {
		result []*diff.Result
	}

	// applyCompleteMsg is sent when files are applied
	applyCompleteMsg struct {
		results []*applier.ApplyResult
	}

	// dependenciesCheckedMsg sent when dependency check completes
	dependenciesCheckedMsg struct {
		results []interface{} // will be []deps.CheckResult but avoiding import cycle
	}

	// dependenciesInstalledMsg sent when dependencies are installed
	dependenciesInstalledMsg struct {
		success bool
	}

	// pluginManagerDetectedMsg sent when plugin manager is found
	pluginManagerDetectedMsg struct {
		manager interface{} // will be *deps.NvimPluginManager
	}

	// pluginManagerInstalledMsg sent when plugin manager is installed
	pluginManagerInstalledMsg struct {
		success bool
	}

	// pathNotFoundMsg sent when auto-detection fails and browser should be shown
	pathNotFoundMsg struct {
		requestedPath string
		repoPath      string
	}

	// directorySelectedMsg sent when user selects a directory from browser
	directorySelectedMsg struct {
		selectedPath string
	}

	// rootSelectedMsg sent when user selects the root directory for their platform
	rootSelectedMsg struct {
		rootPath string
	}
)
