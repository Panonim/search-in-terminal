package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Panonim/search-in-terminal/internal/backend"
	"github.com/Panonim/search-in-terminal/internal/config"
)

type settingsModel struct {
	cfg      config.Config
	fields   []config.Field
	sections []section
	idx      int
	editing  bool
	dirty    bool
	msg      string
	input    textinput.Model
	styles   Styles
}

// section is a run of fields sharing a key prefix, e.g. backends.degoog.
type section struct {
	prefix, name string
	start, end   int
}

func (sec section) height() int { return sec.end - sec.start + 3 }

func (sec section) has(i int) bool { return i >= sec.start && i < sec.end }

func newSettings(cfg config.Config, styles Styles) settingsModel {
	in := textinput.New()
	in.Prompt = "› "
	in.PromptStyle = styles.Accent
	fields := config.Fields()
	return settingsModel{cfg: cfg, fields: fields, sections: sections(fields), input: in, styles: styles}
}

func sections(fields []config.Field) []section {
	var out []section
	for i, f := range fields {
		prefix := f.Key[:strings.LastIndex(f.Key, ".")]
		if len(out) == 0 || out[len(out)-1].prefix != prefix {
			out = append(out, section{prefix: prefix, name: prefix[strings.LastIndex(prefix, ".")+1:], start: i})
		}
		out[len(out)-1].end = i + 1
	}
	return out
}

func (s settingsModel) section() int {
	for i, sec := range s.sections {
		if s.idx < sec.end {
			return i
		}
	}
	return 0
}

func (m Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.settings
	field := s.fields[s.idx]

	if s.editing {
		switch msg.String() {
		case "enter":
			if err := field.Set(&s.cfg, s.input.Value()); err != nil {
				s.msg = err.Error()
			} else {
				s.dirty = true
				s.msg = ""
			}
			s.editing = false
		case "esc":
			s.editing = false
		default:
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			m.renderContent()
			return m, cmd
		}
		m.renderContent()
		return m, nil
	}

	switch msg.String() {
	case "esc", m.cfg.Keys.Settings, "q":
		m.closePane()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "j", "down", "ctrl+n":
		s.idx = min(s.idx+1, len(s.fields)-1)
	case "k", "up", "ctrl+p":
		s.idx = max(s.idx-1, 0)
	case "g", "home":
		s.idx = 0
	case "G", "end":
		s.idx = len(s.fields) - 1
	case "tab":
		s.jumpSection(1)
	case "shift+tab":
		s.jumpSection(-1)
	case "right", "l":
		s.moveAcross(1, m.vp.Width)
	case "left", "h":
		s.moveAcross(-1, m.vp.Width)
	case "enter":
		if field.Kind == config.KindBool || field.Kind == config.KindEnum {
			_ = field.Cycle(&s.cfg, 1)
			s.dirty = true
			break
		}
		s.editing = true
		s.input.SetValue(field.Get(&s.cfg))
		s.input.CursorEnd()
		s.input.Focus()
		m.renderContent()
		return m, textinput.Blink
	case " ":
		_ = field.Cycle(&s.cfg, 1)
		s.dirty = true
	case "ctrl+s", "w":
		return m.applySettings(true)
	case "ctrl+r":
		s.cfg = config.Default()
		s.dirty = true
		s.msg = "defaults restored (not saved yet)"
	default:
		if n := int(msg.String()[0] - '0'); len(msg.String()) == 1 && n >= 1 && n <= len(s.sections) {
			s.idx = s.sections[n-1].start
		}
	}
	m.renderContent()
	return m, nil
}

func (s *settingsModel) jumpSection(dir int) {
	s.idx = s.sections[(s.section()+dir+len(s.sections))%len(s.sections)].start
}

// moveAcross picks the field in the neighbouring column closest to the current row, or the next section in a single column.
func (s *settingsModel) moveAcross(dir, width int) {
	col, line := s.place(width)
	best := -1
	for i := range s.fields {
		if col[i] == col[s.idx]+dir && (best < 0 || abs(line[i]-line[s.idx]) < abs(line[best]-line[s.idx])) {
			best = i
		}
	}
	if best < 0 {
		s.jumpSection(dir)
		return
	}
	s.idx = best
}

// place returns the column and body line of every field, counting the hint under the selected one.
func (s settingsModel) place(width int) (col, line []int) {
	cols, cardWidth := s.layout(width)
	extra := len(s.hint(cardWidth))
	col, line = make([]int, len(s.fields)), make([]int, len(s.fields))
	for c, secs := range cols {
		y := 0
		for _, sec := range secs {
			for i := sec.start; i < sec.end; i++ {
				col[i], line[i] = c, y+2+i-sec.start
				if i > s.idx && sec.has(s.idx) {
					line[i] += extra
				}
			}
			y += sec.height()
			if sec.has(s.idx) {
				y += extra
			}
		}
	}
	return col, line
}

func (s settingsModel) layout(width int) ([][]section, int) {
	cols := columns(s.sections, max(1, min(3, width/44)))
	return cols, (width - len(cols) + 1) / len(cols)
}

