package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/Panonim/search-in-terminal/internal/backend"
	"github.com/Panonim/search-in-terminal/internal/config"
	img "github.com/Panonim/search-in-terminal/internal/image"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("SIT_CONFIG_DIR", t.TempDir())
	t.Setenv("SIT_CACHE_DIR", t.TempDir())
	cfg := config.Default()
	cfg.Theme.Icons = false
	m, err := New(cfg, "test", "")
	if err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(Model)
}

func withResults(t *testing.T, m Model, n int) Model {
	t.Helper()
	results := make([]backend.Result, n)
	for i := range results {
		results[i] = backend.Result{Title: "result", URL: fmt.Sprintf("https://example.com/%d", i), Snippet: "snippet"}
	}
	m.query = "go"
	next, _ := m.Update(resultsMsg{reqID: m.reqID, page: 1, results: results})
	return next.(Model)
}

func press(m Model, key string) Model {
	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestInitialQueryResultsAreKept(t *testing.T) {
	t.Setenv("SIT_CONFIG_DIR", t.TempDir())
	t.Setenv("SIT_CACHE_DIR", t.TempDir())
	cfg := config.Default()
	cfg.Theme.Icons = false
	m, err := New(cfg, "test", "go")
	if err != nil {
		t.Fatal(err)
	}
	if !m.loading {
		t.Error("initial query should start loading")
	}
	// The initial search is the model's first request, so its reply carries reqID 1.
	next, _ := m.Update(resultsMsg{reqID: 1, page: 1, results: []backend.Result{{Title: "r", URL: "https://example.com"}}})
	if got := len(next.(Model).results); got != 1 {
		t.Errorf("initial query results dropped, got %d", got)
	}
}

func TestResultsBlurInputAndNavigate(t *testing.T) {
	m := withResults(t, newTestModel(t), 5)
	if m.input.Focused() {
		t.Fatal("input should blur once results arrive so j/k navigate")
	}
	m = press(m, "j")
	m = press(m, "j")
	if m.sel != 2 {
		t.Errorf("selection = %d, want 2", m.sel)
	}
	m = press(m, "k")
	if m.sel != 1 {
		t.Errorf("selection after k = %d, want 1", m.sel)
	}
	m = press(m, "G")
	if m.sel != 4 {
		t.Errorf("selection after G = %d, want 4", m.sel)
	}
}

func TestTypingStaysInInput(t *testing.T) {
	m := newTestModel(t)
	for _, r := range "golang" {
		m = press(m, string(r))
	}
	if got := m.input.Value(); got != "golang" {
		t.Errorf("input = %q", got)
	}
	if m.sel != 0 {
		t.Errorf("typing must not move the selection, sel = %d", m.sel)
	}
}

func TestHelpAndSettingsPanes(t *testing.T) {
	m := withResults(t, newTestModel(t), 3)
	m = press(m, "?")
	if m.pane != paneHelp {
		t.Fatal("? should open help")
	}
	if !strings.Contains(m.vp.View(), "search the web without leaving your terminal") {
		t.Error("help pane missing its description")
	}
	m = press(m, "esc")
	m = press(m, ",")
	if m.pane != paneSettings {
		t.Fatal(", should open settings")
	}
	if !strings.Contains(m.vp.View(), "results_per_page") {
		t.Error("settings pane missing its fields")
	}
	m = press(m, "tab")
	if f := m.settings.fields[m.settings.idx]; f.Key != "theme.accent" {
		t.Errorf("tab should jump to the next section, got %s", f.Key)
	}
	m = press(m, "3")
	if f := m.settings.fields[m.settings.idx]; f.Key != "backends.degoog.instance" {
		t.Errorf("3 should jump to the third section, got %s", f.Key)
	}
	for _, line := range strings.Split(m.settings.view(50, 20), "\n") {
		if w := ansi.StringWidth(line); w > 50 {
			t.Errorf("settings line wider than the pane (%d): %q", w, line)
		}
	}
}

func TestBackendCycleKeepsQuery(t *testing.T) {
	m := withResults(t, newTestModel(t), 2)
	before := m.backend.Name()
	m = press(m, "tab")
	if m.backend.Name() == before {
		t.Fatal("tab should switch backend")
	}
	if m.query != "go" {
		t.Errorf("query lost on backend switch: %q", m.query)
	}
	if m.loading {
		t.Error("switching backend should not re-run the query automatically")
	}
	if !m.input.Focused() {
		t.Error("switching backend should focus the search input")
	}
}

func appendPage(m Model, page int, urls ...string) (Model, tea.Cmd) {
	results := make([]backend.Result, len(urls))
	for i, u := range urls {
		results[i] = backend.Result{Title: "more", URL: u}
	}
	next, cmd := m.Update(resultsMsg{reqID: m.reqID, page: page, append: true, results: results})
	return next.(Model), cmd
}

func TestLoadMoreButtonAddsOnlyNewResults(t *testing.T) {
	m := withResults(t, newTestModel(t), 2)
	for range 3 {
		m = press(m, "j")
	}
	if m.sel != 2 || m.loading {
		t.Fatalf("j should stop on the button without loading, sel = %d, loading = %v", m.sel, m.loading)
	}
	var button string
	for _, line := range strings.Split(ansi.Strip(m.vp.View()), "\n") {
		if strings.Contains(line, "[ load more results ]") {
			button = line
		}
	}
	if pad := len(button) - len(strings.TrimLeft(button, " ")); pad != (100-21)/2 {
		t.Errorf("button not centered, %d leading spaces: %q", pad, button)
	}
	if !strings.Contains(m.footer(), "2/2") {
		t.Errorf("footer should not count the button as a result: %q", m.footer())
	}

	m = press(m, "enter")
	if view := m.vp.View(); !m.loading || !strings.Contains(view, "loading…") || strings.Contains(view, "load more results") {
		t.Fatal("enter on the button should load more and show loading on the button")
	}
	m, _ = appendPage(m, 2, "https://example.com/0/", "https://example.com/new", "https://example.com/new")
	if len(m.results) != 3 || m.results[2].URL != "https://example.com/new" || m.sel != 2 || m.page != 2 {
		t.Fatalf("want one new result selected on page 2, got %d results, sel %d, page %d", len(m.results), m.sel, m.page)
	}

	m.loadMore()
	m, _ = appendPage(m, 3, "https://example.com/new")
	if len(m.results) != 3 || m.page != 3 {
		t.Errorf("a page of repeats should add nothing but still advance, got %d results, page %d", len(m.results), m.page)
	}
	m.loadMore()
	m, _ = appendPage(m, 4)
	if m.page != 3 {
		t.Errorf("an empty page should not advance, page = %d", m.page)
	}
}

func TestClickLoadMoreButton(t *testing.T) {
	m := withResults(t, newTestModel(t), 2)
	click := func(row int) Model {
		next, _ := m.Update(tea.MouseMsg{X: 50, Y: headerHeight + row - m.vp.YOffset, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
		return next.(Model)
	}
	if click(m.rowStart[1]).loading {
		t.Error("clicking a result must not load more")
	}
	if clicked := click(m.rowStart[2]); !clicked.loading || !strings.Contains(clicked.vp.View(), "loading…") {
		t.Error("clicking the button should load more and show loading")
	}
}

func TestLoadMoreFetchesIconsForNewResultsOnly(t *testing.T) {
	t.Setenv("TMUX", "")
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		png.Encode(w, image.NewNRGBA(image.Rect(0, 0, 16, 16)))
	}))
	defer srv.Close()

	m := withResults(t, newTestModel(t), 1)
	m.renderer = img.NewRenderer(img.ProtocolKitty, t.TempDir(), iconWidth, true, false, color.NRGBA{})
	next, cmd := m.Update(resultsMsg{reqID: m.reqID, page: 2, append: true, results: []backend.Result{
		{Title: "dup", URL: "https://example.com/0", FaviconURL: srv.URL + "/dup.png"},
		{Title: "new", URL: "https://fresh.test/", FaviconURL: srv.URL + "/fresh.png", Cached: true},
	}})
	if cmd == nil {
		t.Fatal("new results should prefetch icons")
	}
	next, _ = next.Update(cmd())
	if len(hits) != 1 || hits[0] != "/fresh.png" {
		t.Errorf("want only the new result's favicon fetched, got %v", hits)
	}
	view := next.(Model).vp.View()
	if !strings.Contains(view, "\x1b_G") || !strings.Contains(view, "ᶻ") {
		t.Error("appended cached result should render its favicon and cache mark")
	}
}

