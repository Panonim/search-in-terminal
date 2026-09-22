// Package img handles terminal graphics detection and favicon rendering.
package img

import (
	"net/url"
	"os"
	"strings"
	"unicode"
)

type Protocol int

const (
	ProtocolNone Protocol = iota
	ProtocolKitty
	ProtocolITerm
	ProtocolSixel
)

func (p Protocol) String() string {
	switch p {
	case ProtocolKitty:
		return "kitty"
	case ProtocolITerm:
		return "iterm2"
	case ProtocolSixel:
		return "sixel"
	default:
		return "none"
	}
}

// Detect picks the best supported protocol from the environment only, never by querying the terminal, because a reply would corrupt Bubble Tea's input.
func Detect() Protocol {
	switch strings.ToLower(os.Getenv("SIT_GRAPHICS")) {
	case "kitty":
		return ProtocolKitty
	case "iterm2", "iterm":
		return ProtocolITerm
	case "sixel":
		return ProtocolSixel
	case "none", "off":
		return ProtocolNone
	}

	term := strings.ToLower(os.Getenv("TERM"))
	if term == "" || term == "dumb" || os.Getenv("CI") != "" {
		return ProtocolNone
	}
	prog := os.Getenv("TERM_PROGRAM")

	switch {
	case strings.Contains(term, "kitty"),
		os.Getenv("KITTY_WINDOW_ID") != "",
		strings.EqualFold(prog, "ghostty"),
		os.Getenv("GHOSTTY_RESOURCES_DIR") != "",
		os.Getenv("WEZTERM_EXECUTABLE") != "":
		return ProtocolKitty
	case prog == "iTerm.app", strings.EqualFold(os.Getenv("LC_TERMINAL"), "iTerm2"):
		return ProtocolITerm
	case strings.Contains(term, "sixel"),
		strings.EqualFold(prog, "mintty"),
		strings.EqualFold(prog, "WezTerm"),
		strings.Contains(strings.ToLower(os.Getenv("COLORTERM")), "sixel"):
		return ProtocolSixel
	}
	return ProtocolNone
}

// Monogram is the fallback glyph when no icon can be drawn.
func Monogram(pageURL string) string {
	host := hostOf(pageURL)
	if host == "" {
		return "•"
	}
	label, _, _ := strings.Cut(host, ".")
	for _, r := range label {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "•"
}

// hostOf returns the lowercased host of pageURL without port or leading "www.".
func hostOf(pageURL string) string {
	pageURL = strings.TrimSpace(pageURL)
	if pageURL == "" {
		return ""
	}
	u, err := url.Parse(pageURL)
	if err != nil || u.Host == "" {
		if u, err = url.Parse("https://" + pageURL); err != nil {
			return ""
		}
	}
	host := strings.ToLower(u.Hostname())
	return strings.TrimPrefix(host, "www.")
}
