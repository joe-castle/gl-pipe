package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"

	"github.com/joeca/gl-pipe/internal/api"
)

// Palette mirrors internal/ui's (components can't import ui — that would
// be the cycle the other way). k9s-inspired: cyan accent, strong cursor
// band, muted secondary text.
var (
	colorAccent   = lipgloss.Color("51") // cyan
	colorMuted    = lipgloss.Color("244")
	colorSuccess  = lipgloss.Color("42")
	colorFailed   = lipgloss.Color("203")
	colorRunning  = lipgloss.Color("39")
	colorPending  = lipgloss.Color("221")
	colorCanceled = lipgloss.Color("244")
	colorManual   = lipgloss.Color("135")

	helpKeyStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	helpDescStyle  = lipgloss.NewStyle().Foreground(colorMuted)
	errorTextStyle = lipgloss.NewStyle().Foreground(colorFailed)
)

// TableStyles is the shared k9s-flavored skin for every table in the app:
// bold accent headers and a solid cyan cursor band.
//
// Cell is deliberately left with no color of its own (Padding only).
// bubbles/table measures each cell's width with a plain rune counter
// (mattn/go-runewidth) that has no idea what an ANSI escape sequence is —
// coloring individual cell text would make it count escape-code bytes as
// visible characters and truncate mid-sequence, corrupting the row. That
// also means Selected (which wraps the *already-rendered* row in one
// style call) only paints cleanly if no cell already injected its own
// color+reset first. So: color lives in the header (safe — applied after
// the plain title is truncated) and the selected-row band (safe — wraps
// plain cells), never inside a data cell. Status is distinguished with a
// plain-text icon (StatusIcon) instead of color for that reason.
func TableStyles() table.Styles {
	return table.Styles{
		Header: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			Padding(0, 1),
		Cell: lipgloss.NewStyle().
			Padding(0, 1),
		Selected: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("16")).
			Background(colorAccent),
	}
}

// StatusIcon renders a pipeline/job status as a short plain-text icon +
// label — safe to embed in a table.Row (see TableStyles), unlike a
// colored badge.
func StatusIcon(s api.PipelineStatus) string {
	switch s {
	case api.StatusSuccess:
		return "✓ success"
	case api.StatusFailed:
		return "✗ failed"
	case api.StatusRunning:
		return "● running"
	case api.StatusPending, api.StatusWaiting, api.StatusCreated:
		return "… pending"
	case api.StatusCanceled:
		return "⊘ canceled"
	case api.StatusSkipped:
		return "⊙ skipped"
	case api.StatusManual:
		return "▶ manual"
	default:
		return string(s)
	}
}

// RenderHelp renders a component's bottom hotkey line consistently: key in
// bold accent, description muted, pairs separated by " · ".
func RenderHelp(pairs ...[2]string) string {
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = helpKeyStyle.Render(p[0]) + " " + helpDescStyle.Render(p[1])
	}
	return strings.Join(parts, helpDescStyle.Render(" · "))
}
