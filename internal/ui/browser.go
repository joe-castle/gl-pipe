package ui

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openBrowser opens url in the user's default browser, per <Space> o / gx.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening browser for %s: %w", url, err)
	}
	return nil
}
