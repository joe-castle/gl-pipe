package components

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joeca/gl-pipe/internal/api"
)

// manyRefs builds n refs named ref-0..ref-(n-1), well past the picker's
// fixed table height, to exercise scrolling rather than a list that
// happens to fit on screen.
func manyRefs(n int) []api.Ref {
	refs := make([]api.Ref, n)
	for i := range refs {
		refs[i] = api.Ref{Name: fmt.Sprintf("ref-%d", i), IsTag: i%2 == 0}
	}
	return refs
}

func TestRefPicker_SelectionPastVisibleWindowStillPicksCorrectRef(t *testing.T) {
	var r refPicker
	refs := manyRefs(20)
	r.Open(refs)

	// Move the cursor well past the fixed 15-row table height configured
	// in Open, into territory that requires the table to have scrolled.
	for i := 0; i < 18; i++ {
		r, _, _ = r.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}

	next, selected, ref := r.update(tea.KeyMsg{Type: tea.KeyEnter})
	if !selected {
		t.Fatal("expected a selection on enter")
	}
	if ref.Name != refs[18].Name {
		t.Fatalf("selected %q, want %q (the 19th ref, past the visible window)", ref.Name, refs[18].Name)
	}
	if next.active {
		t.Fatal("expected the picker to close after selecting")
	}
}

func TestRefPicker_ViewDoesNotRenderEveryRowUnbounded(t *testing.T) {
	var r refPicker
	r.Open(manyRefs(50))

	lines := strings.Count(r.viewString(), "\n")
	if lines > 30 {
		t.Fatalf("expected the rendered view to stay bounded by the table's fixed height, got %d lines for 50 refs", lines)
	}
}

func TestRefPicker_EscClosesWithoutSelecting(t *testing.T) {
	var r refPicker
	r.Open([]api.Ref{{Name: "main"}})

	next, selected, _ := r.update(tea.KeyMsg{Type: tea.KeyEsc})
	if selected {
		t.Fatal("expected no selection on esc")
	}
	if next.active {
		t.Fatal("expected the picker to close on esc")
	}
}
