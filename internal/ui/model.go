// Package ui implements sit's Bubble Tea interface.
package ui

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/Panonim/search-in-terminal/internal/backend"
	"github.com/Panonim/search-in-terminal/internal/config"
	img "github.com/Panonim/search-in-terminal/internal/image"
	"github.com/Panonim/search-in-terminal/internal/open"
)

type pane int

const (
	paneResults pane = iota
	paneHelp
	paneSettings
)

type Model struct {
	cfg      config.Config
	version  string
	styles   Styles
	backend  backend.Backend
	renderer *img.Renderer

	input textinput.Model
	spin  spinner.Model
	vp    viewport.Model

	results []backend.Result
	sel     int
	page    int
	query   string
	pane    pane

	loading bool
	reqID   int
	errMsg  string
	status  string
	bg      color.NRGBA
	// initCmd is built in New because Init's value receiver would drop search's reqID/loading updates.
	initCmd tea.Cmd

	settings settingsModel
	rowStart []int
	width    int
	height   int
	ready    bool
}

type resultsMsg struct {
	reqID   int
	page    int
	append  bool
	results []backend.Result
	err     error
}

type faviconMsg struct{ url string }

type suggestionsMsg struct {
	query string
	items []string
}

type statusMsg struct{ text string }

type clearStatusMsg struct{ token string }

func New(cfg config.Config, version, initialQuery string) (Model, error) {
	b, err := backend.New(cfg.General.Backend, cfg)
	if err != nil {
		return Model{}, err
	}
	styles := NewStyles(cfg.Theme.Accent)

	in := textinput.New()
	in.Placeholder = "search the web…"
	in.Prompt = ""
	in.Focus()
	in.SetValue(initialQuery)
	in.CursorEnd()
	in.ShowSuggestions = true
	// Tab cycles backends, so completion is accepted with ctrl+e or right instead.
	in.KeyMap.AcceptSuggestion = key.NewBinding(key.WithKeys("ctrl+e", "right"))

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styles.Accent

	bg := termBackground()
	m := Model{
		cfg:      cfg,
		version:  version,
		styles:   styles,
		backend:  b,
		bg:       bg,
		renderer: newRenderer(cfg, bg),
		input:    in,
		spin:     sp,
		vp:       viewport.New(80, 20),
		page:     1,
		settings: newSettings(cfg, styles),
	}
	if initialQuery != "" {
		m.query = initialQuery
		m.initCmd = m.search(m.query, 1, false)
	}
	return m, nil
}

// termBackground must run before Bubble Tea owns stdin, so the terminal's reply cannot reach its input.
func termBackground() color.NRGBA {
	r, g, b := termenv.ConvertToRGB(termenv.NewOutput(os.Stdout).BackgroundColor()).RGB255()
	return color.NRGBA{r, g, b, 0xff}
}

