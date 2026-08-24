package components

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SwitchInstanceMsg asks the root model to switch the active GitLab profile.
type SwitchInstanceMsg struct {
	Name string
}

// Settings is the instance switcher / preset & TTL overview opened by
// <Space> s.
type Settings struct {
	Active  bool
	Names   []string
	Current string
	cursor  int

	TTL     time.Duration
	Presets []string
}

func NewSettings() Settings { return Settings{} }

// Open populates the settings screen from the live config.
func (s *Settings) Open(names []string, current string, ttl time.Duration, presets []string) {
	sort.Strings(names)
	sort.Strings(presets)
	s.Names = names
	s.Current = current
	s.TTL = ttl
	s.Presets = presets
	s.Active = true
	for i, n := range names {
		if n == current {
			s.cursor = i
		}
	}
}

func (s Settings) Update(msg tea.Msg) (Settings, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch km.String() {
	case "esc":
		s.Active = false
		return s, nil
	case "j", "down":
		if s.cursor < len(s.Names)-1 {
			s.cursor++
		}
		return s, nil
	case "k", "up":
		if s.cursor > 0 {
			s.cursor--
		}
		return s, nil
	case "enter":
		if s.cursor < len(s.Names) {
			name := s.Names[s.cursor]
			s.Active = false
			return s, func() tea.Msg { return SwitchInstanceMsg{Name: name} }
		}
	}
	return s, nil
}

func (s Settings) View() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Instances") + "\n")
	for i, n := range s.Names {
		marker := "  "
		if i == s.cursor {
			marker = "> "
		}
		active := ""
		if n == s.Current {
			active = " (active)"
		}
		b.WriteString(marker + n + active + "\n")
	}
	b.WriteString(fmt.Sprintf("\nCache TTL: %s\n", s.TTL))
	if len(s.Presets) > 0 {
		b.WriteString("\nPresets: " + strings.Join(s.Presets, ", ") + "\n")
	}
	b.WriteString("\nj/k: move · enter: switch instance · esc: close")
	return b.String()
}
