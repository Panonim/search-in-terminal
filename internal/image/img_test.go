package img

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestSixelNotOverwrittenBySpaces(t *testing.T) {
	r := NewRenderer(ProtocolSixel, t.TempDir(), 2, true, false, color.NRGBA{A: 0xff})
	seq, err := r.encodeSixel(image.NewRGBA(image.Rect(0, 0, 18, 18)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(seq, "\x1b8\x1b[2C") {
		t.Errorf("image must be followed by a cursor move, not text: %q", seq[len(seq)-10:])
	}
	if w := ansi.StringWidth(seq); w != 2 {
		t.Errorf("width = %d, want 2", w)
	}
}

func TestMonogram(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://example.com/search?q=go", "E"},
		{"https://www.golang.org", "G"},
		{"golang.org", "G"},
		{"https://192.168.0.1:8080/x", "1"},
		{"not a url", "•"},
		{"", "•"},
		{"https://-weird.example", "W"},
	}
	for _, c := range cases {
		if got := Monogram(c.in); got != c.want {
			t.Errorf("Monogram(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestProtocolString(t *testing.T) {
	cases := map[Protocol]string{
		ProtocolNone:  "none",
		ProtocolKitty: "kitty",
		ProtocolITerm: "iterm2",
		ProtocolSixel: "sixel",
		Protocol(42):  "none",
	}
	for p, want := range cases {
		if got := p.String(); got != want {
			t.Errorf("Protocol(%d).String() = %q, want %q", int(p), got, want)
		}
	}
}

func TestDetect(t *testing.T) {
	env := []string{
		"SIT_GRAPHICS", "TERM", "CI", "TERM_PROGRAM", "KITTY_WINDOW_ID",
		"GHOSTTY_RESOURCES_DIR", "WEZTERM_EXECUTABLE", "LC_TERMINAL", "COLORTERM",
	}
	cases := []struct {
		name string
		set  map[string]string
		want Protocol
	}{
		{"override", map[string]string{"SIT_GRAPHICS": "sixel", "TERM": "xterm-kitty"}, ProtocolSixel},
		{"override none", map[string]string{"SIT_GRAPHICS": "none", "TERM": "xterm-kitty"}, ProtocolNone},
		{"auto falls through", map[string]string{"SIT_GRAPHICS": "auto", "TERM": "xterm-kitty"}, ProtocolKitty},
		{"kitty term", map[string]string{"TERM": "xterm-kitty"}, ProtocolKitty},
		{"ghostty", map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "ghostty"}, ProtocolKitty},
		{"wezterm", map[string]string{"TERM": "xterm-256color", "WEZTERM_EXECUTABLE": "/usr/bin/wezterm"}, ProtocolKitty},
		{"iterm", map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "iTerm.app"}, ProtocolITerm},
		{"lc_terminal", map[string]string{"TERM": "xterm-256color", "LC_TERMINAL": "iTerm2"}, ProtocolITerm},
		{"sixel term", map[string]string{"TERM": "xterm-sixel"}, ProtocolSixel},
		{"mintty", map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "mintty"}, ProtocolSixel},
		{"plain", map[string]string{"TERM": "xterm-256color"}, ProtocolNone},
		{"dumb", map[string]string{"TERM": "dumb", "KITTY_WINDOW_ID": "1"}, ProtocolNone},
		{"ci", map[string]string{"TERM": "xterm-kitty", "CI": "true"}, ProtocolNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, k := range env {
				t.Setenv(k, c.set[k])
			}
			if got := Detect(); got != c.want {
				t.Errorf("Detect() = %v, want %v", got, c.want)
			}
		})
	}
}
