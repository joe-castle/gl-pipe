package components

import "testing"

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
