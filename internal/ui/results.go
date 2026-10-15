package ui

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

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
	// Help and settings lay themselves out to the pane height, so a leftover results scroll would cut them off.
	if m.pane != paneResults {
		m.vp.GotoTop()
	}
}

// closePane returns to the results with the selection back in view.
func (m *Model) closePane() {
	m.pane = paneResults
	m.renderContent()
	m.scrollToSelection()
}

func (m *Model) resultLines() ([]string, []int) {
	width := max(20, m.vp.Width)
	if len(m.results) == 0 {
		return append(m.answerLines(width), m.emptyLines()...), nil
	}
	textWidth := width - textIndex

	compact := m.cfg.Theme.Layout == "compact"
	lines := m.answerLines(width)
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
		host := hostLabel(r.URL)
		if m.cfg.Theme.ShowSource && r.Source != "" {
			host += "  ·  " + r.Source
		}
		if compact {
			titleText := truncate(r.Title, textWidth*2/3) + suffix
			row := title.Render(titleText) + "  " + m.styles.URL.Render(host)
			lines = append(lines, prefix+m.icon(r.URL)+" "+truncate(row, textWidth))
			continue
		}
		titleText := truncate(r.Title, textWidth-utf8.RuneCountInString(suffix)) + suffix
		lines = append(lines, prefix+m.icon(r.URL)+" "+title.Render(titleText))
		lines = append(lines, indent+m.styles.URL.Render(truncate(host, textWidth)))
		for _, s := range wrap(r.Snippet, textWidth, m.cfg.Theme.SnippetLines) {
			lines = append(lines, indent+m.styles.Snippet.Render(s))
		}
		lines = append(lines, "")
	}
	if compact {
		lines = append(lines, "")
	}
	starts = append(starts, len(lines))
	label, style := "[ load more results ]", m.styles.Dim
	if m.loading {
		label = "loading…"
	} else if m.sel == len(m.results) {
		style = m.styles.TitleSel
	}
	lines = append(lines, center(style.Render(label), width))
	return lines, starts
}

// answerLines draws the instant answer as a card above the results, or one line in the compact layout.
func (m *Model) answerLines(width int) []string {
	a := m.answer
	if a == nil {
		return nil
	}
	head := a.Title
	if a.Source != "" {
		head += "  ·  " + a.Source
	}
	if m.cfg.Theme.Layout == "compact" {
		row := m.styles.Dim.Render(head) + "  " + m.styles.TitleSel.Render(strings.Join(strings.Fields(a.Body), " "))
		return []string{"  " + truncate(row, width-2), ""}
	}
	inner := max(10, width-6)
	body := []string{m.styles.Dim.Render(truncate(head, inner))}
	for _, l := range strings.Split(a.Body, "\n") {
		for _, s := range wrap(l, inner, maxAnswerLines) {
			if len(body) <= maxAnswerLines {
				body = append(body, m.styles.TitleSel.Render(s))
			}
		}
	}
	if a.URL != "" {
		body = append(body, m.styles.URL.Render(truncate(a.URL, inner)))
	}
	card := m.styles.BoxActive.Render(strings.Join(body, "\n"))
	var out []string
	for _, l := range strings.Split(card, "\n") {
		out = append(out, " "+l)
	}
	return append(out, "")
}

const maxAnswerLines = 8

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
	return strings.Split(pad+center(m.styles.Dim.Render(body), m.vp.Width), "\n")
}

func center(s string, w int) string {
	return strings.Repeat(" ", max(0, (w-ansi.StringWidth(s))/2)) + s
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
	if m.pane != paneResults || len(m.rowStart) == 0 || m.sel >= len(m.rowStart) {
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
