package backend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseQuery(t *testing.T) {
	q := parseQuery(`rust +fast  site:GitHub.com -site:gist.github.com -Unsafe -filetype:pdf -"old  api" "borrow checker" intitle:"The Book" inurl:docs ext:HTML After:2024-01-01 before:2024-06-30 after:yesterday - ""`)
	want := query{
		text:   `rust +fast site:GitHub.com -site:gist.github.com -Unsafe -filetype:pdf -"old  api" "borrow checker" intitle:"The Book" inurl:docs ext:HTML after:yesterday - ""`,
		after:  "2024-01-01",
		before: "2024-06-30",
		rules: []rule{
			{op: "site", value: "github.com"},
			{op: "site", value: "gist.github.com", not: true},
			{value: "unsafe", not: true},
			{op: "filetype", value: "pdf", not: true},
			{value: "old  api", not: true},
			{value: "borrow checker"},
			{op: "intitle", value: "the book"},
			{op: "inurl", value: "docs"},
			{op: "filetype", value: "html"},
		},
	}
	if !reflect.DeepEqual(q, want) {
		t.Errorf("parseQuery =\n%+v\nwant\n%+v", q, want)
	}
	if got := q.span("to"); got != "2024-01-01to2024-06-30" {
		t.Errorf("span = %q", got)
	}
	today := time.Now().Format(dateLayout)
	if got := parseQuery("x after:2024-01-01").span(".."); got != "2024-01-01.."+today {
		t.Errorf("open-ended span = %q", got)
	}
	if got := parseQuery("x before:2024-01-01").span(".."); got != "1970-01-01..2024-01-01" {
		t.Errorf("open-start span = %q", got)
	}
	if got := parseQuery("x").span(".."); got != "" {
		t.Errorf("no dates, span = %q", got)
	}
}

func TestRecent(t *testing.T) {
	daysAgo := func(n int) string { return time.Now().AddDate(0, 0, -n).Format(dateLayout) }
	for after, want := range map[string]string{
		daysAgo(0):   "day",
		daysAgo(3):   "week",
		daysAgo(20):  "month",
		daysAgo(200): "year",
		daysAgo(900): "",
	} {
		if got := (query{after: after}).recent(); got != want {
			t.Errorf("recent(%s) = %q, want %q", after, got, want)
		}
	}
}

func TestKeep(t *testing.T) {
	q := parseQuery("go site:github.com/golang -site:gist.github.com -go1")
	for u, want := range map[string]bool{
		"https://github.com/golang/go":        true,
		"https://api.github.com/golang/x":     true,
		"https://github.com/GoLang/tools":     true,
		"https://github.com/rust-lang/rust":   false,
		"https://notgithub.com/golang/go":     false,
		"https://gist.github.com/golang/1234": false,
	} {
		if got := q.keep(Result{URL: u}); got != want {
			t.Errorf("keep(%s) = %v, want %v", u, got, want)
		}
	}
	q = parseQuery("go -java")
	if !q.keep(Result{URL: "https://a.example", Title: "JavaScript and Go"}) {
		t.Error("-java must not drop a result that only mentions javascript")
	}
	if q.keep(Result{URL: "https://a.example", Snippet: "Go vs. Java, compared"}) {
		t.Error("-java must drop a result mentioning java")
	}
}

func TestKeepSyntax(t *testing.T) {
	pdf := Result{Title: "Unit Testing Guide", URL: "https://docs.example.com/guides/testing.PDF", Snippet: "How to write a test for every function."}
	page := Result{Title: "Test driven development", URL: "https://blog.example.org/tdd", Snippet: "Red, green, refactor."}
	other := Result{Title: "Testing in production", URL: "https://example.net/prod", Snippet: "Feature flags and canaries."}
	for query, want := range map[string][]bool{
		`"test"`:                               {true, true, false},
		`"write a test"`:                       {true, false, false},
		`"a test for"`:                         {true, false, false},
		`-"red green"`:                         {true, false, true},
		`intitle:test`:                         {false, true, false},
		`intitle:"unit testing"`:               {true, false, false},
		`-intitle:testing`:                     {false, true, false},
		`inurl:guides`:                         {true, false, false},
		`filetype:pdf`:                         {true, false, false},
		`ext:pdf`:                              {true, false, false},
		`-filetype:pdf`:                        {false, true, true},
		`"canaries" OR "refactor"`:             {false, true, true},
		`site:example.org OR site:example.net`: {false, true, true},
		`testing`:                              {true, true, true},
		`+testing`:                             {true, true, true},
	} {
		q := parseQuery(query)
		for i, r := range []Result{pdf, page, other} {
			if got := q.keep(r); got != want[i] {
				t.Errorf("%s: keep(%s) = %v, want %v", query, r.URL, got, want[i])
			}
		}
	}
}

