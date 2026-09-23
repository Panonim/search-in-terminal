package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/Panonim/search-in-terminal/internal/backend"
	"github.com/Panonim/search-in-terminal/internal/config"
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
		results[i] = backend.Result{Title: "result", URL: "https://example.com", Snippet: "snippet"}
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
	if !strings.Contains(m.vp.View(), "general.backend") {
		t.Error("settings pane missing its fields")
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

func TestViewFitsTerminalWidth(t *testing.T) {
	m := withResults(t, newTestModel(t), 3)
	for _, line := range strings.Split(m.View(), "\n") {
		if w := ansi.StringWidth(line); w > 100 {
			t.Errorf("line wider than the terminal (%d): %q", w, line)
		}
	}
}
