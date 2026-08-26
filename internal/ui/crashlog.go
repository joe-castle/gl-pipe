package ui

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Panic containment for tea.Cmds.
//
// Invariant #1 puts every GitLab call inside a tea.Cmd closure, which
// bubbletea runs on its own goroutine (handleCommands). A panic in any of
// them — a nil map, an unexpected nil pointer in an API response, a bug in
// the SDK — was fatal to the whole session, and worse, unreadable: with
// tea.WithAltScreen() the "Caught panic: ..." line bubbletea prints goes to
// the *program output*, i.e. into the alternate screen buffer, which the
// terminal discards on restore. The user was left staring at a bare stack
// trace with no message, unable to scroll back, in a terminal whose modes
// hadn't been cleanly torn down.
//
// safeCmd turns that into an ordinary error in the status bar and appends
// the full report (panic value + stack) to a file that outlives the alt
// screen.

var (
	crashMu       sync.Mutex
	crashLogPath  string
	crashLogCount int
)

// SetCrashLog points panic reports at path and resets the session's report
// counter. Called once by New; tests use it to redirect the log.
func SetCrashLog(path string) {
	crashMu.Lock()
	defer crashMu.Unlock()
	crashLogPath = path
	crashLogCount = 0
}

// CrashReports returns where panic reports are written and how many have
// been recorded this session — main uses it to point at the log on exit,
// once the terminal is back in a state where the message can actually be
// read.
func CrashReports() (path string, count int) {
	crashMu.Lock()
	defer crashMu.Unlock()
	return crashLogPath, crashLogCount
}

// recordPanic appends one panic report to the crash log. Failing to write
// it is deliberately silent: this runs inside a recover, and a second
// failure here must not take down the process the recover just saved.
func recordPanic(r any, stack []byte) {
	crashMu.Lock()
	defer crashMu.Unlock()
	crashLogCount++
	if crashLogPath == "" {
		return
	}
	f, err := os.OpenFile(crashLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "\n=== gl-pipe panic %s ===\n%v\n\n%s\n",
		time.Now().Format(time.RFC3339), r, stack)
}

// safeCmd wraps a Cmd body so a panic inside it becomes an errMsg (shown in
// the status bar, which also clears the loading state via setErr) instead of
// killing the program. Every Cmd factory in commands.go goes through this.
func safeCmd(fn func() tea.Msg) tea.Cmd {
	return func() (msg tea.Msg) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			recordPanic(r, debug.Stack())
			path, _ := CrashReports()
			if path == "" {
				msg = errMsg{err: fmt.Errorf("internal error: %v", r)}
				return
			}
			msg = errMsg{err: fmt.Errorf("internal error: %v (details in %s)", r, path)}
		}()
		return fn()
	}
}

// safeGo runs fn on its own goroutine with the same panic containment.
// Used for the log-streaming producer, which is started by a Cmd but
// outlives it and so isn't covered by safeCmd's recover.
func safeGo(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				recordPanic(r, debug.Stack())
			}
		}()
		fn()
	}()
}
