// Package components holds gl-pipe's individual Bubbletea sub-models: the
// first-run wizard, project explorer, pipeline/job matrix, variable editor,
// log viewer, leader menu, and settings screen. Each owns only its own
// widget state; async GitLab calls and cross-component orchestration live
// in internal/ui's root model.
package components

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// WizardField identifies which input is focused.
type WizardField int

const (
	WizardFieldURL WizardField = iota
	WizardFieldToken
)

// WizardSubmitMsg is emitted when the user asks to validate & save the
// entered instance URL and token. The root model owns the actual network
// call and config persistence.
type WizardSubmitMsg struct {
	URL   string
	Token string
}

// Wizard is the first-run onboarding form: instance URL + masked PAT input,
// validated against the GitLab API before anything is written to disk.
type Wizard struct {
	URLInput   textinput.Model
	TokenInput textinput.Model
	Focus      WizardField

	Validating bool
	Err        error
	Username   string // set by the root model after a successful validation
}

// NewWizard constructs a Wizard with the URL field focused and defaulted to
// gitlab.com, per the spec's first-run defaults.
func NewWizard() Wizard {
	url := textinput.New()
	url.Placeholder = "https://gitlab.com"
	url.SetValue("https://gitlab.com")
	url.CharLimit = 200
	url.Width = 50
	url.Focus()

	token := textinput.New()
	token.Placeholder = "glpat-xxxxxxxxxxxxxxxxxxxx"
	token.EchoMode = textinput.EchoPassword
	token.EchoCharacter = '•'
	token.CharLimit = 200
	token.Width = 50

	return Wizard{URLInput: url, TokenInput: token, Focus: WizardFieldURL}
}

func (w *Wizard) SetValidating(v bool) { w.Validating = v }

func (w *Wizard) SetError(err error) {
	w.Err = err
	w.Validating = false
}

func (w Wizard) Update(msg tea.Msg) (Wizard, tea.Cmd) {
	if w.Validating {
		return w, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab", "down", "up":
			if w.Focus == WizardFieldURL {
				w.Focus = WizardFieldToken
				w.URLInput.Blur()
				w.TokenInput.Focus()
			} else {
				w.Focus = WizardFieldURL
				w.TokenInput.Blur()
				w.URLInput.Focus()
			}
			return w, nil
		case "enter":
			if w.URLInput.Value() == "" || w.TokenInput.Value() == "" {
				w.Err = errRequiredFields
				return w, nil
			}
			w.Err = nil
			return w, func() tea.Msg {
				return WizardSubmitMsg{URL: w.URLInput.Value(), Token: w.TokenInput.Value()}
			}
		}
	}

	var cmd tea.Cmd
	if w.Focus == WizardFieldURL {
		w.URLInput, cmd = w.URLInput.Update(msg)
	} else {
		w.TokenInput, cmd = w.TokenInput.Update(msg)
	}
	return w, cmd
}

var errRequiredFields = wizardErr("instance URL and token are both required")

type wizardErr string

func (e wizardErr) Error() string { return string(e) }

func (w Wizard) View() string {
	label := lipgloss.NewStyle().Bold(true).Width(16)
	body := "Welcome to gl-pipe — let's connect to a GitLab instance.\n\n"
	body += label.Render("Instance URL:") + w.URLInput.View() + "\n"
	body += label.Render("Access Token:") + w.TokenInput.View() + "\n\n"

	switch {
	case w.Validating:
		body += "Validating credentials...\n"
	case w.Err != nil:
		body += lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("Error: "+w.Err.Error()) + "\n"
	case w.Username != "":
		body += lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("Connected as "+w.Username) + "\n"
	}

	body += "\ntab: switch field · enter: validate & continue · ctrl+c: quit"
	return body
}
