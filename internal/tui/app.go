// package tui provides the terminal user interface
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/milxzy/dot-generator/internal/applier"
	"github.com/milxzy/dot-generator/internal/backup"
	"github.com/milxzy/dot-generator/internal/cache"
	"github.com/milxzy/dot-generator/internal/config"
	"github.com/milxzy/dot-generator/internal/deps"
	"github.com/milxzy/dot-generator/internal/diff"
	"github.com/milxzy/dot-generator/internal/manifest"
)

// Model represents the entire app state
type Model struct {
	// current screen
	screen Screen

	// configuration and services
	cfg      *config.Config
	manifest *manifest.Manifest
	fetcher  *manifest.Fetcher
	cache    *cache.Manager
	backup   *backup.Manager
	applier  *applier.Applier

	// ui state
	categoryList list.Model
	creatorList  list.Model
	dotfileList  list.Model
	spinner      spinner.Model
	dirBrowser   *DirBrowser

	// selections
	selectedCategory *manifest.Category
	selectedCreator  *manifest.Creator
	selectedDotfile  *manifest.Dotfile

	// workflow state
	repoStructure manifest.RepoStructure
	fileMap       map[string]string // source path -> target path
	diffResults   []*diff.Result

	// dependency checking
	depChecker    *deps.Checker
	depResults    []deps.CheckResult
	pluginManager *deps.NvimPluginManager

	// status
	width, height int
	err           error
	statusMsg     string
}

// listItem implements list.Item for categories/creators
type listItem struct {
	title       string
	description string
	data        interface{}
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return i.description }
func (i listItem) FilterValue() string { return i.title }

// New creates a new TUI model
func New(ctx context.Context) (*Model, error) {
	// load config
	cfg, err := config.Default()
	if err != nil {
		return nil, err
	}

	if err := cfg.EnsureDirectories(); err != nil {
		return nil, err
	}

	// create services
	fetcher := manifest.NewFetcher(cfg.ManifestURL, cfg.ManifestCachePath)
	cacheManager := cache.NewManager(cfg.CacheDir)
	backupManager := backup.NewManager(cfg.BackupDir)
	applierInstance, err := applier.NewApplier(backupManager)
	if err != nil {
		return nil, err
	}

	// create spinner for loading
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	// create dependency checker (optional - won't fail if not available)
	depChecker, err := deps.NewChecker()
	if err != nil {
		// log warning but don't fail - dependency checking is optional
		depChecker = nil
	}

	return &Model{
		screen:     ScreenLoading,
		cfg:        cfg,
		fetcher:    fetcher,
		cache:      cacheManager,
		backup:     backupManager,
		applier:    applierInstance,
		spinner:    s,
		depChecker: depChecker,
	}, nil
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.fetchManifest,
	)
}