func newRenderer(cfg config.Config, bg color.NRGBA) *img.Renderer {
	dir := config.FaviconDir()
	if !cfg.General.Cache {
		dir = ""
	}
	return img.NewRenderer(img.Detect(), dir, iconWidth, cfg.Theme.Icons, cfg.Theme.IconBackdrop, bg)
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spin.Tick, m.initCmd)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		m.layout()
		m.renderContent()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case resultsMsg:
		return m.handleResults(msg)

	case faviconMsg:
		m.renderContent()
		return m, nil

	case suggestionsMsg:
		if msg.query == m.input.Value() {
			m.input.SetSuggestions(msg.items)
		}
		return m, nil

	case statusMsg:
		m.status = msg.text
		token := msg.text
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{token} })

	case clearStatusMsg:
		if m.status == msg.token {
			m.status = ""
		}
		return m, nil

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.onMoreButton(msg.Y) {
			return m, m.loadMore()
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleResults(msg resultsMsg) (tea.Model, tea.Cmd) {
	if msg.reqID != m.reqID {
		return m, nil
	}
	m.loading = false
	if msg.err != nil {
		m.errMsg = msg.err.Error()
		m.renderContent()
		return m, nil
	}
	m.errMsg = ""
	if msg.append {
		fresh := unseen(m.results, msg.results)
		// A page of only repeats still advances, so the next press asks for the page after it.
		if len(msg.results) > 0 {
			m.page = msg.page
		}
		if len(fresh) == 0 {
			m.renderContent()
			return m, statusCmd("no new results")
		}
		m.sel = len(m.results)
		m.results = append(m.results, fresh...)
		msg.results = fresh
	} else {
		m.results = msg.results
		m.sel = 0
		m.page = msg.page
		m.vp.GotoTop()
	}
	m.input.Blur()
	m.renderContent()
	m.scrollToSelection()
	return m, m.prefetchIcons(msg.results)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	k := m.cfg.Keys

	if m.pane == paneSettings {
		return m.updateSettings(msg)
	}
	if m.pane == paneHelp {
		switch key {
		case k.Help, "esc", "q", "enter":
			m.closePane()
			return m, nil
		}
	}

	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.input.Focused() {
			m.input.Blur()
			return m, nil
		}
		m.closePane()
		return m, nil
	case k.Focus:
		if !m.input.Focused() {
			m.input.Focus()
			return m, textinput.Blink
		}
	case k.NextBackend:
		return m.switchBackend(1)
	case k.PrevBackend:
		return m.switchBackend(-1)
	}

	if m.input.Focused() {
		switch key {
		case "enter":
			q := strings.TrimSpace(m.input.Value())
			if q == "" {
				return m, nil
			}
			m.query = q
			return m, m.search(q, 1, false)
		case "up", "down", "pgup", "pgdown", "ctrl+d", "ctrl+u":
			return m.navigate(key)
		}
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if after := m.input.Value(); after != before {
			return m, tea.Batch(cmd, m.suggest(after))
		}
		return m, cmd
	}

	switch key {
	case k.Quit, "q":
		return m, tea.Quit
	case k.Help:
		m.pane = paneHelp
		m.renderContent()
		return m, nil
	case k.Settings:
		m.pane = paneSettings
		m.settings = newSettings(m.cfg, m.styles)
		m.renderContent()
		return m, nil
	case k.Open, "enter":
		if m.sel == len(m.results) {
			return m, m.loadMore()
		}
		return m.openSelected()
	case k.Copy:
		if r, ok := m.selected(); ok {
			if err := open.Copy(r.URL); err != nil {
				return m, statusCmd(err.Error())
			}
			return m, statusCmd("copied " + r.URL)
		}
		return m, nil
	case k.NextPage:
		return m, m.loadMore()
	case k.PrevPage:
		if m.query != "" && m.page > 1 {
			return m, m.search(m.query, m.page-1, false)
		}
		return m, nil
	case "r":
		if m.query != "" {
			return m, m.search(m.query, m.page, false)
		}
		return m, nil
	}
	return m.navigate(key)
}

func (m Model) navigate(key string) (tea.Model, tea.Cmd) {
	k := m.cfg.Keys
	switch key {
	case k.Down, "down", "ctrl+n":
		if m.sel < len(m.results) {
			m.sel++
		}
	case k.Up, "up", "ctrl+p":
		if m.sel > 0 {
			m.sel--
		}
	case "pgdown", "ctrl+d":
		m.sel = min(m.sel+5, len(m.results))
	case "pgup", "ctrl+u":
		m.sel = max(m.sel-5, 0)
	case "g", "home":
		m.sel = 0
	case "G", "end":
		m.sel = max(0, len(m.results)-1)
	default:
		return m, nil
	}
	m.renderContent()
	m.scrollToSelection()
	return m, nil
}

func (m Model) switchBackend(step int) (tea.Model, tea.Cmd) {
	name := backend.Next(m.backend.Name(), step)
	b, err := backend.New(name, m.cfg)
	if err != nil {
		return m, statusCmd(err.Error())
	}
	m.backend = b
	m.errMsg = ""
	if ok, why := b.Ready(); !ok {
		m.errMsg = name + ": " + why
	}
	m.renderContent()
	if m.query == "" {
		return m, nil
	}
	m.input.Focus()
	return m, tea.Batch(textinput.Blink, statusCmd("switched to "+name+" — press enter to search"))
}

func (m Model) openSelected() (tea.Model, tea.Cmd) {
	r, ok := m.selected()
	if !ok {
		return m, nil
	}
	if err := open.URL(r.URL, m.cfg.General.OpenCommand); err != nil {
		return m, statusCmd(err.Error())
	}
	return m, statusCmd("opened " + r.URL)
}

func (m Model) selected() (backend.Result, bool) {
	if m.sel < 0 || m.sel >= len(m.results) {
		return backend.Result{}, false
	}
	return m.results[m.sel], true
}

