package components

import (
	"strings"
	"testing"

	"github.com/joeca/gl-pipe/internal/api"
)

func TestFlexColumnWidth_UsesLeftoverSpace(t *testing.T) {
	// total 120, fixed columns 3+20=23, 2 columns -> overhead 2*3=6
	got := flexColumnWidth(120, []int{3, 20}, 20)
	want := 120 - 23 - 6
	if got != want {
		t.Fatalf("flexColumnWidth() = %d, want %d", got, want)
	}
}

func TestFlexColumnWidth_FloorsOnNarrowTerminal(t *testing.T) {
	got := flexColumnWidth(20, []int{3, 20}, 20)
	if got != 20 {
		t.Fatalf("flexColumnWidth() = %d, want floor 20", got)
	}
}

func TestSplitFlexWidth_RespectsRatioAndFloors(t *testing.T) {
	a, b := splitFlexWidth(100, []int{3, 9, 12}, 0.4, 10, 10)
	if a+b <= 0 {
		t.Fatalf("expected positive split, got a=%d b=%d", a, b)
	}
	if a < 10 || b < 10 {
		t.Fatalf("expected both above their floors, got a=%d b=%d", a, b)
	}
}

func TestSplitFlexWidth_FloorsBothOnNarrowTerminal(t *testing.T) {
	a, b := splitFlexWidth(30, []int{3, 9, 12}, 0.4, 14, 16)
	if a != 14 || b != 16 {
		t.Fatalf("expected both floors on a too-narrow terminal, got a=%d b=%d", a, b)
	}
}

// TestProjectList_LongPathNotTruncatedOnWideTerminal is a regression test
// for fixed-width columns silently truncating repo names regardless of how
// wide the terminal actually was.
func TestProjectList_LongPathNotTruncatedOnWideTerminal(t *testing.T) {
	longPath := "some-org/some-subgroup/a-fairly-long-service-name-that-would-not-fit-in-40-chars"
	p := NewProjectList()
	p.SetSize(160, 30) // wide terminal
	p.SetProjects([]api.Project{{ID: 1, PathWithNamespace: longPath}})

	if strings.Contains(p.View(), "…") {
		t.Fatalf("expected the full path to fit on a wide terminal, got:\n%s", p.View())
	}
	if !strings.Contains(p.View(), longPath) {
		t.Fatalf("expected the full path %q to appear untruncated, got:\n%s", longPath, p.View())
	}
}

// TestProjectList_NarrowTerminalStillTruncatesGracefully makes sure the
// floor kicks in rather than a negative/zero column width panicking.
func TestProjectList_NarrowTerminalStillTruncatesGracefully(t *testing.T) {
	p := NewProjectList()
	p.SetSize(10, 30) // unrealistically narrow
	p.SetProjects([]api.Project{{ID: 1, PathWithNamespace: "backend/svc-a"}})

	_ = p.View() // must not panic
}