// Update handles messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Handle screen-specific keys first
		if m.screen == ScreenDependencyCheck {
			switch msg.String() {
			case "i", "I":
				// install missing dependencies
				m.screen = ScreenDownloading
				m.statusMsg = "installing dependencies"
				return m, tea.Batch(m.spinner.Tick, m.installMissingDependencies)
			case "s", "S":
				// skip and continue to structure detection
				m.screen = ScreenDownloading
				m.statusMsg = "resolving file paths"
				return m, tea.Batch(m.spinner.Tick, m.detectStructure)
			case "enter":
				// all installed, continue to structure detection
				m.screen = ScreenDownloading
				m.statusMsg = "resolving file paths"
				return m, tea.Batch(m.spinner.Tick, m.detectStructure)
			}
		}

		if m.screen == ScreenPluginManagerDetect {
			switch msg.String() {
			case "y", "Y":
				m.screen = ScreenDownloading
				m.statusMsg = "installing plugin manager"
				return m, tea.Batch(m.spinner.Tick, m.installPluginManager)
			case "n", "N":
				// skip plugin manager, continue to diffs
				return m, m.generateDiffs
			}
		}

		if m.screen == ScreenDirectoryBrowser {
			switch msg.String() {
			case "c", "C":
				// Confirm current directory selection (for individual dotfile)
				selectedPath := m.dirBrowser.GetCurrentPath()
				return m, func() tea.Msg {
					return directorySelectedMsg{selectedPath: selectedPath}
				}
			case "esc":
				// Cancel and go back to dotfile selection
				m.screen = ScreenDotfile
				return m, nil
			}
		}

		// Error screen handling removed - ESC navigation handles going back

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			// go back to previous screen
			switch m.screen {
			case ScreenCreator:
				m.screen = ScreenCategory
			case ScreenDotfile:
				m.screen = ScreenCreator
			case ScreenTreeConfirm:
				// Cancel and go back to dotfile selection
				m.screen = ScreenDotfile
			case ScreenDependencyCheck:
				m.screen = ScreenDotfile
			case ScreenPluginManagerDetect:
				m.screen = ScreenTreeConfirm
			case ScreenDirectoryBrowser:
				m.screen = ScreenDotfile
			case ScreenDiff:
				m.screen = ScreenTreeConfirm
			case ScreenComplete:
				m.screen = ScreenCategory
			case ScreenError:
				// try to go back to a safe screen
				if m.selectedCreator != nil {
					m.screen = ScreenDotfile
				} else if m.selectedCategory != nil {
					m.screen = ScreenCreator
				} else {
					m.screen = ScreenCategory
				}
			default:
				// from category screen or unknown, quit
				return m, tea.Quit
			}
			return m, nil
		case "enter":
			return m.handleEnter()
		}

	case manifestLoadedMsg:
		m.manifest = msg.manifest
		m.screen = ScreenCategory
		m.buildCategoryList()
		return m, nil

	case repoDownloadedMsg:
		// repo downloaded, proceed to dependency check or structure detection
		// (submodules are skipped - modern plugin managers auto-install)
		if m.depChecker != nil && len(m.selectedDotfile.Dependencies) > 0 {
			return m, m.checkDependencies
		}
		// No dependencies, proceed to structure detection
		m.screen = ScreenDownloading
		m.statusMsg = "resolving file paths"
		return m, tea.Batch(m.spinner.Tick, m.detectStructure)

	case dependenciesCheckedMsg:
		// convert back from interface{}
		m.depResults = make([]deps.CheckResult, len(msg.results))
		for i, r := range msg.results {
			if result, ok := r.(deps.CheckResult); ok {
				m.depResults[i] = result
			}
		}
		m.screen = ScreenDependencyCheck
		return m, nil

	case dependenciesInstalledMsg:
		if msg.success {
			// dependencies installed, proceed to structure detection
			m.screen = ScreenDownloading
			m.statusMsg = "resolving file paths"
			return m, tea.Batch(m.spinner.Tick, m.detectStructure)
		}
		// installation failed, stay on dependency screen
		return m, nil

	case pluginManagerDetectedMsg:
		if pm, ok := msg.manager.(*deps.NvimPluginManager); ok {
			m.pluginManager = pm

			// Check if it's installed
			if deps.IsPluginManagerInstalled(pm) {
				// Already installed, continue to diffs
				return m, m.generateDiffs
			}

			// Not installed, show prompt
			m.screen = ScreenPluginManagerDetect
			return m, nil
		}
		// Couldn't convert, skip
		return m, m.generateDiffs

	case pluginManagerInstalledMsg:
		if msg.success {
			// plugin manager installed, continue to diffs
			return m, m.generateDiffs
		}
		// installation failed, but continue anyway
		return m, m.generateDiffs

	case filesResolvedMsg:
		// files resolved, save state
		m.repoStructure = msg.structure
		m.fileMap = msg.fileMap

		// Show tree confirmation view so user can see what will be applied
		m.screen = ScreenTreeConfirm
		return m, nil

	case pathNotFoundMsg:
		// Auto-detection failed, show directory browser
		m.screen = ScreenDirectoryBrowser
		m.dirBrowser = NewDirBrowser(msg.repoPath, msg.requestedPath, m.width, m.height)
		return m, nil

	case directorySelectedMsg:
		// User selected a directory for a specific dotfile, resolve it and continue
		return m, m.resolveSelectedDirectory(msg.selectedPath)

	case diffGeneratedMsg:
		// diffs generated, show them to user
		m.diffResults = msg.result
		m.screen = ScreenDiff
		return m, nil

	case applyCompleteMsg:
		// files applied successfully
		m.screen = ScreenComplete
		return m, nil

	case errorMsg:
		m.err = msg.err
		m.screen = ScreenError
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	// update current list or browser
	var cmd tea.Cmd
	switch m.screen {
	case ScreenCategory:
		m.categoryList, cmd = m.categoryList.Update(msg)
	case ScreenCreator:
		m.creatorList, cmd = m.creatorList.Update(msg)
	case ScreenDotfile:
		m.dotfileList, cmd = m.dotfileList.Update(msg)
	case ScreenDirectoryBrowser:
		if m.dirBrowser != nil {
			m.dirBrowser, cmd = m.dirBrowser.Update(msg)
		}
	}

	return m, cmd
}

// View renders the UI
func (m *Model) View() string {
	switch m.screen {
	case ScreenLoading:
		return m.viewLoading()
	case ScreenCategory:
		return m.viewCategories()
	case ScreenCreator:
		return m.viewCreators()
	case ScreenDotfile:
		return m.viewDotfiles()
	case ScreenDownloading:
		return m.viewDownloading()
	case ScreenTreeConfirm:
		return m.viewTreeConfirm()
	case ScreenDiff:
		return m.viewDiff()
	case ScreenDependencyCheck:
		return m.viewDependencyCheck()
	case ScreenPluginManagerDetect:
		return m.viewPluginManagerDetect()
	case ScreenDirectoryBrowser:
		return m.viewDirectoryBrowser()
	case ScreenComplete:
		return m.viewComplete()
	case ScreenError:
		return m.viewError()
	default:
		return "unknown screen"
	}
}