func (m *Model) search(query string, page int, appendResults bool) tea.Cmd {
	m.reqID++
	m.loading = true
	m.errMsg = ""
	if page < 1 {
		page = 1
	}
	reqID, b, timeout := m.reqID, m.backend, m.cfg.Timeout()
	return tea.Batch(m.spin.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		res, err := b.Search(ctx, query, page)
		return resultsMsg{reqID: reqID, page: page, append: appendResults, results: res, err: err}
	})
}

func (m *Model) loadMore() tea.Cmd {
	if m.loading || m.query == "" || len(m.results) == 0 {
		return nil
	}
	cmd := m.search(m.query, m.page+1, true)
	m.renderContent()
	return cmd
}

// onMoreButton reports whether screen row y is the load-more row below the results.
func (m Model) onMoreButton(y int) bool {
	return m.pane == paneResults && len(m.rowStart) > len(m.results) && y-headerHeight+m.vp.YOffset == m.rowStart[len(m.results)]
}

// unseen drops results whose URL is already shown or repeated within next.
func unseen(shown, next []backend.Result) []backend.Result {
	seen := make(map[string]bool, len(shown)+len(next))
	for _, r := range shown {
		seen[strings.TrimSuffix(r.URL, "/")] = true
	}
	var out []backend.Result
	for _, r := range next {
		if key := strings.TrimSuffix(r.URL, "/"); !seen[key] {
			seen[key] = true
			out = append(out, r)
		}
	}
	return out
}

func (m Model) prefetchIcons(results []backend.Result) tea.Cmd {
	if !m.renderer.Enabled() {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(results))
	for _, r := range results {
		res := r
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			if err := m.renderer.Fetch(ctx, res.URL, res.FaviconURL); err != nil {
				return nil
			}
			return faviconMsg{url: res.URL}
		})
	}
	return tea.Batch(cmds...)
}

// suggest asks the active backend for completions when it supports them, e.g. a Degoog instance.
func (m Model) suggest(query string) tea.Cmd {
	s, ok := m.backend.(backend.Suggester)
	if !ok || len(query) < 2 {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return suggestionsMsg{query: query, items: s.Suggest(ctx, query)}
	}
}

func statusCmd(text string) tea.Cmd {
	return func() tea.Msg { return statusMsg{text: text} }
}

const headerHeight = 3

func (m *Model) layout() {
	footerHeight := 2
	h := m.height - headerHeight - footerHeight
	if h < 3 {
		h = 3
	}
	m.vp.Width = m.width
	m.vp.Height = h
}

func (m Model) View() string {
	if !m.ready {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n")
	b.WriteString(m.vp.View())
	b.WriteString("\n")
	b.WriteString(m.footer())
	return b.String()
}

func (m Model) header() string {
	tag := m.backend.Label()
	if m.loading {
		tag = m.spin.View() + " " + tag
	}
	tag = m.styles.Dim.Render("sit ▸ ") + m.styles.Tag.Render(tag)

	boxStyle := m.styles.Box
	if m.input.Focused() {
		boxStyle = m.styles.BoxActive
	}
	// The box adds two border cells and two padding cells around its content.
	inner := max(10, m.width-4)
	field := max(8, inner-lipgloss.Width(tag)-1)
	m.input.Width = field - 1
	line := padRight(truncate(m.input.View(), field), field) + " " + tag
	return boxStyle.Render(truncate(line, inner))
}

func (m Model) footer() string {
	if m.errMsg != "" {
		return m.styles.Error.Render(truncate("! "+m.errMsg+"  (r retries, tab switches backend)", m.width))
	}
	if m.status != "" {
		return m.styles.Dim.Render(truncate(m.status, m.width))
	}
	hint := fmt.Sprintf("%s search · %s backend · %s open · %s copy · %s help · %s settings · %s quit",
		m.cfg.Keys.Focus, m.cfg.Keys.NextBackend, m.cfg.Keys.Open, m.cfg.Keys.Copy,
		m.cfg.Keys.Help, m.cfg.Keys.Settings, m.cfg.Keys.Quit)
	if len(m.results) > 0 {
		hint = fmt.Sprintf("%d/%d · page %d · ", min(m.sel+1, len(m.results)), len(m.results), m.page) + hint
	}
	return m.styles.Dim.Render(truncate(hint, m.width))
}
