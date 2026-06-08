package ui

import (
	"errors"
	"os/exec"
	"runtime"
)

// openBrowser launches the system default browser with url.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		for _, name := range []string{"xdg-open", "sensible-browser", "wslview"} {
			if _, err := exec.LookPath(name); err == nil {
				cmd = exec.Command(name, url)
				break
			}
		}
	}
	if cmd == nil {
		return errors.New("no browser launcher found")
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}