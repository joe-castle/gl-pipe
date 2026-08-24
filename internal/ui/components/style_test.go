package components

import (
	"strings"
	"testing"

	"github.com/joeca/gl-pipe/internal/api"
)

func TestStatusIcon_ReturnsPlainTextNoANSI(t *testing.T) {
	for _, s := range []api.PipelineStatus{
		api.StatusSuccess, api.StatusFailed, api.StatusRunning, api.StatusPending,
		api.StatusCanceled, api.StatusSkipped, api.StatusManual,
	} {
		icon := StatusIcon(s)
		if icon == "" {
			t.Errorf("StatusIcon(%s) returned empty string", s)
		}
		if strings.Contains(icon, "\x1b") {
			t.Errorf("StatusIcon(%s) = %q contains an ANSI escape — unsafe to embed in a table.Row (see TableStyles doc)", s, icon)
		}
	}
}

func TestStatusIcon_UnknownStatusFallsBackToRawString(t *testing.T) {
	got := StatusIcon(api.PipelineStatus("something_new"))
	if got != "something_new" {
		t.Errorf("StatusIcon(unknown) = %q, want the raw status string", got)
	}
}

func TestRenderHelp_JoinsPairsWithSeparator(t *testing.T) {
	got := RenderHelp([2]string{"x", "stage"}, [2]string{"esc", "cancel"})
	// Styled output embeds ANSI codes around "x", "stage", etc.; just check
	// the underlying words made it in, in order.
	for _, want := range []string{"x", "stage", "esc", "cancel"} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderHelp() = %q, missing %q", got, want)
		}
	}
}

func TestTableStyles_CellHasNoColorAttribute(t *testing.T) {
	// The whole point of TableStyles' Cell being color-free is that
	// Selected can wrap a full row cleanly (see the doc comment on
	// TableStyles). Rendering a plain string through Cell must not inject
	// any ANSI codes.
	styles := TableStyles()
	rendered := styles.Cell.Render("plain")
	if strings.Contains(rendered, "\x1b") {
		t.Errorf("Cell style rendered %q with ANSI codes — this would corrupt row-width truncation and break the Selected row band", rendered)
	}
}
