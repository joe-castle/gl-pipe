package components

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var errorLinePattern = regexp.MustCompile(`(?i)\b(error|fail(ed|ure)?|fatal|panic|exception)\b`)

// LogViewer streams one job's trace into a scrollable, ANSI-aware viewport
// with in-buffer search and jump-to-first-error.
//
// NOTE: the content field must remain a plain string, not a strings.Builder.
// LogViewer is a value type (stored by value in ui.Model, passed by value to
// View()). strings.Builder must not be copied after first use — copying it
// shares the internal []byte buffer, and a subsequent WriteString/grow that
// reallocates the buffer leaves the copy holding a dangling pointer, which
// the GC scanner reports as "bad pointer in Go heap" during the next GC
// cycle. A plain string is safe to copy: the string value is immutable even
// though the field is reassigned on each Append call.
type LogViewer struct {
	Active bool
	JobID  int
	Done   bool

	viewport viewport.Model
	content  string
	lines    []string

	searching   bool
	searchInput textinput.Model
	matches     []int
	matchIdx    int

	autoScroll bool
}

func NewLogViewer() LogViewer {
	search := textinput.New()
	search.Prompt = "/ "
	return LogViewer{
		viewport:    viewport.New(80, 20),
		searchInput: search,
		autoScroll:  true,
	}
}

// Open resets the buffer and activates the viewer for a new job.
func (l *LogViewer) Open(jobID int) {
	l.Active = true
	l.JobID = jobID
	l.Done = false
	l.content = ""
	l.lines = nil
	l.matches = nil
	l.autoScroll = true
	l.viewport.SetContent("")
	l.viewport.GotoTop()
}

func (l *LogViewer) SetSize(w, h int) {
	l.viewport.Width = w
	l.viewport.Height = h - 2
}

// Append adds an incremental chunk of trace output.
func (l *LogViewer) Append(text string) {
	l.content += text
	l.lines = strings.Split(l.content, "\n")
	l.viewport.SetContent(l.content)
	if l.autoScroll {
		l.viewport.GotoBottom()
	}
}

// MarkDone flags the job as finished (no more chunks expected).
func (l *LogViewer) MarkDone() { l.Done = true }

// HasTextFocus reports whether the in-buffer search input owns keystrokes.
func (l *LogViewer) HasTextFocus() bool { return l.searching }

func (l LogViewer) Update(msg tea.Msg) (LogViewer, tea.Cmd) {
	if l.searching {
		return l.updateSearch(msg)
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			l.Active = false
			return l, nil
		case "/":
			l.searching = true
			l.searchInput.Focus()
			return l, nil
		case "E":
			l.jumpToFirstError()
			return l, nil
		case "n":
			l.nextMatch(1)
			return l, nil
		case "N":
			l.nextMatch(-1)
			return l, nil
		case "G":
			l.autoScroll = true
			l.viewport.GotoBottom()
			return l, nil
		}
	}

	var cmd tea.Cmd
	l.viewport, cmd = l.viewport.Update(msg)
	l.autoScroll = l.viewport.AtBottom()
	return l, cmd
}

func (l LogViewer) updateSearch(msg tea.Msg) (LogViewer, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			l.searching = false
			l.searchInput.Blur()
			return l, nil
		case "enter":
			l.searching = false
			l.searchInput.Blur()
			l.runSearch(l.searchInput.Value())
			return l, nil
		}
	}
	var cmd tea.Cmd
	l.searchInput, cmd = l.searchInput.Update(msg)
	return l, cmd
}

func (l *LogViewer) runSearch(query string) {
	l.matches = nil
	l.matchIdx = -1
	if query == "" {
		return
	}
	q := strings.ToLower(query)
	for i, line := range l.lines {
		if strings.Contains(strings.ToLower(line), q) {
			l.matches = append(l.matches, i)
		}
	}
	l.nextMatch(1)
}

func (l *LogViewer) nextMatch(dir int) {
	if len(l.matches) == 0 {
		return
	}
	l.matchIdx = ((l.matchIdx+dir)%len(l.matches) + len(l.matches)) % len(l.matches)
	l.autoScroll = false
	l.viewport.SetYOffset(l.matches[l.matchIdx])
}

func (l *LogViewer) jumpToFirstError() {
	for i, line := range l.lines {
		if errorLinePattern.MatchString(line) {
			l.autoScroll = false
			l.viewport.SetYOffset(i)
			return
		}
	}
}

func (l LogViewer) View() string {
	status := "streaming..."
	if l.Done {
		status = "finished"
	}
	header := lipgloss.NewStyle().Bold(true).Render("Job log — " + status)
	body := header + "\n" + l.viewport.View()
	if l.searching {
		body += "\n" + l.searchInput.View()
	} else if len(l.matches) > 0 {
		body += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render(
			"match "+strconv.Itoa(l.matchIdx+1)+"/"+strconv.Itoa(len(l.matches))+" (n/N to cycle)")
	}
	body += "\n" + RenderHelp(
		[2]string{"/", "search"},
		[2]string{"E", "jump to error"},
		[2]string{"G", "bottom"},
		[2]string{"esc", "back"},
	)
	return body
}
