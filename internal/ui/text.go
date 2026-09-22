package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

// wrap breaks s on word boundaries into at most maxLines lines of width w, ellipsising the overflow.
func wrap(s string, w, maxLines int) []string {
	if w <= 0 || maxLines <= 0 || strings.TrimSpace(s) == "" {
		return nil
	}
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		candidate := word
		if line != "" {
			candidate = line + " " + word
		}
		if ansi.StringWidth(candidate) <= w {
			line = candidate
			continue
		}
		if line != "" {
			lines = append(lines, line)
		}
		line = word
		if len(lines) == maxLines {
			break
		}
	}
	if line != "" && len(lines) < maxLines {
		lines = append(lines, line)
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for i := range lines {
		lines[i] = truncate(lines[i], w)
	}
	return lines
}

func padRight(s string, w int) string {
	if gap := w - ansi.StringWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}
