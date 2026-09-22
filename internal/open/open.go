// Package open launches URLs in the system browser and copies text to the clipboard.
package open

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"github.com/atotto/clipboard"
)

// URL opens rawURL in the default browser, or with override when the user configured one.
func URL(rawURL, override string) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("refusing to open %q", rawURL)
	}
	if override != "" {
		parts := strings.Fields(override)
		return run(parts[0], append(parts[1:], u.String())...)
	}
	switch runtime.GOOS {
	case "darwin":
		return run("open", u.String())
	case "windows":
		return run("rundll32", "url.dll,FileProtocolHandler", u.String())
	default:
		return run("xdg-open", u.String())
	}
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	go cmd.Wait()
	return nil
}

// Copy puts s on the clipboard, reporting a plain error when no clipboard tool exists.
func Copy(s string) error {
	if clipboard.Unsupported {
		return fmt.Errorf("no clipboard available (install xclip, wl-clipboard or pbcopy)")
	}
	return clipboard.WriteAll(s)
}