// hint wraps the selected field's description to sit under it inside a card.
func (s settingsModel) hint(cardWidth int) []string {
	return wrap(s.fields[s.idx].Desc, max(1, cardWidth-8), 3)
}

// applySettings writes the edited config to disk and rebuilds anything derived from it.
func (m Model) applySettings(save bool) (tea.Model, tea.Cmd) {
	cfg := m.settings.cfg
	if save {
		if err := cfg.Save(); err != nil {
			m.settings.msg = err.Error()
			m.renderContent()
			return m, nil
		}
	}
	backendChanged := cfg.General.Backend != m.cfg.General.Backend
	m.cfg = cfg
	m.styles = NewStyles(cfg.Theme.Accent)
	m.spin.Style = m.styles.Accent
	m.settings.styles = m.styles
	m.settings.dirty = false
	m.settings.msg = "saved to " + config.Path()
	m.renderer = newRenderer(cfg, m.bg)

	var cmd tea.Cmd
	if backendChanged {
		if b, err := backend.New(cfg.General.Backend, cfg); err == nil {
			m.backend = b
			if m.query != "" {
				cmd = m.search(m.query, 1, false)
			}
		}
	}
	m.renderContent()
	return m, cmd
}

func (s settingsModel) view(width, height int) string {
	header := " " + s.styles.Title.Render("settings") + s.styles.Dim.Render("  ·  "+config.Path())
	if s.dirty {
		header += s.styles.Accent.Render("  ● unsaved")
	}

	cols, cardWidth := s.layout(width)
	n := 0
	var blocks []string
	for i, col := range cols {
		var cards []string
		for _, sec := range col {
			n++
			cards = append(cards, s.card(sec, n, cardWidth))
		}
		if i > 0 {
			blocks = append(blocks, " ")
		}
		blocks = append(blocks, lipgloss.JoinVertical(lipgloss.Left, cards...))
	}

	// Keep the selected row centred when the cards are taller than the pane.
	_, line := s.place(width)
	body := strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, blocks...), "\n")
	rows := max(3, height-3)
	start := max(0, min(line[s.idx]-rows/2, len(body)-rows))
	body = body[start:min(start+rows, len(body))]
	for len(body) < rows {
		body = append(body, "")
	}

	status := fmt.Sprintf("↑↓←→ move · 1-%d section · enter edit · space toggle · ctrl+s save · ctrl+r defaults · esc close", len(s.sections))
	if s.msg != "" {
		status = s.msg
	}
	lines := append([]string{header, ""}, body...)
	lines = append(lines, " "+s.styles.Dim.Render(status))
	for i := range lines {
		lines[i] = truncate(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

// columns splits sections into at most n columns of similar height while keeping their reading order.
func columns(secs []section, n int) [][]section {
	total := 0
	for _, sec := range secs {
		total += sec.height()
	}
	cols, h := [][]section{nil}, 0
	for _, sec := range secs {
		if h > 0 && len(cols) < n && abs((h+sec.height())*n-total) > abs(h*n-total) {
			cols, h = append(cols, nil), 0
		}
		cols[len(cols)-1] = append(cols[len(cols)-1], sec)
		h += sec.height()
	}
	return cols
}

func abs(n int) int { return max(n, -n) }

func (s settingsModel) card(sec section, n, width int) string {
	inner := max(1, width-4)
	isBackend := strings.HasPrefix(sec.prefix, "backends.")
	inUse := isBackend && sec.name == s.cfg.General.Backend

	title := s.styles.Title.Render(sec.name)
	if isBackend && !inUse {
		title = s.styles.Dim.Render(sec.name)
	}
	title = s.styles.Dim.Render(fmt.Sprint(n)) + " " + title

	keyWidth := 0
	for _, f := range s.fields[sec.start:sec.end] {
		keyWidth = max(keyWidth, len(f.Key)-len(sec.prefix)+1)
	}
	lines := []string{title}
	for i := sec.start; i < sec.end; i++ {
		f := s.fields[i]
		bar, key := " ", s.styles.Dim
		if i == s.idx {
			bar, key = s.styles.Bar.Render("▌"), s.styles.TitleSel
		}
		value := s.value(f, isBackend && !inUse)
		if i == s.idx && s.editing {
			s.input.Width = max(1, inner-keyWidth-5)
			value = s.input.View()
		}
		lines = append(lines, truncate(bar+" "+key.Render(padRight(f.Key[len(sec.prefix)+1:], keyWidth))+value, inner))
		if i == s.idx {
			for j, h := range s.hint(width) {
				mark := "  "
				if j == 0 {
					mark = s.styles.Accent.Render("↳ ")
				}
				lines = append(lines, "  "+mark+s.styles.Snippet.Render(h))
			}
		}
	}

	box := s.styles.Box
	if sec.has(s.idx) || inUse && s.fields[s.idx].Key == "general.backend" {
		box = s.styles.BoxActive
	}
	return box.Width(width - 2).Render(strings.Join(lines, "\n"))
}

func (s settingsModel) value(f config.Field, faded bool) string {
	v := f.Display(&s.cfg)
	if faded || v == "-" || v == "false" {
		return s.styles.Dim.Render(v)
	}
	return s.styles.Accent.Render(v)
}