func TestPanesIgnoreResultsScroll(t *testing.T) {
	next, _ := withResults(t, newTestModel(t), 20).Update(tea.WindowSizeMsg{Width: 240, Height: 60})
	m := press(next.(Model), "G")
	m = press(m, "n")
	m = press(m, ",")
	if m.vp.YOffset != 0 || !strings.Contains(m.vp.View(), "settings") {
		t.Fatalf("settings should open at the top, offset = %d", m.vp.YOffset)
	}
	m, _ = appendPage(m, 2, "https://example.com/late")
	if m.vp.YOffset != 0 {
		t.Errorf("results arriving behind settings must not scroll it, offset = %d", m.vp.YOffset)
	}
	m = press(m, "esc")
	if start := m.rowStart[m.sel]; start < m.vp.YOffset || start >= m.vp.YOffset+m.vp.Height {
		t.Errorf("closing settings should bring the selected result back into view, row %d, offset %d", start, m.vp.YOffset)
	}
	m = press(m, "?")
	if m.vp.YOffset != 0 {
		t.Errorf("help should open at the top, offset = %d", m.vp.YOffset)
	}
}

func TestViewFitsTerminalWidth(t *testing.T) {
	m := withResults(t, newTestModel(t), 3)
	for _, line := range strings.Split(m.View(), "\n") {
		if w := ansi.StringWidth(line); w > 100 {
			t.Errorf("line wider than the terminal (%d): %q", w, line)
		}
	}
}
