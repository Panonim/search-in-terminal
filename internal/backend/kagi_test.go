package backend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveKagi(t *testing.T, h http.HandlerFunc) {
	for _, env := range []string{"SIT_KAGI_API_KEY", "KAGI_API_KEY", "SIT_KAGI_SESSION", "KAGI_SESSION_TOKEN"} {
		t.Setenv(env, "")
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	oldAPI, oldWeb := kagiAPIEndpoint, kagiWebEndpoint
	kagiAPIEndpoint, kagiWebEndpoint = srv.URL+"/api/v1/search", srv.URL+"/html/search"
	t.Cleanup(func() { kagiAPIEndpoint, kagiWebEndpoint = oldAPI, oldWeb })
}

func TestKagiAPI(t *testing.T) {
	var auth string
	var body map[string]any
	serveKagi(t, func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&body)
		w.Write([]byte(`{"meta":{},"data":{"search":[{"url":"https://go.dev/","title":"The Go <b>Programming</b> Language","snippet":"Build &amp; ship"}],"related_search":[{"url":"x","title":"y"}]}}`))
	})
	cfg := testConfig()
	cfg.Backends.Kagi.APIKey = "k"
	b := newKagi(cfg)
	if b.Label() != "kagi" {
		t.Errorf("label = %q", b.Label())
	}
	res, err := b.Search(context.Background(), "golang", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Title != "The Go Programming Language" || res[0].Snippet != "Build & ship" || res[0].URL != "https://go.dev/" {
		t.Errorf("results = %+v", res)
	}
	if auth != "Bearer k" || body["query"] != "golang" || body["page"] != 2.0 || body["limit"] != 10.0 || body["safe_search"] != true {
		t.Errorf("auth = %q, body = %v", auth, body)
	}
}

func TestKagiAPIError(t *testing.T) {
	serveKagi(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`{"meta":{},"data":null,"errors":[{"code":"billing.insufficient","message":"Insufficient credit"}]}`))
	})
	cfg := testConfig()
	cfg.Backends.Kagi.APIKey = "k"
	if _, err := newKagi(cfg).Search(context.Background(), "x", 1); err == nil || !strings.Contains(err.Error(), "Insufficient credit") {
		t.Errorf("err = %v", err)
	}
}

// kagiPage mirrors the markup of kagi.com/html/search: plain results, a grouped result and page chrome.
const kagiPage = `<html><body><div class="_0_main-search-results">
<div class="_0_SRI search-result">
  <div class="_0_TITLE __sri-title"><h3 class="__sri-title-box">
    <a class="__sri_title_link _ext_t" href="https://example1.com/page">Result 1</a></h3></div>
  <div class="__sri-url-box"><a href="https://example1.com/page"><span class="host">example1.com</span></a></div>
  <div class="__sri-body"><div class="__sri-desc">Snippet one about dogs and shows.</div></div>
</div>
<div class="_0_SRI search-result">
  <div class="_0_TITLE __sri-title"><h3 class="__sri-title-box">
    <a href="https://akc.org/breeds" class="__sri_title_link _ext_t">Result 2</a></h3></div>
  <div class="__sri-body"><div class="__sri-desc"><span class="__sri-time">Nov 14, 2025</span>
    Snippet two from the kennel club. <a class="summarize-link">Summarize</a></div></div>
  <div class="sr-group"><div class="__srgi">
    <div class="__srgi-title"><a href="https://akc.org/breeds/poodle">Poodle</a></div>
    <div class="__sri-desc">All about the poodle breed.</div></div></div>
</div>
</div>
<div class="related-searches"><a href="/html/search?q=dogs"><span>dogs</span></a></div>
<a id="load_more_results" href="/html/search?q=dog&batch=2">More Results</a>
</body></html>`

func TestKagiWebWithSession(t *testing.T) {
	var cookie, query string
	serveKagi(t, func(w http.ResponseWriter, r *http.Request) {
		cookie, query = r.Header.Get("Cookie"), r.URL.RawQuery
		w.Write([]byte(kagiPage))
	})
	cfg := testConfig()
	cfg.Backends.Kagi.SessionToken = "https://kagi.com/search?token=abc.def&q=%s"
	b := newKagi(cfg)
	if b.Label() != "kagi (web)" {
		t.Errorf("label = %q", b.Label())
	}
	res, err := b.Search(context.Background(), "dog", 2)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != "kagi_session=abc.def" || query != "batch=2&q=dog" {
		t.Errorf("cookie = %q, query = %q", cookie, query)
	}
	want := []Result{
		{Title: "Result 1", URL: "https://example1.com/page", Snippet: "Snippet one about dogs and shows."},
		{Title: "Result 2", URL: "https://akc.org/breeds", Snippet: "Nov 14, 2025 Snippet two from the kennel club."},
		{Title: "Poodle", URL: "https://akc.org/breeds/poodle", Snippet: "All about the poodle breed."},
	}
	if len(res) != len(want) {
		t.Fatalf("results = %+v", res)
	}
	for i, w := range want {
		if r := res[i]; r.Title != w.Title || r.URL != w.URL || r.Snippet != w.Snippet || r.Source != "kagi" {
			t.Errorf("result %d = %+v, want %+v", i, r, w)
		}
	}
}

func TestKagiWebExpiredSession(t *testing.T) {
	serveKagi(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/html/search" {
			http.Redirect(w, r, "/welcome", http.StatusFound)
			return
		}
		w.Write([]byte("<html>sign up</html>"))
	})
	cfg := testConfig()
	cfg.Backends.Kagi.SessionToken = "stale"
	if _, err := newKagi(cfg).Search(context.Background(), "x", 1); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Errorf("err = %v", err)
	}
}

func TestKagiNeedsCredentials(t *testing.T) {
	serveKagi(t, func(w http.ResponseWriter, r *http.Request) {})
	if ok, _ := newKagi(testConfig()).Ready(); ok {
		t.Error("kagi must not be ready without a key or session")
	}
}
