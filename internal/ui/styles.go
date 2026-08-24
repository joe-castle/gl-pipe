package ui

import "github.com/charmbracelet/lipgloss"

// Palette. k9s-inspired: cyan accent, dark chrome, strong status colors.
// Kept to a small, deliberately reused set so the whole app reads as one
// system rather than a pile of ad hoc hex codes. Mirrored (not shared, to
// avoid an import cycle: components can't import ui) in
// internal/ui/components/style.go for the tables and their own help lines.
var (
	colorMuted  = lipgloss.Color("244")
	colorAccent = lipgloss.Color("51") // cyan — k9s' signature accent
	colorBorder = lipgloss.Color("238")
	colorFailed = lipgloss.Color("203")
)

var (
	// titleStyle is the " gl-pipe " badge in the header bar.
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("16")).
			Background(colorAccent).
			Padding(0, 1)

	// breadcrumbStyle names the current view (EXPLORER / PIPELINES / ...).
	breadcrumbStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	// contextStyle renders secondary header info (instance name, counts).
	contextStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorFailed).
			Bold(true)

	// contentBorderStyle frames the main pane (explorer/pipelines). No
	// padding — it wraps an already-rendered body, so the extra 2 cols/2
	// rows it adds are exactly the border itself, keeping the width/height
	// math WindowSizeMsg does for the components inside it simple.
	contentBorderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder)

	modalStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)
)
