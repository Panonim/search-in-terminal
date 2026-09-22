package ui

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	img "github.com/Panonim/search-in-terminal/internal/image"
)

const (
	barWidth  = 2
	iconWidth = 2
	textIndex = barWidth + iconWidth + 1
)

func (m *Model) renderContent() {
	switch m.pane {
	case paneHelp:
		m.vp.SetContent(m.helpView())
	case paneSettings:
		m.vp.SetContent(m.settings.view(m.vp.Width, m.vp.Height))
	default:
		lines, starts := m.resultLines()
		m.rowStart = starts
		m.vp.SetContent(strings.Join(lines, "\n"))
	}
}

func (m *Model) resultLines() ([]string, []int) {
	if len(m.results) == 0 {
		return m.emptyLines(), nil
	}
	width := max(20, m.vp.Width)
	textWidth := width - textIndex

	var lines []string
	starts := make([]int, 0, len(m.results))
	for i, r := range m.results {
		starts = append(starts, len(lines))
		bar, title := " ", m.styles.Title
		if i == m.sel {
			bar, title = m.styles.Bar.Render("▌"), m.styles.TitleSel
		}
		prefix := bar + " "
		indent := prefix + strings.Repeat(" ", iconWidth+1)

		suffix := ""
		if r.Cached {
			suffix = " ᶻ 𝗓 𐰁"
		}
		titleText := truncate(r.Title, textWidth-utf8.RuneCountInString(suffix)) + suffix
		lines = append(lines, prefix+m.icon(r.URL)+" "+title.Render(titleText))
		host := hostLabel(r.URL)
		if m.cfg.Theme.ShowSource && r.Source != "" {
			host += "  ·  " + r.Source
		}
		lines = append(lines, indent+m.styles.URL.Render(truncate(host, textWidth)))
		for _, s := range wrap(r.Snippet, textWidth, m.cfg.Theme.SnippetLines) {
			lines = append(lines, indent+m.styles.Snippet.Render(s))
		}
		lines = append(lines, "")
	}
	if m.loading {
		lines = append(lines, m.styles.Dim.Render("  loading page "+fmt.Sprint(m.page+1)+"…"))
	}
	return lines, starts
}

func (m *Model) emptyLines() []string {
	pad := strings.Repeat("\n", max(0, m.vp.Height/3))
	var body string
	switch {
	case m.loading:
		body = "searching…"
	case m.query == "":
		body = "type a query and press enter"
	case m.errMsg != "":
		body = "nothing to show"
	default:
		body = fmt.Sprintf("no results for %q - try another backend with %s", m.query, m.cfg.Keys.NextBackend)
	}
	centered := strings.Repeat(" ", max(0, (m.vp.Width-len(body))/2)) + m.styles.Dim.Render(body)
	return strings.Split(pad+centered, "\n")
}

// icon returns a favicon escape sequence or a monogram, always exactly iconWidth cells wide.
func (m *Model) icon(pageURL string) string {
	if m.renderer.Enabled() {
		if seq, ok := m.renderer.Cell(pageURL); ok {
			return seq
		}
	}
	return m.styles.Accent.Render(img.Monogram(pageURL)) + " "
}

func (m *Model) scrollToSelection() {
	if len(m.rowStart) == 0 || m.sel >= len(m.rowStart) {
		return
	}
	start := m.rowStart[m.sel]
	end := m.vp.TotalLineCount()
	if m.sel+1 < len(m.rowStart) {
		end = m.rowStart[m.sel+1]
	}
	if start < m.vp.YOffset {
		m.vp.SetYOffset(start)
		return
	}
	if end > m.vp.YOffset+m.vp.Height {
		m.vp.SetYOffset(min(start, max(0, end-m.vp.Height)))
	}
}

func hostLabel(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	host := strings.TrimPrefix(u.Host, "www.")
	if p := strings.TrimSuffix(u.Path, "/"); p != "" {
		return host + p
	}
	return host
}
