package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Panonim/search-in-terminal/internal/backend"
	"github.com/Panonim/search-in-terminal/internal/config"
)

type settingsModel struct {
	cfg     config.Config
	fields  []config.Field
	idx     int
	editing bool
	dirty   bool
	msg     string
	input   textinput.Model
	styles  Styles
}

func newSettings(cfg config.Config, styles Styles) settingsModel {
	in := textinput.New()
	in.Prompt = "› "
	in.PromptStyle = styles.Accent
	return settingsModel{cfg: cfg, fields: config.Fields(), input: in, styles: styles}
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
		m.pane = paneResults
		m.renderContent()
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
	case " ", "right", "l":
		_ = field.Cycle(&s.cfg, 1)
		s.dirty = true
	case "left", "h":
		_ = field.Cycle(&s.cfg, -1)
		s.dirty = true
	case "ctrl+s", "w":
		return m.applySettings(true)
	case "ctrl+r":
		s.cfg = config.Default()
		s.dirty = true
		s.msg = "defaults restored (not saved yet)"
	}
	m.renderContent()
	return m, nil
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
	var b strings.Builder
	b.WriteString("  " + s.styles.Title.Render("settings") + s.styles.Dim.Render("  ·  "+config.Path()) + "\n\n")

	rows := max(3, height-6)
	start := max(0, min(s.idx-rows/2, len(s.fields)-rows))
	end := min(start+rows, len(s.fields))
	keyWidth := 28

	for i := start; i < end; i++ {
		f := s.fields[i]
		bar, key := " ", s.styles.Dim
		if i == s.idx {
			bar, key = s.styles.Bar.Render("▌"), s.styles.Title
		}
		value := f.Display(&s.cfg)
		if i == s.idx && s.editing {
			value = s.input.View()
		}
		line := bar + " " + key.Render(padRight(f.Key, keyWidth)) + s.styles.Accent.Render(value)
		b.WriteString(truncate(line, width) + "\n")
		if i == s.idx && !s.editing {
			b.WriteString("  " + strings.Repeat(" ", keyWidth) + s.styles.Dim.Render(truncate(f.Desc, max(10, width-keyWidth-4))) + "\n")
		}
	}

	b.WriteString("\n")
	status := "enter edit/toggle · ←→ cycle · ctrl+s save · ctrl+r defaults · esc close"
	if s.dirty {
		status = "unsaved changes · " + status
	}
	if s.msg != "" {
		status = s.msg
	}
	b.WriteString("  " + s.styles.Dim.Render(truncate(status, max(10, width-4))))
	return b.String()
}
