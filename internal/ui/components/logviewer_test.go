package components

import (
	"strings"
	"testing"
)

func TestFirstErrorLines_ReturnsTheMatchPlusContext(t *testing.T) {
	trace := "Running with gitlab-runner\nok 1\nFAILED: token_test.go:88\nexpected 200, got 401\nmore context\nand more\n"

	got := FirstErrorLines(trace, 3)
	want := []string{"FAILED: token_test.go:88", "expected 200, got 401", "more context"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("FirstErrorLines = %q, want %q", got, want)
	}
}

func TestFirstErrorLines_StripsANSIAndCarriageReturns(t *testing.T) {
	// Real GitLab traces are full of colour escapes and \r, and this text is
	// rendered outside a viewport (in a plain table detail panel), so it has
	// to come back clean.
	trace := "ok\n\x1b[0;31mERROR: build step failed\x1b[0;m\r\n\x1b[32mdetail line\x1b[0m\r\n"

	got := FirstErrorLines(trace, 2)
	want := []string{"ERROR: build step failed", "detail line"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("FirstErrorLines = %q, want %q", got, want)
	}
}

func TestFirstErrorLines_EmptyWhenNothingMatches(t *testing.T) {
	if got := FirstErrorLines("all clear\nnothing to see\n", 3); len(got) != 0 {
		t.Errorf("FirstErrorLines = %q, want empty", got)
	}
}

func TestFirstErrorLines_SkipsBlankContextLines(t *testing.T) {
	trace := "panic: boom\n\n\n   \nthe real detail\n"

	got := FirstErrorLines(trace, 2)
	want := []string{"panic: boom", "the real detail"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("FirstErrorLines = %q, want %q", got, want)
	}
}

func TestFirstErrorLines_StopsAtTheEndOfTheTrace(t *testing.T) {
	got := FirstErrorLines("fatal: nothing after this\n", 5)
	if len(got) != 1 || got[0] != "fatal: nothing after this" {
		t.Errorf("FirstErrorLines = %q, want one line", got)
	}
}

func TestFirstErrorLines_MatchesAgainstTheStrippedLine(t *testing.T) {
	// The word is split by a colour escape in the raw bytes; matching before
	// stripping would miss it.
	got := FirstErrorLines("ok\nbuild \x1b[0;31mfailed\x1b[0m here\n", 1)
	if len(got) != 1 || got[0] != "build failed here" {
		t.Errorf("FirstErrorLines = %q, want the stripped line", got)
	}
}

func TestLogViewer_AppendGrowsContent(t *testing.T) {
	l := NewLogViewer()
	l.Open(1)
	l.Append("hello\n")
	l.Append("world\n")

	if len(l.lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d: %+v", len(l.lines), l.lines)
	}
}

func TestLogViewer_JumpToFirstError(t *testing.T) {
	l := NewLogViewer()
	l.Open(1)
	l.SetSize(80, 5) // viewport.Height becomes 3, so a longer buffer actually scrolls
	l.Append("0\n1\nERROR occurred\n3\n4\n5\n6\n")

	l.jumpToFirstError()
	if l.viewport.YOffset != 2 {
		t.Fatalf("expected YOffset 2 (0-indexed error line), got %d", l.viewport.YOffset)
	}
}

func TestLogViewer_JumpToFirstErrorNoOpWhenNoneFound(t *testing.T) {
	l := NewLogViewer()
	l.Open(1)
	l.Append("all clear\nnothing to see\n")

	l.jumpToFirstError()
	if l.viewport.YOffset != 0 {
		t.Fatalf("expected YOffset unchanged at 0, got %d", l.viewport.YOffset)
	}
}

func TestLogViewer_SearchFindsAllMatches(t *testing.T) {
	l := NewLogViewer()
	l.Open(1)
	l.Append("alpha\nbeta needle here\ngamma\nneedle again\n")

	l.runSearch("needle")
	if len(l.matches) != 2 {
		t.Fatalf("expected 2 matches, got %d: %+v", len(l.matches), l.matches)
	}
}

func TestLogViewer_HasTextFocusWhileSearching(t *testing.T) {
	l := NewLogViewer()
	l.Open(1)
	if l.HasTextFocus() {
		t.Fatal("should not have text focus initially")
	}

	updated, _ := l.Update(runeKey('/'))
	if !updated.HasTextFocus() {
		t.Fatal("expected text focus after '/'")
	}
}

func TestLogViewer_OpenResetsBufferForNewJob(t *testing.T) {
	l := NewLogViewer()
	l.Open(1)
	l.Append("stale content\n")
	l.MarkDone()

	l.Open(2)
	if l.Done {
		t.Fatal("expected Done reset on Open")
	}
	if len(l.lines) != 0 || l.content != "" {
		t.Fatalf("expected empty buffer after Open, got lines=%+v content=%q", l.lines, l.content)
	}
}
