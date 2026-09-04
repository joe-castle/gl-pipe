package components

import (
	"strings"
	"testing"
)

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

// The reported failure: nothing in "mvn: command not found" matches
// error|failed|fatal|panic|exception, so first-match found only GitLab's own
// trailing "ERROR: Job failed" line and reported the exit code as the cause.
func TestFailureSummary_FindsACommandNotFoundAtTheTail(t *testing.T) {
	trace := strings.Join([]string{
		"Running with gitlab-runner 16.9.0",
		`Preparing the "docker" executor`,
		"Getting source from Git repository",
		"Checking out abc123 as main...",
		`Executing "step_script" stage of the job script`,
		"$ mvn clean install",
		"/bin/bash: line 123: mvn: command not found",
		"Cleaning up project directory and file based variables",
		"ERROR: Job failed: exit code 127",
	}, "\n")

	got := FailureSummary(trace, 3)
	joined := strings.Join(got, " | ")
	if !strings.Contains(joined, "mvn: command not found") {
		t.Fatalf("summary missed the real cause: %q", got)
	}
	if strings.Contains(joined, "ERROR: Job failed") {
		t.Errorf("summary should not report the runner's own epilogue: %q", got)
	}
	if strings.Contains(joined, "Cleaning up project directory") {
		t.Errorf("summary should not report runner boilerplate: %q", got)
	}
}

func TestFailureSummary_KeepsTheFailingCommandAsContext(t *testing.T) {
	trace := "$ ./gradlew build\nsome output\nBUILD FAILED in 3s\nERROR: Job failed: exit code 1"
	got := FailureSummary(trace, 3)
	if !strings.Contains(strings.Join(got, " | "), "BUILD FAILED") {
		t.Fatalf("summary = %q", got)
	}
}

func TestFailureSummary_DropsArtifactUploadNoiseAfterTheFailure(t *testing.T) {
	trace := strings.Join([]string{
		"$ mvn test",
		"[ERROR] Tests run: 4, Failures: 1",
		"Uploading artifacts for failed job",
		"WARNING: target/surefire-reports/: no matching files",
		"ERROR: Job failed: exit code 1",
	}, "\n")

	got := FailureSummary(trace, 3)
	joined := strings.Join(got, " | ")
	if !strings.Contains(joined, "Tests run: 4, Failures: 1") {
		t.Fatalf("summary lost the real failure behind upload noise: %q", got)
	}
	if strings.Contains(joined, "no matching files") {
		t.Errorf("artifact upload warnings are not the cause: %q", got)
	}
}

func TestFailureSummary_StripsANSI(t *testing.T) {
	trace := "$ build\n\x1b[0;31m/bin/sh: gcc: not found\x1b[0m\nERROR: Job failed: exit code 127"
	got := FailureSummary(trace, 2)
	for _, l := range got {
		if strings.Contains(l, "\x1b") {
			t.Fatalf("escape survived stripping: %q", got)
		}
	}
}

func TestFailureSummary_EmptyTraceYieldsNothing(t *testing.T) {
	if got := FailureSummary("", 3); len(got) != 0 {
		t.Errorf("FailureSummary = %q, want empty", got)
	}
}

// A trace that is nothing but boilerplate must not come back claiming the
// runner epilogue is the cause.
func TestFailureSummary_AllBoilerplateYieldsNothing(t *testing.T) {
	trace := "Running with gitlab-runner 16.9.0\nCleaning up project directory and file based variables\nERROR: Job failed: exit code 1"
	if got := FailureSummary(trace, 3); len(got) != 0 {
		t.Errorf("FailureSummary = %q, want empty", got)
	}
}

func TestTraceFailureReason_ExtractsTheExitCode(t *testing.T) {
	if got := TraceFailureReason("blah\nERROR: Job failed: exit code 127"); got != "exit code 127" {
		t.Errorf("TraceFailureReason = %q, want %q", got, "exit code 127")
	}
	if got := TraceFailureReason("blah\nERROR: Job failed (system failure): pod not found"); got != "system failure: pod not found" {
		t.Errorf("TraceFailureReason = %q", got)
	}
	if got := TraceFailureReason("no verdict here"); got != "" {
		t.Errorf("TraceFailureReason = %q, want empty", got)
	}
}
