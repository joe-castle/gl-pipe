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

// ansiPattern matches the CSI escape sequences GitLab traces are full of.
// The log viewer keeps them (a viewport renders them correctly); anything
// pulling trace text *out* of the viewer has to strip them, both because
// the destination isn't ANSI-aware and because bubbles/table measures
// width with go-runewidth, which counts escape bytes as visible characters.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes colour escapes and carriage returns from one trace line.
func stripANSI(line string) string {
	return strings.TrimSpace(strings.ReplaceAll(ansiPattern.ReplaceAllString(line, ""), "\r", ""))
}

// FirstErrorLines returns the first error-looking line of a job trace plus
// up to n-1 following non-empty lines of context, all ANSI- and CR-stripped.
// It returns nothing when the trace has no match.
//
// This shares errorLinePattern with the log viewer's own 'E' (jump to first
// error) deliberately, so the failure digest and the log viewer can never
// disagree about what counts as an error line.
//
// First-match is a heuristic and can land on noise — a dependency whose name
// contains "error", say. If that proves annoying in practice the fix is to
// skip the runner's own boilerplate (section_start/section_end, the trailing
// "ERROR: Job failed") and prefer the last meaningful match; not worth
// building speculatively.
func FirstErrorLines(trace string, n int) []string {
	if n < 1 {
		return nil
	}
	lines := strings.Split(trace, "\n")
	for i, raw := range lines {
		line := stripANSI(raw)
		if line == "" || !errorLinePattern.MatchString(line) {
			continue
		}
		out := []string{line}
		for _, follow := range lines[i+1:] {
			if len(out) >= n {
				break
			}
			if s := stripANSI(follow); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

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
