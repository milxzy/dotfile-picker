// package tui provides the terminal user interface
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/milxzy/dot-generator/internal/applier"
	"github.com/milxzy/dot-generator/internal/backup"
	"github.com/milxzy/dot-generator/internal/cache"
	"github.com/milxzy/dot-generator/internal/config"
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

	return &Model{
		screen:  ScreenLoading,
		cfg:     cfg,
		fetcher: fetcher,
		cache:   cacheManager,
		backup:  backupManager,
		applier: applierInstance,
		spinner: s,
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
	b.WriteString(fmt.Sprintf("   %s downloading %s's dotfiles...\n\n", m.spinner.View(), m.selectedCreator.Name))
	b.WriteString(mutedStyle.Render("   this may take a moment..."))

	return b.String()
}

// viewComplete shows the completion screen
func (m *Model) viewComplete() string {
	var b strings.Builder

	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n\n")
	b.WriteString(formatSuccess(fmt.Sprintf("ready to apply %s from %s!", m.statusMsg, m.selectedCreator.Name)))
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("   note: full apply functionality coming soon!\n"))
	b.WriteString(mutedStyle.Render("   for now, this demonstrates the workflow.\n\n"))
	b.WriteString(mutedStyle.Render("   what would happen:\n"))
	b.WriteString(mutedStyle.Render("   1. download the repo to cache\n"))
	b.WriteString(mutedStyle.Render("   2. show you a diff of changes\n"))
	b.WriteString(mutedStyle.Render("   3. create backups of existing configs\n"))
	b.WriteString(mutedStyle.Render("   4. apply the new configs\n\n"))
	b.WriteString(formatHelp("esc: back to dotfiles • q: quit"))

	return b.String()
}

// viewError shows an error
func (m *Model) viewError() string {
	var b strings.Builder

	b.WriteString(formatTitle("dotfile picker"))
	b.WriteString("\n\n")
	b.WriteString(formatError(m.err.Error()))
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
				// for now, just show a message
				// full implementation would download repo, diff, and apply
				m.screen = ScreenComplete
			}
		}
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
