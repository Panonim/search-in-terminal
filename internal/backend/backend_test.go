package backend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Panonim/search-in-terminal/internal/config"
)

func testConfig() config.Config {
	cfg := config.Default()
	cfg.General.ResultsPerPage = 10
	return cfg
}

func TestDegoogSearch(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path+"?"+r.URL.RawQuery, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[{"title":"Rust <b>lifetimes</b>","url":"https://doc.rust-lang.org/","snippet":"a &amp; b","source":"Brave","sources":["Brave","DuckDuckGo"]}]}`))
	}))
	defer srv.Close()

	cfg := testConfig()
	cfg.Backends.Degoog.Instance = srv.URL
	cfg.Backends.Degoog.APIKey = "secret"
	b := newDegoog(cfg)

	results, err := b.Search(context.Background(), "rust lifetimes", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Title != "Rust lifetimes" || results[0].Snippet != "a & b" {
		t.Errorf("markup not cleaned: %+v", results[0])
	}
	if results[0].Source != "Brave+DuckDuckGo" {
		t.Errorf("want merged sources, got %q", results[0].Source)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("want bearer auth, got %q", gotAuth)
	}
	if want := "/api/search?page=2&q=rust+lifetimes&safeMode=moderate&type=web"; gotPath != want {
		t.Errorf("request = %q, want %q", gotPath, want)
	}
}

func TestDegoogProtectedInstance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"You shall not pass!"}`))
	}))
	defer srv.Close()

	cfg := testConfig()
	cfg.Backends.Degoog.Instance = srv.URL
	if _, err := newDegoog(cfg).Search(context.Background(), "hi", 1); err == nil {
		t.Fatal("want an error for a protected instance")
	}
}

func TestDegoogPinnedEnginesUsePost(t *testing.T) {
	var method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	cfg := testConfig()
	cfg.Backends.Degoog.Instance = srv.URL
	cfg.Backends.Degoog.Engines = []string{"brave-engine"}
	if _, err := newDegoog(cfg).Search(context.Background(), "hi", 1); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost {
		t.Errorf("want POST when engines are pinned, got %s", method)
	}
}

func TestDegoogSuggest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"text":"rust lifetimes","source":"Google"},{"text":""}]`))
	}))
	defer srv.Close()

	cfg := testConfig()
	cfg.Backends.Degoog.Instance = srv.URL
	got := newDegoog(cfg).Suggest(context.Background(), "rust")
	if len(got) != 1 || got[0] != "rust lifetimes" {
		t.Errorf("suggestions = %v", got)
	}
}

func TestSearXNGFallsBackToNextInstance(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>bot check</html>"))
	}))
	defer broken.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[{"title":"ok","url":"https://example.com","content":"snippet","engine":"google"}]}`))
	}))
	defer good.Close()

	cfg := testConfig()
	cfg.Backends.SearXNG.Instance = broken.URL
	cfg.Backends.SearXNG.Fallbacks = []string{good.URL}
	results, err := newSearXNG(cfg).Search(context.Background(), "q", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].URL != "https://example.com" {
		t.Errorf("results = %+v", results)
	}
}

func TestParseDDGLite(t *testing.T) {
	body := `<tr><td><a rel="nofollow" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fgo.dev%2F&amp;rut=x" class='result-link'>The Go <b>Programming</b> Language</a></td></tr>
	<tr><td class='result-snippet'>Build simple, secure &amp; scalable systems.</td></tr>`
	results := parseDDG(body, liteLinkRE, liteSnippetRE)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].URL != "https://go.dev/" {
		t.Errorf("redirect not unwrapped: %q", results[0].URL)
	}
	if results[0].Title != "The Go Programming Language" {
		t.Errorf("title = %q", results[0].Title)
	}
	if results[0].Snippet != "Build simple, secure & scalable systems." {
		t.Errorf("snippet = %q", results[0].Snippet)
	}
}

func TestNextCyclesBackends(t *testing.T) {
	if got := Next("ddg", 1); got != "degoog" {
		t.Errorf("next after ddg = %q", got)
	}
	if got := Next("ddg", -1); got != "brave" {
		t.Errorf("previous of ddg = %q", got)
	}
}

func TestBraveNeedsKey(t *testing.T) {
	t.Setenv("SIT_BRAVE_API_KEY", "")
	t.Setenv("BRAVE_API_KEY", "")
	if ok, _ := newBrave(testConfig()).Ready(); ok {
		t.Error("brave should not be ready without a key")
	}
}
