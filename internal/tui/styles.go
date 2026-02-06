// package tui provides the terminal user interface
package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// colors - using a nice purple/pink palette
	primaryColor   = lipgloss.Color("205") // bright pink
	secondaryColor = lipgloss.Color("141") // purple
	accentColor    = lipgloss.Color("219") // light pink
	successColor   = lipgloss.Color("86")  // green
	errorColor     = lipgloss.Color("203") // red
	mutedColor     = lipgloss.Color("241") // gray

	// title style - big and bold
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginBottom(1)

	// subtitle for section headers
	subtitleStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true).
			MarginTop(1)

	// normal text
	textStyle = lipgloss.NewStyle()

	// muted text for descriptions
	mutedStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true)

	// selected item in a list
	selectedStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true).
			PaddingLeft(2)

	// unselected item in a list
	unselectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			PaddingLeft(2)

	// border style for boxes
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(secondaryColor).
			Padding(1, 2)

	// error message style
	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true).
			Padding(1)

	// success message style
	successStyle = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true).
			Padding(1)

	// help text at the bottom
	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	// status bar at the top
	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(secondaryColor).
			Padding(0, 1)

	// diff additions (green)
	diffAddStyle = lipgloss.NewStyle().
			Foreground(successColor)

	// diff deletions (red)
	diffDelStyle = lipgloss.NewStyle().
			Foreground(errorColor)

	// diff context (gray)
	diffContextStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	// category badge style
	badgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(accentColor).
			Padding(0, 1).
			MarginRight(1)

	// spinner style for loading
	spinnerStyle = lipgloss.NewStyle().
			Foreground(primaryColor)

	// progress bar style
	progressBarStyle = lipgloss.NewStyle().
				Foreground(primaryColor)
)

// formatTitle renders a title with the app name
func formatTitle(text string) string {
	return titleStyle.Render(text)
}

// formatSubtitle renders a section header
func formatSubtitle(text string) string {
	return subtitleStyle.Render(text)
}

// formatError renders an error message
func formatError(text string) string {
	return errorStyle.Render("oops! " + text)
}

// formatSuccess renders a success message
func formatSuccess(text string) string {
	return successStyle.Render("nice! " + text)
}

// formatHelp renders help text
func formatHelp(text string) string {
	return helpStyle.Render(text)
}

// formatStatusBar renders a status bar
func formatStatusBar(text string) string {
	return statusBarStyle.Render(text)
}

// formatBadge renders a badge (for counts, tags, etc)
func formatBadge(text string) string {
	return badgeStyle.Render(text)
}

// formatDiffLine formats a diff line based on its prefix
func formatDiffLine(line string) string {
	if len(line) == 0 {
		return line
	}

	switch line[0] {
	case '+':
		return diffAddStyle.Render(line)
	case '-':
		return diffDelStyle.Render(line)
	default:
		return diffContextStyle.Render(line)
	}
}
