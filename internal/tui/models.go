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
	ScreenDiff
	ScreenComplete
	ScreenError
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

	// diffGeneratedMsg is sent when a diff is generated
	diffGeneratedMsg struct {
		result *diff.Result
	}

	// applyCompleteMsg is sent when files are applied
	applyCompleteMsg struct {
		results []*applier.ApplyResult
	}
)
