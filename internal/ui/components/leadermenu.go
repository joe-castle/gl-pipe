package components

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LeaderAction mirrors ui.LeaderAction without importing the ui package
// (which imports components), keeping the dependency direction one-way.
type LeaderAction struct {
	Key   string
	Label string
}

// LeaderActionMsg is emitted when the user picks one leader menu entry.
type LeaderActionMsg struct {
	Key string
}

// LeaderMenu is the Helix-style bottom modal opened by <Space>.
type LeaderMenu struct {
	Active  bool
	Actions []LeaderAction
	Width   int
}

func NewLeaderMenu(actions []LeaderAction) LeaderMenu {
	return LeaderMenu{Actions: actions}
}

func (l *LeaderMenu) Open()  { l.Active = true }
func (l *LeaderMenu) Close() { l.Active = false }

func (l LeaderMenu) Update(msg tea.Msg) (LeaderMenu, tea.Cmd) {
	if !l.Active {
		return l, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" {
			l.Active = false
			return l, nil
		}
		for _, a := range l.Actions {
			if msg.String() == a.Key {
				l.Active = false
				key := a.Key
				return l, func() tea.Msg { return LeaderActionMsg{Key: key} }
			}
		}
	}
	return l, nil
}

func (l LeaderMenu) View() string {
	parts := make([]string, 0, len(l.Actions))
	for _, a := range l.Actions {
		parts = append(parts, helpKeyStyle.Render(a.Key)+" "+helpDescStyle.Render(a.Label))
	}
	content := strings.Join(parts, "   ")

	style := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(0, 1)
	if l.Width > 0 {
		style = style.Width(l.Width - 4)
	}
	return style.Render(content)
}
