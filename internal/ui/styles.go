package ui

import (
	"os"

	"github.com/charmbracelet/lipgloss"

	"github.com/Panonim/search-in-terminal/internal/config"
)

type Styles struct {
	Accent    lipgloss.Style
	Bar       lipgloss.Style
	Title     lipgloss.Style
	TitleSel  lipgloss.Style
	URL       lipgloss.Style
	Snippet   lipgloss.Style
	Dim       lipgloss.Style
	Error     lipgloss.Style
	Tag       lipgloss.Style
	Key       lipgloss.Style
	Box       lipgloss.Style
	BoxActive lipgloss.Style
}

// palette colours a preset; an empty url keeps hostnames faint instead of coloured.
type palette struct {
	accent, snippet, url, border, error string
}

var palettes = map[string]palette{
	"default":    {accent: "#5f9ea0", snippet: "250", border: "8", error: "1"},
	"nord":       {accent: "#88c0d0", snippet: "#d8dee9", url: "#81a1c1", border: "#4c566a", error: "#bf616a"},
	"gruvbox":    {accent: "#fabd2f", snippet: "#ebdbb2", url: "#a89984", border: "#504945", error: "#fb4934"},
	"catppuccin": {accent: "#cba6f7", snippet: "#cdd6f4", url: "#9399b2", border: "#45475a", error: "#f38ba8"},
}

func NewStyles(theme config.Theme) Styles {
	p, ok := palettes[theme.Preset]
	if !ok {
		p = palettes["default"]
	}
	if theme.Accent != "" {
		p.accent = theme.Accent
	}
	accentColor := lipgloss.Color(p.accent)
	dim := lipgloss.NewStyle().Faint(true)
	url := dim
	if p.url != "" {
		url = lipgloss.NewStyle().Foreground(lipgloss.Color(p.url))
	}
	s := Styles{
		Accent:   lipgloss.NewStyle().Foreground(accentColor),
		Bar:      lipgloss.NewStyle().Foreground(accentColor),
		Title:    lipgloss.NewStyle().Bold(true),
		TitleSel: lipgloss.NewStyle().Bold(true).Foreground(accentColor),
		URL:      url,
		Snippet:  lipgloss.NewStyle().Foreground(lipgloss.Color(p.snippet)),
		Dim:      dim,
		Error:    lipgloss.NewStyle().Foreground(lipgloss.Color(p.error)).Faint(true),
		Tag:      lipgloss.NewStyle().Foreground(accentColor),
		Key:      lipgloss.NewStyle().Bold(true),
		Box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(p.border)).
			Padding(0, 1),
		BoxActive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accentColor).
			Padding(0, 1),
	}
	if os.Getenv("NO_COLOR") != "" || theme.Preset == "mono" {
		s.Accent = lipgloss.NewStyle()
		s.Bar = lipgloss.NewStyle()
		s.TitleSel = lipgloss.NewStyle().Bold(true)
		s.URL = dim
		s.Tag = lipgloss.NewStyle()
		s.Error = lipgloss.NewStyle()
		s.Snippet = lipgloss.NewStyle()
		s.Box = s.Box.BorderForeground(lipgloss.NoColor{})
		s.BoxActive = s.BoxActive.BorderForeground(lipgloss.NoColor{})
	}
	return s
}
