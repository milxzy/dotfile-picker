// package tui provides a directory browser component
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DirEntry represents a file or directory in the browser
type DirEntry struct {
	name  string
	path  string
	isDir bool
}

// FilterValue implements list.Item interface
func (d DirEntry) FilterValue() string { return d.name }

// Title implements list.DefaultItem interface
func (d DirEntry) Title() string {
	if d.isDir {
		return "📁 " + d.name
	}
	return "📄 " + d.name
}

// Description implements list.DefaultItem interface
func (d DirEntry) Description() string {
	// Return empty string - we'll show file counts in the main view
	return ""
}

// DirBrowser is a directory navigation component
type DirBrowser struct {
	rootPath    string // Original root path
	currentPath string // Current directory being viewed
	list        list.Model
	width       int
	height      int
	targetName  string   // The directory/file we're looking for (e.g., "nvim")
	fileCount   int      // Number of files in current directory
	dirCount    int      // Number of subdirectories in current directory
	files       []string // List of files in current directory (for preview)
}

// NewDirBrowser creates a new directory browser
func NewDirBrowser(rootPath, targetName string, width, height int) *DirBrowser {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), width, height-10)
	l.Title = "Select directory for: " + targetName
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(false)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("170")).
		Bold(true).
		Padding(0, 1)

	browser := &DirBrowser{
		rootPath:    rootPath,
		currentPath: rootPath,
		list:        l,
		width:       width,
		height:      height,
		targetName:  targetName,
	}

	browser.LoadDirectory(rootPath)
	return browser
}

// LoadDirectory reads the directory and populates the list
func (b *DirBrowser) LoadDirectory(path string) {
	entries, err := os.ReadDir(path)
	if err != nil {
		// If we can't read the directory, just show empty list
		b.list.SetItems([]list.Item{})
		return
	}

	items := []list.Item{}

	// Add parent directory entry if not at root
	if path != b.rootPath {
		items = append(items, DirEntry{
			name:  "..",
			path:  filepath.Dir(path),
			isDir: true,
		})
	}

	// Separate directories and files
	var dirs []DirEntry
	var files []DirEntry
	var fileNames []string

	for _, entry := range entries {
		// Skip hidden files/dirs except .config
		if strings.HasPrefix(entry.Name(), ".") && entry.Name() != ".config" {
			continue
		}

		de := DirEntry{
			name:  entry.Name(),
			path:  filepath.Join(path, entry.Name()),
			isDir: entry.IsDir(),
		}

		if entry.IsDir() {
			dirs = append(dirs, de)
		} else {
			files = append(files, de)
			fileNames = append(fileNames, entry.Name())
		}
	}

	// Sort directories and files alphabetically
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].name < dirs[j].name
	})
	sort.Slice(files, func(i, j int) bool {
		return files[i].name < files[j].name
	})
	sort.Strings(fileNames)

	// Add directories first, then files
	for _, d := range dirs {
		items = append(items, d)
	}
	for _, f := range files {
		items = append(items, f)
	}

	b.list.SetItems(items)
	b.currentPath = path
	b.fileCount = len(files)
	b.dirCount = len(dirs)
	b.files = fileNames
	b.updateTitle()
}

// updateTitle updates the list title with current path
func (b *DirBrowser) updateTitle() {
	relPath, err := filepath.Rel(b.rootPath, b.currentPath)
	if err != nil {
		relPath = b.currentPath
	}
	if relPath == "." {
		relPath = "/"
	}
	b.list.Title = "Select directory for: " + b.targetName + " (in: " + relPath + ")"
}

// Update handles messages
func (b *DirBrowser) Update(msg tea.Msg) (*DirBrowser, tea.Cmd) {
	// Handle enter key BEFORE passing to list
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" {
			// Get selected item
			if item, ok := b.list.SelectedItem().(DirEntry); ok {
				if item.isDir {
					// Navigate into directory
					b.LoadDirectory(item.path)
					return b, nil
				}
			}
			// If not a directory, don't pass enter to list
			return b, nil
		}
	}

	// Pass other keys to the list
	var cmd tea.Cmd
	b.list, cmd = b.list.Update(msg)
	return b, cmd
}

// View renders the browser with file preview
func (b *DirBrowser) View() string {
	var view strings.Builder

	// Show current directory stats
	statsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	fileStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("170"))

	stats := fmt.Sprintf("📊 Current directory: %d directories, %d files", b.dirCount, b.fileCount)
	if b.dirCount == 0 && b.fileCount == 0 {
		stats = "📊 Current directory: empty"
	}
	view.WriteString(statsStyle.Render(stats))
	view.WriteString("\n\n")

	// Show file preview if there are files
	if len(b.files) > 0 {
		view.WriteString(fileStyle.Render("📄 Files in this directory:\n"))

		maxFiles := 8 // Show max 8 files
		for i, fileName := range b.files {
			if i >= maxFiles {
				remaining := len(b.files) - maxFiles
				view.WriteString(statsStyle.Render(fmt.Sprintf("   ... and %d more files\n", remaining)))
				break
			}
			view.WriteString(fmt.Sprintf("   • %s\n", fileName))
		}
		view.WriteString("\n")
	}

	view.WriteString(b.list.View())

	helpText := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("\n↑/↓: navigate • enter: open directory • c: confirm this directory • esc: cancel")
	view.WriteString(helpText)

	return view.String()
}

// GetSelectedPath returns the currently selected path
func (b *DirBrowser) GetSelectedPath() string {
	if item, ok := b.list.SelectedItem().(DirEntry); ok {
		return item.path
	}
	return b.currentPath
}

// GetCurrentPath returns the current directory being viewed
func (b *DirBrowser) GetCurrentPath() string {
	return b.currentPath
}
