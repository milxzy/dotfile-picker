// package tui provides the terminal user interface
package tui

import (
	"context"
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

	// selections
	selectedCategory *manifest.Category
	selectedCreator  *manifest.Creator
	selectedDotfile  *manifest.Dotfile
	selectedDotfiles []*manifest.Dotfile

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
		if m.screen == ScreenSubmoduleConfirm {
			switch msg.String() {
			case "y", "Y":
				m.screen = ScreenDownloading
				m.statusMsg = "initializing submodules"
				return m, tea.Batch(m.spinner.Tick, m.initSubmodules)
			case "n", "N":
				// skip submodules, continue
				return m, m.detectStructure
			}
		}

		if m.screen == ScreenDependencyCheck {
			switch msg.String() {
			case "i", "I":
				// install missing dependencies
				m.screen = ScreenDownloading
				m.statusMsg = "installing dependencies"
				return m, tea.Batch(m.spinner.Tick, m.installMissingDependencies)
			case "s", "S":
				// skip and continue with download
				m.screen = ScreenDownloading
				return m, tea.Batch(m.spinner.Tick, m.downloadRepo)
			case "enter":
				// all installed, continue
				m.screen = ScreenDownloading
				return m, tea.Batch(m.spinner.Tick, m.downloadRepo)
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

		if m.screen == ScreenError {
			switch msg.String() {
			case "c", "C":
				// Continue without submodules (if it was a submodule error)
				if m.err != nil && strings.Contains(m.err.Error(), "submodule") {
					m.err = nil
					m.screen = ScreenDownloading
					m.statusMsg = "detecting repository structure"
					return m, tea.Batch(m.spinner.Tick, m.detectStructure)
				}
			}
		}

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
			case ScreenDependencyCheck:
				m.screen = ScreenDotfile
			case ScreenSubmoduleConfirm:
				m.screen = ScreenDotfile
			case ScreenPluginManagerDetect:
				m.screen = ScreenDiff
			case ScreenDiff:
				m.screen = ScreenDotfile
			case ScreenComplete:
				m.screen = ScreenDotfile
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
		// repo downloaded, check for submodules
		return m, m.checkSubmodules

	case submodulesDetectedMsg:
		if msg.hasSubmodules {
			// ask user if they want to initialize submodules
			m.screen = ScreenSubmoduleConfirm
			return m, nil
		}
		// no submodules, proceed to structure detection
		return m, m.detectStructure

	case submodulesInitializedMsg:
		// submodules initialized, proceed to structure detection
		return m, m.detectStructure

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
			// dependencies installed, proceed with download
			m.screen = ScreenDownloading
			return m, tea.Batch(m.spinner.Tick, m.downloadRepo)
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

		// If this is a neovim config, check for plugin managers
		if m.selectedDotfile.ID == "nvim" {
			return m, m.detectPluginManager
		}

		// Not nvim or no plugin manager needed, continue to diffs
		return m, m.generateDiffs

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

	// update current list
	var cmd tea.Cmd
	switch m.screen {
	case ScreenCategory:
		m.categoryList, cmd = m.categoryList.Update(msg)
	case ScreenCreator:
		m.creatorList, cmd = m.creatorList.Update(msg)
	case ScreenDotfile:
		m.dotfileList, cmd = m.dotfileList.Update(msg)
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
	case ScreenDiff:
		return m.viewDiff()
	case ScreenSubmoduleConfirm:
		return m.viewSubmoduleConfirm()
	case ScreenDependencyCheck:
		return m.viewDependencyCheck()
	case ScreenPluginManagerDetect:
		return m.viewPluginManagerDetect()
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

// viewDownloading shows the downloading screen
func (m *Model) viewDownloading() string {
	var b strings.Builder

	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("   %s downloading and analyzing %s's dotfiles...\n\n", m.spinner.View(), m.selectedCreator.Name))
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

// viewSubmoduleConfirm shows the submodule initialization prompt
func (m *Model) viewSubmoduleConfirm() string {
	var b strings.Builder

	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n\n")
	b.WriteString("This repository contains git submodules (nested repositories).\n\n")
	b.WriteString("Submodules may contain additional configuration files needed\n")
	b.WriteString("for the dotfiles to work properly.\n\n")
	b.WriteString("Would you like to initialize them?\n")
	b.WriteString(mutedStyle.Render("(This may take a moment)\n\n"))
	b.WriteString(mutedStyle.Render("Note: Some submodules may require SSH keys for authentication.\n"))
	b.WriteString(mutedStyle.Render("If you don't have SSH keys set up, you can skip this step.\n\n"))
	b.WriteString(formatHelp("y: yes, initialize • n: no, skip • q: quit"))

	return b.String()
}

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

	// Check if it's a submodule error - offer to skip and continue
	if strings.Contains(errMsg, "submodule") {
		b.WriteString(mutedStyle.Render("Submodules are optional. You can skip them and continue.\n\n"))
		b.WriteString(formatHelp("c: continue without submodules • esc: back • q: quit"))
	} else {
		b.WriteString(formatHelp("esc: back • q: quit"))
	}

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
		// select creator
		if item, ok := m.creatorList.SelectedItem().(listItem); ok {
			if creator, ok := item.data.(*manifest.Creator); ok {
				m.selectedCreator = creator
				m.screen = ScreenDotfile
				m.buildDotfileList()
			}
		}
	case ScreenDotfile:
		// apply dotfile
		if item, ok := m.dotfileList.SelectedItem().(listItem); ok {
			if dotfile, ok := item.data.(*manifest.Dotfile); ok {
				m.selectedDotfile = dotfile
				m.statusMsg = dotfile.Name

				// Check dependencies FIRST if checker is available
				if m.depChecker != nil && len(dotfile.Dependencies) > 0 {
					return m, m.checkDependencies
				}

				// No dependencies or checker unavailable, proceed
				m.screen = ScreenDownloading
				return m, tea.Batch(m.spinner.Tick, m.downloadRepo)
			}
		}
	case ScreenDiff:
		// user confirmed, apply the files
		m.screen = ScreenDownloading
		m.statusMsg = "applying files and creating backups"
		return m, tea.Batch(m.spinner.Tick, m.applyFiles)
	}
	return m, nil
}

// fetchManifest loads the manifest
func (m *Model) fetchManifest() tea.Msg {
	manifest, err := m.fetcher.Fetch(context.Background())
	if err != nil {
		return errorMsg{err}
	}
	return manifestLoadedMsg{manifest}
}

// downloadRepo downloads the selected creator's repo
func (m *Model) downloadRepo() tea.Msg {
	ctx := context.Background()
	if err := m.cache.EnsureRepo(ctx, m.selectedCreator); err != nil {
		return errorMsg{err}
	}
	return repoDownloadedMsg{creatorID: m.selectedCreator.ID}
}

// checkSubmodules checks if the repo has git submodules
func (m *Model) checkSubmodules() tea.Msg {
	repoPath := m.cache.GetRepoPath(m.selectedCreator.ID)
	hasSubmodules, err := cache.HasSubmodules(repoPath)
	if err != nil {
		// not critical, just log and continue
		hasSubmodules = false
	}
	return submodulesDetectedMsg{
		creatorID:     m.selectedCreator.ID,
		hasSubmodules: hasSubmodules,
	}
}

// initSubmodules initializes git submodules
func (m *Model) initSubmodules() tea.Msg {
	ctx := context.Background()
	repoPath := m.cache.GetRepoPath(m.selectedCreator.ID)
	if err := cache.InitSubmodules(ctx, repoPath); err != nil {
		errStr := err.Error()

		// Check for private/non-existent repos (very common, not really an error)
		if strings.Contains(errStr, "private or don't exist") || strings.Contains(errStr, "Repository not found") {
			return errorMsg{fmt.Errorf("⚠️  Some submodules are private repositories\n\nThis is completely normal! Many creators have private configs.\nThe main dotfiles will work fine without them.\n\n👉 Press 'c' to continue, or 'n' next time to skip submodules.\n\n%s", errStr)}
		}

		// Check if it's an SSH/auth error
		if strings.Contains(errStr, "SSH") || strings.Contains(errStr, "ssh:") ||
			strings.Contains(errStr, "Permission denied") || strings.Contains(errStr, "access rights") {
			return errorMsg{fmt.Errorf("🔐 Submodules need authentication\n\nSome submodules require SSH keys or are private.\nMost dotfiles work fine without submodules!\n\n👉 Press 'c' to continue without them.\n\nIf you need submodules:\n  1. Set up SSH keys: https://docs.github.com/en/authentication\n  2. Or skip this step and continue\n\n%s", errStr)}
		}

		// Generic error
		return errorMsg{fmt.Errorf("⚠️  Submodule initialization had issues\n\nDon't worry - the main dotfiles will still work!\n\n👉 Press 'c' to continue without submodules.\n\n%s", errStr)}
	}
	return submodulesInitializedMsg{creatorID: m.selectedCreator.ID}
}

// detectStructure detects the repo structure and resolves file paths
func (m *Model) detectStructure() tea.Msg {
	repoPath := m.cache.GetRepoPath(m.selectedCreator.ID)
	structure := manifest.DetectStructure(repoPath)

	// resolve file paths
	fileMap := make(map[string]string)
	for _, path := range m.selectedDotfile.Paths {
		sourcePath, found := manifest.ResolveFilePath(repoPath, path, structure)
		if !found {
			return errorMsg{fmt.Errorf("couldn't find file: %s in repo", path)}
		}

		// check if it's a file or directory
		info, err := os.Stat(sourcePath)
		if err != nil {
			return errorMsg{fmt.Errorf("couldn't stat %s: %w", sourcePath, err)}
		}

		if info.IsDir() {
			// for directories, we'll walk and add all files
			err := filepath.Walk(sourcePath, func(walkPath string, walkInfo os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
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
				}
				return nil
			})
			if err != nil {
				return errorMsg{fmt.Errorf("couldn't walk directory %s: %w", sourcePath, err)}
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
