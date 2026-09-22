package backend

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Panonim/search-in-terminal/internal/config"
)

// The DuckDuckGo backend scrapes the Lite/HTML endpoints; unofficial and best-effort, it can break without notice.
type ddg struct {
	cfg config.Config
}

func newDDG(cfg config.Config) *ddg { return &ddg{cfg: cfg} }

func (d *ddg) Name() string          { return "ddg" }
func (d *ddg) Label() string         { return "duckduckgo" }
func (d *ddg) Ready() (bool, string) { return true, "" }

var (
	liteLinkRE    = regexp.MustCompile(`(?is)<a\s+[^>]*href=["']([^"']+)["'][^>]*class=['"]result-link['"][^>]*>(.*?)</a>`)
	liteSnippetRE = regexp.MustCompile(`(?is)<td[^>]*class=['"]result-snippet['"][^>]*>(.*?)</td>`)
	htmlLinkRE    = regexp.MustCompile(`(?is)<a\s+[^>]*class=["'][^"']*result__a[^"']*["'][^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	htmlSnippetRE = regexp.MustCompile(`(?is)class=["'][^"']*result__snippet[^"']*["'][^>]*>(.*?)</a>`)
)

func (d *ddg) Search(ctx context.Context, query string, page int) ([]Result, error) {
	offset := max(0, page-1) * 20
	form := url.Values{}
	form.Set("q", query)
	form.Set("kl", ddgRegion(d.cfg.General.Region))
	form.Set("kp", ddgSafe(d.cfg.General.SafeSearch))
	if offset > 0 {
		form.Set("s", strconv.Itoa(offset))
		form.Set("dc", strconv.Itoa(offset+1))
		form.Set("v", "l")
		form.Set("o", "json")
		form.Set("api", "d.js")
	}

	body, err := d.post(ctx, "https://lite.duckduckgo.com/lite/", form)
	if err != nil {
		return nil, err
	}
	results := parseDDG(body, liteLinkRE, liteSnippetRE)
	if len(results) == 0 {
		if body, err = d.post(ctx, "https://html.duckduckgo.com/html/", form); err != nil {
			return nil, err
		}
		results = parseDDG(body, htmlLinkRE, htmlSnippetRE)
	}
	if len(results) == 0 && strings.Contains(body, "anomaly") {
		return nil, errors.New("duckduckgo: blocked this request, try again later or switch backend")
	}
	return results, nil
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