func TestOperatorsReachBraveAndFilterResults(t *testing.T) {
	var got url.Values
	serveBraveWeb(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Write([]byte(`<div data-type="web"><a href="https://github.com/a/tui"><div class="title">TUI one</div></a><p>A small terminal UI library.</p></div>` +
			`<div data-type="web"><a href="https://example.com/tui"><div class="title">Off-site TUI</div></a><p>A page the engine let through.</p></div>` +
			`<div data-type="web"><a href="https://github.com/b/tui"><div class="title">Electron TUI</div></a><p>Built on electron, sadly.</p></div>`))
	})
	cfg := testConfig()
	cfg.General.Cache = false
	b, err := New("brave", cfg)
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Search(context.Background(), "tui site:github.com -electron after:2024-01-01 before:2024-12-31", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Get("q") != "tui site:github.com -electron" || got.Get("tf") != "2024-01-01to2024-12-31" {
		t.Errorf("request = %v", got)
	}
	if len(res) != 1 || res[0].URL != "https://github.com/a/tui" {
		t.Errorf("results = %+v", res)
	}
}

func TestDatesReachEachEngine(t *testing.T) {
	var got url.Values
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		got = r.Form
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[],"data":{"search":[]}}`))
	}))
	defer srv.Close()
	ctx := context.Background()
	week := time.Now().AddDate(0, 0, -3).Format(dateLayout)

	cfg := testConfig()
	cfg.Backends.SearXNG.Instance, cfg.Backends.SearXNG.Fallbacks = srv.URL, nil
	newSearXNG(cfg).Search(ctx, "go after:"+week, 1)
	if got.Get("q") != "go" || got.Get("time_range") != "week" {
		t.Errorf("searxng got %v", got)
	}

	cfg.Backends.Degoog.Instance = srv.URL
	newDegoog(cfg).Search(ctx, "go before:2024-02-01", 1)
	if got.Get("q") != "go" || got.Get("time") != "custom" || got.Get("dateTo") != "2024-02-01" || got.Has("dateFrom") {
		t.Errorf("degoog got %v", got)
	}

	cfg.Backends.Degoog.Engines = []string{"e"}
	newDegoog(cfg).Search(ctx, "go after:2024-01-01", 1)
	if body["query"] != "go" || body["time"] != "custom" || body["dateFrom"] != "2024-01-01" {
		t.Errorf("degoog POST got %v", body)
	}

	serveKagi(t, srv.Config.Handler.ServeHTTP)
	cfg.Backends.Kagi.APIKey = "k"
	newKagi(cfg).Search(ctx, "go after:2024-01-01", 1)
	if body["query"] != "go" || !reflect.DeepEqual(body["filters"], map[string]any{"after": "2024-01-01"}) {
		t.Errorf("kagi API got %v", body)
	}

	if got := newDDG(cfg).base("go site:go.dev before:2024-01-01"); got.Get("q") != "go site:go.dev" || got.Get("df") != "1970-01-01..2024-01-01" {
		t.Errorf("ddg form = %v", got)
	}
}

func TestKagiWebDates(t *testing.T) {
	var got url.Values
	serveKagi(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Write([]byte(kagiPage))
	})
	cfg := testConfig()
	cfg.Backends.Kagi.SessionToken = "t"
	newKagi(cfg).Search(context.Background(), "dog after:2024-01-01 before:2024-02-01", 1)
	if got.Get("q") != "dog" || got.Get("from_date") != "2024-01-01" || got.Get("to_date") != "2024-02-01" {
		t.Errorf("kagi web got %v", got)
	}
}

func TestCacheKeepsExclusionsApart(t *testing.T) {
	t.Setenv("SIT_CACHE_DIR", t.TempDir())
	cfg := testConfig()
	cfg.General.CacheTTLSeconds = 300
	inner := &countingBackend{}
	b := withCache(inner, cfg)
	for _, q := range []string{"go rust", "go -rust", "go -rsut", `go "rust"`, `"go rust"`} {
		if res, _ := b.Search(context.Background(), q, 1); len(res) != 1 || (res[0].Cached && q != "go -rsut") {
			t.Fatalf("%q = %+v", q, res)
		}
	}
	if inner.calls != 4 || !strings.HasPrefix(normalizeQuery("-Rust go"), "-rust") {
		t.Errorf("want 4 engine hits with a typo sharing the -rust entry, got %d", inner.calls)
	}
}
