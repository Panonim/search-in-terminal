package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestWrap(t *testing.T) {
	got := wrap("the quick brown fox jumps over the lazy dog", 12, 2)
	if len(got) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(got), got)
	}
	for _, line := range got {
		if w := ansi.StringWidth(line); w > 12 {
			t.Errorf("line %q is %d cells wide", line, w)
		}
	}
	if wrap("anything", 0, 2) != nil || wrap("  ", 10, 2) != nil {
		t.Error("degenerate inputs should wrap to nothing")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abcdefghij", 5); ansi.StringWidth(got) > 5 {
		t.Errorf("truncate too wide: %q", got)
	}
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("short strings must be untouched, got %q", got)
	}
}

func TestPadRight(t *testing.T) {
	if got := ansi.StringWidth(padRight("ab", 6)); got != 6 {
		t.Errorf("padded width = %d", got)
	}
	if got := padRight("abcdef", 3); got != "abcdef" {
		t.Errorf("padding must not truncate, got %q", got)
	}
}

func TestHostLabel(t *testing.T) {
	cases := map[string]string{
		"https://www.example.com/":          "example.com",
		"https://go.dev/doc/":               "go.dev/doc",
		"https://en.wikipedia.org/wiki/Foo": "en.wikipedia.org/wiki/Foo",
	}
	for in, want := range cases {
		if got := hostLabel(in); got != want {
			t.Errorf("hostLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