// viewLoading shows the loading screen
func (m *Model) viewLoading() string {
	return fmt.Sprintf("\n\n   %s loading manifest...\n\n", m.spinner.View())
}

// viewCategories shows the category selection
func (m *Model) viewCategories() string {
	var b strings.Builder

	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n\n")
	b.WriteString(formatSubtitle("select a category"))
	b.WriteString("\n\n")
	b.WriteString(m.categoryList.View())
	b.WriteString("\n\n")
	b.WriteString(formatHelp("enter: select • q: quit"))

	return b.String()
}

// viewCreators shows creators in selected category
func (m *Model) viewCreators() string {
	var b strings.Builder

	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n")
	b.WriteString(formatSubtitle(m.selectedCategory.Name))
	b.WriteString("\n\n")
	b.WriteString(m.creatorList.View())
	b.WriteString("\n\n")
	b.WriteString(formatHelp("enter: select • esc: back • q: quit"))

	return b.String()
}

// viewDotfiles shows dotfiles from selected creator
func (m *Model) viewDotfiles() string {
	var b strings.Builder

	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n")
	b.WriteString(formatSubtitle(m.selectedCreator.Name + "'s dotfiles"))
	b.WriteString("\n\n")
	b.WriteString(m.dotfileList.View())
	b.WriteString("\n\n")
	b.WriteString(formatHelp("enter: apply • esc: back • q: quit"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("note: applying will create backups of your existing configs"))

	return b.String()
}

// viewTreeConfirm shows the tree structure of files that will be applied
func (m *Model) viewTreeConfirm() string {
	var b strings.Builder

	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n")
	b.WriteString(formatSubtitle(fmt.Sprintf("%s - %s", m.selectedCreator.Name, m.selectedDotfile.Name)))
	b.WriteString("\n\n")

	// Show detected structure type
	structureType := "unknown"
	switch m.repoStructure {
	case manifest.StructureFlat:
		structureType = "flat (dotfiles at root)"
	case manifest.StructureStow:
		structureType = "stow layout"
	case manifest.StructureChezmoi:
		structureType = "chezmoi"
	case manifest.StructureConfig:
		structureType = "config directory"
	case manifest.StructureBareRepo:
		structureType = "bare repository"
	}

	b.WriteString(fmt.Sprintf("📂 Detected structure: %s\n", structureType))
	b.WriteString(fmt.Sprintf("📝 Files to apply: %d\n\n", len(m.fileMap)))

	// Show file tree (limit to prevent overflow)
	maxFiles := 15
	count := 0
	b.WriteString("Files that will be installed:\n\n")

	for _, targetPath := range m.fileMap {
		if count >= maxFiles {
			remaining := len(m.fileMap) - maxFiles
			b.WriteString(mutedStyle.Render(fmt.Sprintf("\n... and %d more files\n", remaining)))
			break
		}

		// Expand tilde for display
		displayPath := targetPath
		if strings.HasPrefix(displayPath, "~") {
			homeDir, _ := os.UserHomeDir()
			displayPath = filepath.Join(homeDir, displayPath[1:])
		}

		b.WriteString(fmt.Sprintf("  • %s\n", displayPath))
		count++
	}

	b.WriteString("\n")
	b.WriteString(formatHelp("enter: confirm and continue • esc: cancel • q: quit"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("note: backups will be created before any files are modified"))

	return b.String()
}

// viewDownloading shows the downloading screen with dynamic status
func (m *Model) viewDownloading() string {
	var b strings.Builder

	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n\n")

	// Show the dynamic status message if available
	if m.statusMsg != "" {
		b.WriteString(fmt.Sprintf("   %s %s...\n\n", m.spinner.View(), m.statusMsg))
	} else {
		b.WriteString(fmt.Sprintf("   %s processing...\n\n", m.spinner.View()))
	}

	b.WriteString(mutedStyle.Render("   this may take a moment..."))

	return b.String()
}

// viewDiff shows the diff screen
func (m *Model) viewDiff() string {
	var b strings.Builder

	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n")
	b.WriteString(formatSubtitle(fmt.Sprintf("%s - %s", m.selectedCreator.Name, m.selectedDotfile.Name)))
	b.WriteString("\n\n")

	if len(m.diffResults) == 0 {
		b.WriteString(mutedStyle.Render("no changes to show"))
	} else {
		// show summary stats
		totalAdditions := 0
		totalDeletions := 0
		newFiles := 0
		modifiedFiles := 0

		for _, result := range m.diffResults {
			if result.IsNew {
				newFiles++
			} else if !result.IsIdentical {
				modifiedFiles++
				adds, dels := diff.GetDiffStats(result)
				totalAdditions += adds
				totalDeletions += dels
			}
		}

		// summary
		b.WriteString(fmt.Sprintf("Files: %d new, %d modified\n", newFiles, modifiedFiles))
		if totalAdditions > 0 || totalDeletions > 0 {
			b.WriteString(fmt.Sprintf("Changes: +%d additions, -%d deletions\n\n", totalAdditions, totalDeletions))
		} else {
			b.WriteString("\n")
		}

		// show first few diffs (limit to avoid huge output)
		maxToShow := 3
		shown := 0
		for _, result := range m.diffResults {
			if shown >= maxToShow {
				remaining := len(m.diffResults) - shown
				b.WriteString(mutedStyle.Render(fmt.Sprintf("\n... and %d more files", remaining)))
				break
			}

			if result.IsIdentical {
				continue // skip identical files
			}

			b.WriteString(formatSubtitle(fmt.Sprintf("→ %s", result.TargetPath)))
			b.WriteString("\n")

			if result.IsNew {
				b.WriteString(mutedStyle.Render("(new file)\n"))
			}

			// show diff (truncate if too long)
			diffLines := strings.Split(result.Diff, "\n")
			maxLines := 15
			if len(diffLines) > maxLines {
				for i := 0; i < maxLines; i++ {
					b.WriteString(diffLines[i])
					b.WriteString("\n")
				}
				b.WriteString(mutedStyle.Render(fmt.Sprintf("... (%d more lines)\n", len(diffLines)-maxLines)))
			} else {
				b.WriteString(result.Diff)
			}
			b.WriteString("\n")
			shown++
		}
	}

	b.WriteString("\n")
	b.WriteString(formatHelp("enter: apply with backups • esc: cancel • q: quit"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("note: your existing configs will be backed up before applying"))

	return b.String()
}

// viewDependencyCheck shows dependency status
func (m *Model) viewDependencyCheck() string {
	var b strings.Builder

	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n")
	b.WriteString(formatSubtitle("checking dependencies"))
	b.WriteString("\n\n")

	allInstalled := true
	missing := []deps.CheckResult{}

	for _, result := range m.depResults {
		if result.Installed {
			b.WriteString(fmt.Sprintf("  ✓ %s - installed", result.Dependency.Name))
			if result.Version != "" {
				b.WriteString(fmt.Sprintf(" (%s)", result.Version))
			}
			b.WriteString("\n")
		} else {
			b.WriteString(fmt.Sprintf("  ✗ %s - not found\n", result.Dependency.Name))
			allInstalled = false
			missing = append(missing, result)
		}
	}

	b.WriteString("\n")

	if !allInstalled {
		b.WriteString("Missing dependencies can be installed:\n\n")
		for _, result := range missing {
			b.WriteString(fmt.Sprintf("  %s\n", result.InstallCmd))
		}
		b.WriteString("\n")
		b.WriteString(formatHelp("i: install missing • s: skip and continue • esc: cancel"))
	} else {
		b.WriteString(formatSuccess("all dependencies installed!"))
		b.WriteString("\n\n")
		b.WriteString(formatHelp("enter: continue • esc: cancel"))
	}

	return b.String()
}

// viewPluginManagerDetect shows plugin manager installation prompt
func (m *Model) viewPluginManagerDetect() string {
	var b strings.Builder

	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("This neovim config uses %s for plugins.\n\n", m.pluginManager.Name))
	b.WriteString("Would you like to install it?\n")
	b.WriteString(fmt.Sprintf("Install location: %s\n\n", m.pluginManager.InstallPath))
	b.WriteString(mutedStyle.Render("After installing, you may need to run plugin installation commands.\n\n"))
	b.WriteString(formatHelp("y: install • n: skip • q: quit"))

	return b.String()
}

// Note: viewSubmoduleConfirm removed - submodules are now skipped entirely

// viewComplete shows the completion screen
func (m *Model) viewComplete() string {
	var b strings.Builder

	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n\n")
	b.WriteString(formatSuccess(fmt.Sprintf("successfully applied %s from %s!", m.selectedDotfile.Name, m.selectedCreator.Name)))
	b.WriteString("\n\n")

	// show what was done
	filesApplied := 0
	backupsCreated := 0

	b.WriteString("Applied files:\n")
	for _, result := range m.diffResults {
		if !result.IsIdentical {
			b.WriteString(fmt.Sprintf("  ✓ %s\n", result.TargetPath))
			filesApplied++

			// check if backup was created (look for corresponding apply result)
			// for now, just count non-new files as having backups
			if !result.IsNew {
				backupsCreated++
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Summary: %d files applied, %d backups created\n", filesApplied, backupsCreated))
	b.WriteString(fmt.Sprintf("Backups stored in: %s\n\n", m.cfg.BackupDir))

	b.WriteString(mutedStyle.Render("tip: backups are kept forever, you can manually restore if needed\n\n"))
	b.WriteString(formatHelp("esc: back to dotfiles • q: quit"))

	return b.String()
}

// viewError shows an error
func (m *Model) viewError() string {
	var b strings.Builder

	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n\n")

	// Handle nil error
	if m.err == nil {
		b.WriteString(formatError("An unknown error occurred"))
		b.WriteString("\n\n")
		b.WriteString(formatHelp("esc: back • q: quit"))
		return b.String()
	}

	errMsg := m.err.Error()
	b.WriteString(formatError(errMsg))
	b.WriteString("\n\n")
	b.WriteString(formatHelp("esc: back • q: quit"))

	return b.String()
}

// handleEnter processes enter key based on current screen
func (m *Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenCategory:
		// select category
		if item, ok := m.categoryList.SelectedItem().(listItem); ok {
			if cat, ok := item.data.(*manifest.Category); ok {
				m.selectedCategory = cat
				m.screen = ScreenCreator
				m.buildCreatorList()
			}
		}
	case ScreenCreator:
		// select creator - show dotfile list immediately (don't download yet)
		if item, ok := m.creatorList.SelectedItem().(listItem); ok {
			if creator, ok := item.data.(*manifest.Creator); ok {
				m.selectedCreator = creator
				m.screen = ScreenDotfile
				m.buildDotfileList()
			}
		}
	case ScreenDotfile:
		// select dotfile - now download repo and proceed
		if item, ok := m.dotfileList.SelectedItem().(listItem); ok {
			if dotfile, ok := item.data.(*manifest.Dotfile); ok {
				m.selectedDotfile = dotfile
				m.statusMsg = "downloading " + m.selectedCreator.Name + "'s dotfiles"

				// Download the repo
				m.screen = ScreenDownloading
				return m, tea.Batch(m.spinner.Tick, m.downloadRepo)
			}
		}
	case ScreenTreeConfirm:
		// User confirmed the tree view, proceed to plugin manager check or diffs
		if m.selectedDotfile.ID == "nvim" {
			return m, m.detectPluginManager
		}
		// Not nvim, go straight to diffs
		return m, m.generateDiffs
	case ScreenDiff:
		// user confirmed, apply the files
		m.screen = ScreenDownloading
		m.statusMsg = "applying files and creating backups"
		return m, tea.Batch(m.spinner.Tick, m.applyFiles)
	case ScreenDirectoryBrowser:
		// navigate into selected directory
		if m.dirBrowser != nil {
			if item, ok := m.dirBrowser.list.SelectedItem().(DirEntry); ok {
				if item.isDir {
					m.dirBrowser.LoadDirectory(item.path)
				}
			}
		}
	}
	return m, nil
}

// fetchManifest loads the manifest
// fetchManifest loads the manifest from the local configs directory
func (m *Model) fetchManifest() tea.Msg {
	// Use local manifest file for faster startup and offline support
	// The manifest is bundled with the binary in configs/manifest.json
	localManifestPath := filepath.Join("configs", "manifest.json")

	// Try to read the local manifest file
	manifest, err := m.loadLocalManifest(localManifestPath)
	if err != nil {
		// If local file doesn't exist, try the old remote fetch as fallback
		manifest, err = m.fetcher.Fetch(context.Background())
		if err != nil {
			return errorMsg{fmt.Errorf("couldn't load manifest: %w", err)}
		}
	}

	return manifestLoadedMsg{manifest}
}

// loadLocalManifest reads and parses the local manifest.json file
func (m *Model) loadLocalManifest(path string) (*manifest.Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var man manifest.Manifest
	if err := json.Unmarshal(data, &man); err != nil {
		return nil, fmt.Errorf("couldn't parse manifest: %w", err)
	}

	return &man, nil
}

// downloadRepo downloads the selected creator's repo
func (m *Model) downloadRepo() tea.Msg {
	ctx := context.Background()
	if err := m.cache.EnsureRepo(ctx, m.selectedCreator); err != nil {
		return errorMsg{err}
	}
	return repoDownloadedMsg{creatorID: m.selectedCreator.ID}
}

// Note: checkSubmodules and initSubmodules removed - submodules are now skipped entirely
// Modern plugin managers auto-install, and submodule failures rarely matter for dotfiles

// detectStructure detects the repo structure and resolves file paths
func (m *Model) detectStructure() tea.Msg {
	// Always use repo root - we auto-detect structure now
	searchPath := m.cache.GetRepoPath(m.selectedCreator.ID)
	structure := manifest.DetectStructure(searchPath)

	// resolve file paths
	fileMap := make(map[string]string)
	for _, path := range m.selectedDotfile.Paths {
		sourcePath, found := manifest.ResolveFilePath(searchPath, path, structure)
		if !found {
			// Return pathNotFoundMsg to trigger directory browser
			return pathNotFoundMsg{
				requestedPath: path,
				repoPath:      searchPath,
			}
		}

		// check if it's a file or directory
		info, err := os.Stat(sourcePath)
		if err != nil {
			return errorMsg{fmt.Errorf("couldn't stat %s: %w", sourcePath, err)}
		}

		if info.IsDir() {
			// for directories, we'll walk and add all files
			filesFound := 0
			err := filepath.Walk(sourcePath, func(walkPath string, walkInfo os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}

				// Skip .git directories entirely (both file placeholders and real git dirs)
				if walkInfo.IsDir() && filepath.Base(walkPath) == ".git" {
					return filepath.SkipDir
				}

				if !walkInfo.IsDir() {
					// get relative path from source
					relPath, err := filepath.Rel(sourcePath, walkPath)
					if err != nil {
						return err
					}
					// target path is the dotfile path + relative path
					targetPath := filepath.Join(path, relPath)
					fileMap[walkPath] = targetPath
					filesFound++
				}
				return nil
			})
			if err != nil {
				return errorMsg{fmt.Errorf("couldn't walk directory %s: %w", sourcePath, err)}
			}

			// If directory is empty (or only has .git file), try to resolve as submodule
			if filesFound == 0 {
				// Check if this might be a submodule
				isEmpty, err := cache.IsEmptyDirectory(sourcePath)
				if err == nil && isEmpty {
					// Try to resolve all submodules recursively (depth limit 3)
					// This handles nested submodules automatically
					ctx := context.Background()
					repoPath := m.cache.GetRepoPath(m.selectedCreator.ID)

					if err := cache.ResolveSubmodulesRecursive(ctx, repoPath, 3); err == nil {
						// Submodules resolved! Re-walk the directory
						filesFound = 0
						err := filepath.Walk(sourcePath, func(walkPath string, walkInfo os.FileInfo, walkErr error) error {
							if walkErr != nil {
								return walkErr
							}

							// Skip .git directories entirely (both file placeholders and real git dirs)
							if walkInfo.IsDir() && filepath.Base(walkPath) == ".git" {
								return filepath.SkipDir
							}

							if !walkInfo.IsDir() {
								relPath, err := filepath.Rel(sourcePath, walkPath)
								if err != nil {
									return err
								}
								targetPath := filepath.Join(path, relPath)
								fileMap[walkPath] = targetPath
								filesFound++
							}
							return nil
						})
						if err != nil {
							return errorMsg{fmt.Errorf("couldn't walk submodule directory %s: %w", sourcePath, err)}
						}
					}
				}

				// If still empty after trying submodule resolution, show browser
				if filesFound == 0 {
					return pathNotFoundMsg{
						requestedPath: path,
						repoPath:      searchPath,
					}
				}
			}
		} else {
			// single file
			fileMap[sourcePath] = path
		}
	}

	return filesResolvedMsg{
		structure: structure,
		fileMap:   fileMap,
	}
}

// detectPluginManager scans neovim config for plugin managers
func (m *Model) detectPluginManager() tea.Msg {
	// Find the nvim config directory in the file map
	var nvimConfigPath string
	for sourcePath := range m.fileMap {
		if strings.Contains(sourcePath, "nvim") {
			// Use the directory containing nvim files
			info, err := os.Stat(sourcePath)
			if err == nil && info.IsDir() {
				nvimConfigPath = sourcePath
				break
			}
			// If it's a file, use its parent directory
			nvimConfigPath = filepath.Dir(sourcePath)
			break
		}
	}

	if nvimConfigPath == "" {
		// No config found, skip plugin manager detection
		return m.generateDiffs()
	}

	manager, err := deps.DetectPluginManager(nvimConfigPath)
	if err != nil || manager == nil {
		// No plugin manager detected, continue
		return m.generateDiffs()
	}

	return pluginManagerDetectedMsg{manager: manager}
}

// generateDiffs generates diffs for all resolved files
func (m *Model) generateDiffs() tea.Msg {
	var results []*diff.Result
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return errorMsg{fmt.Errorf("couldn't get home directory: %w", err)}
	}

	for sourcePath, targetRelPath := range m.fileMap {
		// resolve target path to absolute
		targetPath := m.applier.ResolveTargetPath(targetRelPath, homeDir)

		result, err := diff.GenerateDiff(sourcePath, targetPath)
		if err != nil {
			return errorMsg{fmt.Errorf("couldn't generate diff for %s: %w", targetRelPath, err)}
		}
		results = append(results, result)
	}

	return diffGeneratedMsg{result: results}
}

// checkDependencies checks if required tools are installed
func (m *Model) checkDependencies() tea.Msg {
	// Convert string dependencies to Dependency structs
	deps := make([]deps.Dependency, 0, len(m.selectedDotfile.Dependencies))

	for _, depName := range m.selectedDotfile.Dependencies {
		dep := getDependencyInfo(depName)
		deps = append(deps, dep)
	}

	results := m.depChecker.CheckMultiple(deps)

	// Convert to interface{} to avoid import cycles in models.go
	interfaceResults := make([]interface{}, len(results))
	for i, r := range results {
		interfaceResults[i] = r
	}

	return dependenciesCheckedMsg{results: interfaceResults}
}

// getDependencyInfo returns dependency metadata
func getDependencyInfo(name string) deps.Dependency {
	// Map common dependency names to their info
	depMap := map[string]deps.Dependency{
		"neovim": {
			Name:    "neovim",
			Command: "nvim",
			PackageNames: map[string]string{
				"homebrew": "neovim",
				"apt":      "neovim",
				"pacman":   "neovim",
				"dnf":      "neovim",
			},
			Description: "Neovim text editor",
		},
		"tmux": {
			Name:    "tmux",
			Command: "tmux",
			PackageNames: map[string]string{
				"homebrew": "tmux",
				"apt":      "tmux",
				"pacman":   "tmux",
				"dnf":      "tmux",
			},
			Description: "Terminal multiplexer",
		},
		"zsh": {
			Name:    "zsh",
			Command: "zsh",
			PackageNames: map[string]string{
				"homebrew": "zsh",
				"apt":      "zsh",
				"pacman":   "zsh",
				"dnf":      "zsh",
			},
			Description: "Z shell",
		},
		"alacritty": {
			Name:    "alacritty",
			Command: "alacritty",
			PackageNames: map[string]string{
				"homebrew": "alacritty",
				"apt":      "alacritty",
				"pacman":   "alacritty",
				"dnf":      "alacritty",
			},
			Description: "GPU-accelerated terminal emulator",
		},
		"i3-wm": {
			Name:    "i3-wm",
			Command: "i3",
			PackageNames: map[string]string{
				"homebrew": "i3",
				"apt":      "i3-wm",
				"pacman":   "i3-wm",
				"dnf":      "i3",
			},
			Description: "i3 tiling window manager",
		},
		"polybar": {
			Name:    "polybar",
			Command: "polybar",
			PackageNames: map[string]string{
				"homebrew": "polybar",
				"apt":      "polybar",
				"pacman":   "polybar",
				"dnf":      "polybar",
			},
			Description: "Polybar status bar",
		},
		"oh-my-zsh": {
			Name:    "oh-my-zsh",
			Command: "omz", // oh-my-zsh doesn't have a binary, just a framework
			PackageNames: map[string]string{
				"homebrew": "", // oh-my-zsh is installed via script, not package manager
			},
			Description: "Oh My Zsh framework (install manually)",
		},
	}

	if dep, ok := depMap[name]; ok {
		return dep
	}

	// Fallback for unknown dependencies
	return deps.Dependency{
		Name:    name,
		Command: name,
		PackageNames: map[string]string{
			"homebrew": name,
			"apt":      name,
			"pacman":   name,
			"dnf":      name,
		},
		Description: name,
	}
}

// installMissingDependencies installs all missing dependencies
func (m *Model) installMissingDependencies() tea.Msg {
	for _, result := range m.depResults {
		if !result.Installed {
			err := m.depChecker.GetPackageManager().Install(result.Dependency)
			if err != nil {
				return errorMsg{fmt.Errorf("failed to install %s: %w", result.Dependency.Name, err)}
			}
		}
	}

	// Re-check to confirm installation
	deps := make([]deps.Dependency, 0, len(m.depResults))
	for _, result := range m.depResults {
		deps = append(deps, result.Dependency)
	}

	results := m.depChecker.CheckMultiple(deps)

	// Check if all are now installed
	allInstalled := true
	for _, result := range results {
		if !result.Installed {
			allInstalled = false
			break
		}
	}

	if allInstalled {
		return dependenciesInstalledMsg{success: true}
	}

	return errorMsg{fmt.Errorf("some dependencies failed to install")}
}

// installPluginManager installs the neovim plugin manager
func (m *Model) installPluginManager() tea.Msg {
	// Run the install command
	parts := strings.Fields(m.pluginManager.InstallCmd)
	if len(parts) == 0 {
		return errorMsg{fmt.Errorf("invalid install command")}
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorMsg{fmt.Errorf("failed to install %s: %w\n%s", m.pluginManager.Name, err, output)}
	}

	return pluginManagerInstalledMsg{success: true}
}

// applyFiles applies all files with backups
func (m *Model) applyFiles() tea.Msg {
	results := m.applier.ApplyMultiple(m.fileMap, m.selectedCreator, m.selectedDotfile)

	// check for errors
	var hasErrors bool
	for _, result := range results {
		if result.Error != nil {
			hasErrors = true
			break
		}
	}

	if hasErrors {
		// collect error messages
		var errMsgs []string
		for _, result := range results {
			if result.Error != nil {
				errMsgs = append(errMsgs, fmt.Sprintf("%s: %v", result.TargetPath, result.Error))
			}
		}
		return errorMsg{fmt.Errorf("failed to apply some files:\n%s", strings.Join(errMsgs, "\n"))}
	}

	return applyCompleteMsg{results: results}
}

// buildCategoryList creates the category list
func (m *Model) buildCategoryList() {
	var items []list.Item
	for i := range m.manifest.Categories {
		cat := &m.manifest.Categories[i]
		creators := m.manifest.GetCreatorsByCategory(cat.ID)
		items = append(items, listItem{
			title:       fmt.Sprintf("%s (%d creators)", cat.Name, len(creators)),
			description: cat.Description,
			data:        cat,
		})
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = selectedStyle
	delegate.Styles.SelectedDesc = mutedStyle

	m.categoryList = list.New(items, delegate, 80, 20)
	m.categoryList.Title = ""
	m.categoryList.SetShowStatusBar(false)
	m.categoryList.SetFilteringEnabled(true)
	m.categoryList.Styles.Title = titleStyle
}

// buildCreatorList creates the creator list for selected category
func (m *Model) buildCreatorList() {
	creators := m.manifest.GetCreatorsByCategory(m.selectedCategory.ID)
	var items []list.Item
	for i := range creators {
		creator := &creators[i]
		items = append(items, listItem{
			title:       fmt.Sprintf("%s (%d dotfiles)", creator.Name, len(creator.Dotfiles)),
			description: creator.Description,
			data:        creator,
		})
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = selectedStyle
	delegate.Styles.SelectedDesc = mutedStyle

	m.creatorList = list.New(items, delegate, 80, 20)
	m.creatorList.Title = ""
	m.creatorList.SetShowStatusBar(false)
	m.creatorList.SetFilteringEnabled(true)
}

// buildDotfileList creates the dotfile list for selected creator
func (m *Model) buildDotfileList() {
	var items []list.Item
	for i := range m.selectedCreator.Dotfiles {
		dotfile := &m.selectedCreator.Dotfiles[i]
		items = append(items, listItem{
			title:       dotfile.Name,
			description: dotfile.Description,
			data:        dotfile,
		})
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = selectedStyle
	delegate.Styles.SelectedDesc = mutedStyle

	m.dotfileList = list.New(items, delegate, 80, 20)
	m.dotfileList.Title = ""
	m.dotfileList.SetShowStatusBar(false)
	m.dotfileList.SetFilteringEnabled(false)
}

// Note: showRootSelection and viewRootSelection removed - we now auto-detect structure
// instead of forcing manual selection. Directory browser is only shown as a fallback
// when auto-detection fails (via ScreenDirectoryBrowser).

// viewDirectoryBrowser shows the directory browser
func (m *Model) viewDirectoryBrowser() string {
	if m.dirBrowser == nil {
		return "error: directory browser not initialized"
	}

	var b strings.Builder
	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n\n")
	b.WriteString(formatSubtitle(fmt.Sprintf("finding %s config for %s", m.selectedDotfile.Name, m.selectedCreator.Name)))
	b.WriteString("\n\n")

	b.WriteString("🔍 Auto-detection couldn't find the config files automatically.\n\n")

	b.WriteString(formatSubtitle("What you need to do:"))
	b.WriteString("\n")
	b.WriteString("1. Navigate to the directory that contains the dotfiles you want\n")
	b.WriteString("2. Look for directories like '.config', 'nvim', 'tmux', etc.\n")
	b.WriteString("3. Press 'c' when you're in the right directory\n\n")

	b.WriteString(formatSubtitle("Example structures:"))
	b.WriteString("\n")
	b.WriteString("   For nvim:     .config/nvim/  (has init.lua or init.vim)\n")
	b.WriteString("   For tmux:     home/ or root/  (has .tmux.conf)\n")
	b.WriteString("   For zsh:      home/ or root/  (has .zshrc)\n\n")

	b.WriteString(mutedStyle.Render("💡 Tip: Look at the file names shown below to find config files\n"))
	b.WriteString(mutedStyle.Render("   like init.lua, .tmux.conf, .zshrc, etc.\n\n"))

	b.WriteString(m.dirBrowser.View())
	return b.String()
}

// resolveSelectedDirectory resolves a user-selected directory path
func (m *Model) resolveSelectedDirectory(selectedPath string) tea.Cmd {
	return func() tea.Msg {
		// Get the target path from the dotfile manifest
		targetPath := m.selectedDotfile.Paths[0] // Use first path for now

		// Check if it's a file or directory
		info, err := os.Stat(selectedPath)
		if err != nil {
			return errorMsg{fmt.Errorf("couldn't stat selected path %s: %w", selectedPath, err)}
		}

		fileMap := make(map[string]string)

		if info.IsDir() {
			// Walk the directory and add all files
			err := filepath.Walk(selectedPath, func(walkPath string, walkInfo os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !walkInfo.IsDir() {
					// Get relative path from source
					relPath, err := filepath.Rel(selectedPath, walkPath)
					if err != nil {
						return err
					}
					// Target path is the dotfile path + relative path
					targetFullPath := filepath.Join(targetPath, relPath)
					fileMap[walkPath] = targetFullPath
				}
				return nil
			})
			if err != nil {
				return errorMsg{fmt.Errorf("couldn't walk directory %s: %w", selectedPath, err)}
			}
		} else {
			// Single file
			fileMap[selectedPath] = targetPath
		}

		return filesResolvedMsg{
			structure: manifest.StructureUnknown, // User manually selected
			fileMap:   fileMap,
		}
	}
}

// Run starts the TUI application
func Run() error {
	ctx := context.Background()
	m, err := New(ctx)
	if err != nil {
		return err
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
