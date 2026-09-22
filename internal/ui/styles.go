package ui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
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

func NewStyles(accent string) Styles {
	plain := os.Getenv("NO_COLOR") != ""
	accentColor := lipgloss.Color(accent)
	dim := lipgloss.NewStyle().Faint(true)
	s := Styles{
		Accent:   lipgloss.NewStyle().Foreground(accentColor),
		Bar:      lipgloss.NewStyle().Foreground(accentColor),
		Title:    lipgloss.NewStyle().Bold(true),
		TitleSel: lipgloss.NewStyle().Bold(true).Foreground(accentColor),
		URL:      dim,
		Snippet:  lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
		Dim:      dim,
		Error:    lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Faint(true),
		Tag:      lipgloss.NewStyle().Foreground(accentColor),
		Key:      lipgloss.NewStyle().Bold(true),
		Box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8")).
			Padding(0, 1),
		BoxActive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accentColor).
			Padding(0, 1),
	}
	if plain {
		s.Accent = lipgloss.NewStyle()
		s.Bar = lipgloss.NewStyle()
		s.TitleSel = lipgloss.NewStyle().Bold(true)
		s.Tag = lipgloss.NewStyle()
		s.Error = lipgloss.NewStyle()
		s.Snippet = lipgloss.NewStyle()
		s.Box = s.Box.BorderForeground(lipgloss.NoColor{})
		s.BoxActive = s.BoxActive.BorderForeground(lipgloss.NoColor{})
	}
	return s
}
