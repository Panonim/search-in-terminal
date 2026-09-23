package backend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

func serveBraveWeb(t *testing.T, h http.HandlerFunc) {
	t.Setenv("SIT_BRAVE_API_KEY", "")
	t.Setenv("BRAVE_API_KEY", "")
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	old := braveWebEndpoint
	braveWebEndpoint = srv.URL
	t.Cleanup(func() { braveWebEndpoint = old })
}

func TestBraveWebWithoutKey(t *testing.T) {
	var gotQuery, gotCookie string
	serveBraveWeb(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery, gotCookie = r.URL.RawQuery, r.Header.Get("Cookie")
		w.Write([]byte(`<div class="snippet svelte-x" data-pos="1" data-type="web" data-keynav="true">` +
			`<a href="https://github.com/charmbracelet/bubbletea?a=1&amp;b=2" class="svelte-y l1">` +
			`<img src="https://imgs.search.brave.com/fav" alt="" class="favicon size-m"/>` +
			`<div class="title search-snippet-title line-clamp-1" title="x">Bubble <strong>Tea</strong></div></a>` +
			`<div class="generic-snippet svelte-z"><div class="content t-primary"><!---->A TUI &amp; framework<!----></div></div></div>` +
			`<div class="snippet svelte-x" data-pos="2" data-type="web"><a href="https://example.com/">` +
			`<div class="title search-snippet-title">No snippet</div></a></div>`))
	})

	b := newBrave(testConfig())
	if ok, _ := b.Ready(); !ok {
		t.Fatal("brave should be ready without a key")
	}
	results, err := b.Search(context.Background(), "bubbletea", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results", len(results))
	}
	r := results[0]
	if r.Title != "Bubble Tea" || r.URL != "https://github.com/charmbracelet/bubbletea?a=1&b=2" ||
		r.Snippet != "A TUI & framework" || r.FaviconURL != "https://imgs.search.brave.com/fav" {
		t.Errorf("unexpected result %+v", r)
	}
	if results[1].Snippet != "" {
		t.Errorf("second snippet = %q", results[1].Snippet)
	}
	if gotQuery != "offset=1&q=bubbletea&source=web" || gotCookie != "safesearch=moderate" {
		t.Errorf("query = %q, cookie = %q", gotQuery, gotCookie)
	}
}

func ddgLite(links []string, forms string) string {
	var b strings.Builder
	for _, l := range links {
		b.WriteString(`<a rel="nofollow" href="https://` + l + `.example/" class='result-link'>` + l + `</a>` +
			`<td class='result-snippet'>about ` + l + `</td>`)
	}
	return b.String() + forms
}

const ddgPrevForm = `<form class="prev_form" action="/lite/" method="post">
    <input type="submit" class='navbutton' value="&lt; Previous Page">
    <input type="hidden" name="s" value="0">
    <input type="hidden" name="vqd" value="4-tok">
</form>`

func ddgNextForm(s string) string {
	return `<form class="next_form" action="/lite/" method="post">
    <input type="submit" class='navbutton' value="Next Page &gt;">
    <input type="hidden" name="q" value="go">
    <input type="hidden" name="s" value="` + s + `">
    <input type="hidden" name="vqd" value="4-tok">
</form>`
}

// serveDDG fakes three DDG pages where later pages demand the vqd token, and counts requests.
func serveDDG(t *testing.T) *int {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		r.ParseForm()
		token := r.Form.Get("vqd") == "4-tok" && r.Form.Get("kl") == "wt-wt"
		switch s := r.Form.Get("s"); {
		case s == "":
			w.Write([]byte(ddgLite([]string{"a", "b"}, ddgNextForm("2"))))
		case s == "2" && token:
			w.Write([]byte(ddgLite([]string{"c"}, ddgPrevForm+ddgNextForm("5"))))
		case s == "5" && token:
			w.Write([]byte(ddgLite([]string{"d"}, ddgPrevForm)))
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	t.Cleanup(srv.Close)
	oldLite, oldHTML := ddgLiteEndpoint, ddgHTMLEndpoint
	ddgLiteEndpoint, ddgHTMLEndpoint = srv.URL+"/lite/", srv.URL+"/html/"
	t.Cleanup(func() { ddgLiteEndpoint, ddgHTMLEndpoint = oldLite, oldHTML })
	return &hits
}

func TestDDGLoadMoreResumesFromCachedPages(t *testing.T) {
	t.Setenv("SIT_CACHE_DIR", t.TempDir())
	hits := serveDDG(t)
	cfg := testConfig()
	cfg.General.CacheTTLSeconds = 300
	ctx := context.Background()

	first := withCache(newDDG(cfg), cfg)
	for page := 1; page <= 2; page++ {
		if _, err := first.Search(ctx, "go", page); err != nil {
			t.Fatal(err)
		}
	}

	// A new session gets pages 1 and 2 from disk, so page 3 needs only its own request.
	b := withCache(newDDG(cfg), cfg)
	for page := 1; page <= 2; page++ {
		if res, _ := b.Search(ctx, "go", page); len(res) == 0 || !res[0].Cached {
			t.Fatalf("page %d should come from cache: %+v", page, res)
		}
	}
	res, err := b.Search(ctx, "go", 3)
	if err != nil || len(res) != 1 || res[0].URL != "https://d.example/" || res[0].Cached {
		t.Fatalf("page 3 = %+v, %v", res, err)
	}
	if *hits != 3 {
		t.Errorf("cached pages must not be refetched for their token, got %d hits", *hits)
	}
	if res, err := b.Search(ctx, "go", 4); err != nil || len(res) != 0 || *hits != 3 {
		t.Errorf("page 4 = %+v, %v after %d hits; want end of results without a request", res, err, *hits)
	}
}

func TestDDGWalksForTokenWithoutCache(t *testing.T) {
	hits := serveDDG(t)
	res, err := newDDG(testConfig()).Search(context.Background(), "go", 3)
	if err != nil || len(res) != 1 || res[0].URL != "https://d.example/" || *hits != 3 {
		t.Errorf("page 3 = %+v, %v after %d hits", res, err, *hits)
	}
}

func TestBraveWebRateLimited(t *testing.T) {
	serveBraveWeb(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})

	if _, err := newBrave(testConfig()).Search(context.Background(), "x", 1); err == nil {
		t.Error("expected rate-limit error")
	}
}
