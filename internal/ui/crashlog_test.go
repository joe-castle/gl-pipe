package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSafeCmd_PassesNormalMessagesThrough(t *testing.T) {
	SetCrashLog(filepath.Join(t.TempDir(), "crash.log"))

	msg := safeCmd(func() tea.Msg { return tickMsg(time.Now()) })()
	if _, ok := msg.(tickMsg); !ok {
		t.Fatalf("safeCmd changed the message: got %T, want tickMsg", msg)
	}
	if _, n := CrashReports(); n != 0 {
		t.Errorf("CrashReports count = %d, want 0 for a Cmd that didn't panic", n)
	}
}

func TestSafeCmd_TurnsAPanicIntoAnErrMsg(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.log")
	SetCrashLog(path)

	msg := safeCmd(func() tea.Msg { panic("boom in a cmd") })()

	em, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("got %T, want errMsg — a panicking Cmd must not escape into bubbletea", msg)
	}
	if em.err == nil {
		t.Fatal("errMsg.err is nil")
	}
	if !strings.Contains(em.err.Error(), "boom in a cmd") {
		t.Errorf("error %q doesn't name the panic value", em.err)
	}
	if !strings.Contains(em.err.Error(), path) {
		t.Errorf("error %q doesn't point at the crash log", em.err)
	}
}

func TestSafeCmd_WritesPanicAndStackToTheCrashLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.log")
	SetCrashLog(path)

	safeCmd(func() tea.Msg { panic("boom with a stack") })()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading crash log: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "boom with a stack") {
		t.Errorf("crash log doesn't contain the panic value:\n%s", body)
	}
	// The whole point is the frames the alt screen swallowed — the report is
	// useless without them.
	if !strings.Contains(body, "gl-pipe/internal/ui.TestSafeCmd_WritesPanicAndStackToTheCrashLog") {
		t.Errorf("crash log doesn't contain the panicking call stack:\n%s", body)
	}
	if _, n := CrashReports(); n != 1 {
		t.Errorf("CrashReports count = %d, want 1", n)
	}
}

func TestCrashLog_AppendsRatherThanOverwriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.log")
	SetCrashLog(path)

	safeCmd(func() tea.Msg { panic("first panic") })()
	safeCmd(func() tea.Msg { panic("second panic") })()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading crash log: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "first panic") || !strings.Contains(body, "second panic") {
		t.Errorf("crash log lost an earlier report:\n%s", body)
	}
	if _, n := CrashReports(); n != 2 {
		t.Errorf("CrashReports count = %d, want 2", n)
	}
}

func TestSafeCmd_SurvivesAnUnwritableCrashLogPath(t *testing.T) {
	// A crash report that can't be persisted must still be reported in-app
	// rather than panicking a second time inside the recover.
	SetCrashLog(filepath.Join(t.TempDir(), "no-such-dir", "crash.log"))

	msg := safeCmd(func() tea.Msg { panic("unwritable") })()
	if _, ok := msg.(errMsg); !ok {
		t.Fatalf("got %T, want errMsg", msg)
	}
}

// TestJobsForPipelineCmd_ContainsAPanicFromTheAPILayer is the regression
// guard for the reported crash: a panic anywhere under a Cmd (here, a nil
// client dereferenced inside internal/api) took the whole program down,
// printing its trace into the alt screen where it was unreadable.
func TestJobsForPipelineCmd_ContainsAPanicFromTheAPILayer(t *testing.T) {
	SetCrashLog(filepath.Join(t.TempDir(), "crash.log"))

	msg := jobsForPipelineCmd(context.Background(), nil, 1, 2, 3)()
	if _, ok := msg.(errMsg); !ok {
		t.Fatalf("got %T, want errMsg — the Cmd wrapper isn't applied", msg)
	}
	if _, n := CrashReports(); n != 1 {
		t.Errorf("CrashReports count = %d, want 1", n)
	}
}

func TestSetCrashLog_ResetsTheCounter(t *testing.T) {
	SetCrashLog(filepath.Join(t.TempDir(), "crash.log"))
	safeCmd(func() tea.Msg { panic("counted") })()

	SetCrashLog(filepath.Join(t.TempDir(), "crash.log"))
	path, n := CrashReports()
	if n != 0 {
		t.Errorf("count = %d, want 0 after re-pointing the crash log", n)
	}
	if path == "" {
		t.Error("CrashReports path is empty")
	}
}
