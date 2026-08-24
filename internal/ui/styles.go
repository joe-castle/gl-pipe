package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/joeca/gl-pipe/internal/api"
)

// Palette. Kept to a small, deliberately reused set of colors so the whole
// app reads as one system rather than a pile of ad hoc hex codes.
var (
	colorBg       = lipgloss.Color("235")
	colorFg       = lipgloss.Color("252")
	colorMuted    = lipgloss.Color("243")
	colorAccent   = lipgloss.Color("212")
	colorBorder   = lipgloss.Color("240")
	colorSuccess  = lipgloss.Color("42")
	colorFailed   = lipgloss.Color("203")
	colorRunning  = lipgloss.Color("39")
	colorPending  = lipgloss.Color("221")
	colorCanceled = lipgloss.Color("245")
	colorManual   = lipgloss.Color("135")
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(colorAccent).
			Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorFailed).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	focusedBorderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent)

	blurredBorderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder)

	modalStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)

	leaderMenuStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(0, 1)

	leaderKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	selectedRowStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("237")).
				Bold(true)

	checkedStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
)

// StatusBadge renders a short colored label for a pipeline/job status.
func StatusBadge(s api.PipelineStatus) string {
	style := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	switch s {
	case api.StatusSuccess:
		return style.Foreground(colorSuccess).Render("✓ success")
	case api.StatusFailed:
		return style.Foreground(colorFailed).Render("✗ failed")
	case api.StatusRunning:
		return style.Foreground(colorRunning).Render("● running")
	case api.StatusPending, api.StatusWaiting, api.StatusCreated:
		return style.Foreground(colorPending).Render("… pending")
	case api.StatusCanceled:
		return style.Foreground(colorCanceled).Render("⊘ canceled")
	case api.StatusSkipped:
		return style.Foreground(colorCanceled).Render("⊙ skipped")
	case api.StatusManual:
		return style.Foreground(colorManual).Render("▶ manual")
	default:
		return style.Foreground(colorMuted).Render(string(s))
	}
}
