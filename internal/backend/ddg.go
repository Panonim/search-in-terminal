package backend

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/Panonim/search-in-terminal/internal/config"
)

// The DuckDuckGo backend scrapes the Lite/HTML endpoints; unofficial and best-effort, it can break without notice.
type ddg struct {
	cfg config.Config

	mu sync.Mutex
	// next holds DDG's own encoded next-page form keyed by page and query; "" marks the last page.
	next map[string]string
}

func newDDG(cfg config.Config) *ddg { return &ddg{cfg: cfg, next: map[string]string{}} }

func (d *ddg) Name() string          { return "ddg" }
func (d *ddg) Label() string         { return "duckduckgo" }
func (d *ddg) Ready() (bool, string) { return true, "" }

// The endpoints are vars so tests can point them at a local server.
var (
	ddgLiteEndpoint = "https://lite.duckduckgo.com/lite/"
	ddgHTMLEndpoint = "https://html.duckduckgo.com/html/"
)

var (
	ddgNextRE     = regexp.MustCompile(`(?is)<form[^>]*>\s*<input type="submit"[^>]*value="Next[^"]*"[^>]*>(.*?)</form>`)
	ddgHiddenRE   = regexp.MustCompile(`<input type="hidden" name="([^"]+)" value="([^"]*)"`)
	liteLinkRE    = regexp.MustCompile(`(?is)<a\s+[^>]*href=["']([^"']+)["'][^>]*class=['"]result-link['"][^>]*>(.*?)</a>`)
	liteSnippetRE = regexp.MustCompile(`(?is)<td[^>]*class=['"]result-snippet['"][^>]*>(.*?)</td>`)
	htmlLinkRE    = regexp.MustCompile(`(?is)<a\s+[^>]*class=["'][^"']*result__a[^"']*["'][^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	htmlSnippetRE = regexp.MustCompile(`(?is)class=["'][^"']*result__snippet[^"']*["'][^>]*>(.*?)</a>`)
)

func (d *ddg) Search(ctx context.Context, query string, page int) ([]Result, error) {
	form, err := d.form(ctx, query, page)
	if err != nil || form == nil {
		return nil, err
	}

	body, err := d.post(ctx, ddgLiteEndpoint, form)
	if err != nil {
		return nil, err
	}
	results := parseDDG(body, liteLinkRE, liteSnippetRE)
	if len(results) == 0 {
		if body, err = d.post(ctx, ddgHTMLEndpoint, form); err != nil {
			return nil, err
		}
		results = parseDDG(body, htmlLinkRE, htmlSnippetRE)
	}
	if len(results) == 0 && strings.Contains(body, "anomaly") {
		return nil, errors.New("duckduckgo: blocked this request, try again later or switch backend")
	}
	d.remember(query, page+1, body)
	return results, nil
}

// form returns the POST body for page; DDG rejects later pages without the vqd token and offsets from the page before.
func (d *ddg) form(ctx context.Context, query string, page int) (url.Values, error) {
	if page <= 1 {
		return d.base(query), nil
	}
	d.mu.Lock()
	cursor, ok := d.next[ddgKey(query, page)]
	d.mu.Unlock()
	if !ok {
		if _, err := d.Search(ctx, query, page-1); err != nil {
			return nil, err
		}
		cursor = d.Cursor(query, page)
	}
	if cursor == "" {
		return nil, nil
	}
	return url.ParseQuery(cursor)
}

func (d *ddg) remember(query string, page int, body string) {
	cursor := ""
	if m := ddgNextRE.FindStringSubmatch(body); m != nil {
		form := d.base(query)
		for _, in := range ddgHiddenRE.FindAllStringSubmatch(m[1], -1) {
			form.Set(in[1], html.UnescapeString(in[2]))
		}
		cursor = form.Encode()
	}
	d.SetCursor(query, page, cursor)
}

func (d *ddg) Cursor(query string, page int) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.next[ddgKey(query, page)]
}

func (d *ddg) SetCursor(query string, page int, cursor string) {
	d.mu.Lock()
	d.next[ddgKey(query, page)] = cursor
	d.mu.Unlock()
}

func ddgKey(query string, page int) string { return fmt.Sprint(page, "\x00", query) }

func (d *ddg) base(query string) url.Values {
	return url.Values{"q": {query}, "kl": {ddgRegion(d.cfg.General.Region)}, "kp": {ddgSafe(d.cfg.General.SafeSearch)}}
}

func (d *ddg) post(ctx context.Context, endpoint string, form url.Values) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", endpoint)
	resp, err := httpClient(d.cfg).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", statusError("duckduckgo", resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(raw), err
}

func parseDDG(body string, linkRE, snippetRE *regexp.Regexp) []Result {
	links := linkRE.FindAllStringSubmatch(body, -1)
	snippets := snippetRE.FindAllStringSubmatch(body, -1)
	out := make([]Result, 0, len(links))
	for i, m := range links {
		link := unwrapDDG(m[1])
		title := clean(m[2])
		if link == "" || title == "" || strings.HasPrefix(link, "https://duckduckgo.com/y.js") {
			continue
		}
		snippet := ""
		if i < len(snippets) {
			snippet = clean(snippets[i][1])
		}
		out = append(out, Result{Title: title, URL: link, Snippet: snippet, Source: "ddg"})
	}
	return out
}

// unwrapDDG turns a //duckduckgo.com/l/?uddg=... redirect into the real target.
func unwrapDDG(href string) string {
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if target := u.Query().Get("uddg"); target != "" {
		return target
	}
	if !strings.HasPrefix(u.Scheme, "http") {
		return ""
	}
	return u.String()
}

func ddgSafe(level string) string {
	switch level {
	case "off":
		return "-2"
	case "strict":
		return "1"
	default:
		return "-1"
	}
}

func ddgRegion(region string) string {
	if region == "" {
		return "wt-wt"
	}
	r := strings.ToLower(region)
	if strings.Contains(r, "-") {
		return r
	}
	return r + "-" + r
}
