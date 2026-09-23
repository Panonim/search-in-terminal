package ui

import (
	"fmt"
	"strings"

	"github.com/Panonim/search-in-terminal/internal/config"
)

func (m *Model) helpView() string {
	k := m.cfg.Keys
	rows := [][2]string{
		{k.Focus, "focus the search input"},
		{"enter", "search, open the selected result, or load more"},
		{k.NextBackend + " / " + k.PrevBackend, "cycle backend, then enter to search it"},
		{k.Down + " " + k.Up + " / ↓ ↑", "move selection"},
		{"ctrl+d / ctrl+u", "page through results"},
		{k.NextPage + " / " + k.PrevPage, "next / previous result page"},
		{k.Copy, "copy the selected URL"},
		{"r", "retry the current search"},
		{k.Settings, "settings"},
		{k.Help, "toggle this help"},
		{k.Quit, "quit"},
	}

	var b strings.Builder
	b.WriteString("  " + m.styles.Title.Render("sit") + m.styles.Dim.Render(" - search the web without leaving your terminal") + "\n")
	b.WriteString("  " + m.styles.Dim.Render(fmt.Sprintf("%s · backend %s · graphics %s",
		m.version, m.backend.Label(), m.renderer.Protocol())) + "\n\n")
	for _, r := range rows {
		b.WriteString("  " + m.styles.Key.Render(padRight(r[0], 18)) + m.styles.Dim.Render(r[1]) + "\n")
	}
	b.WriteString("\n  " + m.styles.Dim.Render("config: "+config.Path()) + "\n")
	return b.String()
}
