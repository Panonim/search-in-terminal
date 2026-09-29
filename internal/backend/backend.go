// Package backend defines the search backend interface and its implementations.
package backend

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Panonim/search-in-terminal/internal/config"
)

type Result struct {
	Title      string
	URL        string
	Snippet    string
	FaviconURL string
	Source     string
	// Cached reports whether this result was served from the on-disk cache
	// rather than fetched live; not persisted in the cache entry itself.
	Cached bool `json:"-"`
}

// Suggester is implemented by backends that can autocomplete a query.
type Suggester interface {
	Suggest(ctx context.Context, query string) []string
}

// Pager is implemented by backends whose next page needs a token from the page before, so the cache can keep it.
type Pager interface {
	Cursor(query string, page int) string
	SetCursor(query string, page int, cursor string)
}

type Backend interface {
	Name() string
	Label() string
	Search(ctx context.Context, query string, page int) ([]Result, error)
	// Ready reports whether the backend can run, e.g. whether a key is present.
	Ready() (bool, string)
}

// Order is the backend cycle order used by Tab.
var Order = []string{"ddg", "degoog", "searxng", "brave", "kagi", "fanout"}

func New(name string, cfg config.Config) (Backend, error) {
	var b Backend
	var err error
	if strings.EqualFold(name, "fanout") {
		b, err = newFanout(cfg)
	} else {
		b, err = newCached(name, cfg)
	}
	if err != nil {
		return nil, err
	}
	return filtered{b}, nil
}

// newCached caches each engine on its own, so fanout members share entries with standalone searches.
func newCached(name string, cfg config.Config) (Backend, error) {
	b, err := newRaw(name, cfg)
	if err != nil {
		return nil, err
	}
	return withCache(b, cfg), nil
}

func newRaw(name string, cfg config.Config) (Backend, error) {
	switch strings.ToLower(name) {
	case "ddg", "duckduckgo":
		return newDDG(cfg), nil
	case "brave":
		return newBrave(cfg), nil
	case "searxng", "searx":
		return newSearXNG(cfg), nil
	case "degoog":
		return newDegoog(cfg), nil
	case "kagi":
		return newKagi(cfg), nil
	}
	return nil, fmt.Errorf("unknown backend %q (have: %s)", name, strings.Join(Order, ", "))
}

// All returns every backend in cycle order, for status listings.
func All(cfg config.Config) []Backend {
	var out []Backend
	for _, name := range Order {
		if b, err := New(name, cfg); err == nil {
			out = append(out, b)
		}
	}
	return out
}

func Next(name string, step int) string {
	idx := 0
	for i, n := range Order {
		if n == name {
			idx = i
		}
	}
	return Order[(idx+step+len(Order))%len(Order)]
}

const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36"

func httpClient(cfg config.Config) *http.Client {
	return &http.Client{Timeout: cfg.Timeout(), CheckRedirect: stayOnHost}
}

// stayOnHost stops at a redirect to another site, which is an engine running its own !bang rather than a results page.
func stayOnHost(req *http.Request, via []*http.Request) error {
	if !strings.EqualFold(req.URL.Hostname(), via[0].URL.Hostname()) {
		return http.ErrUseLastResponse
	}
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	return nil
}

func get(ctx context.Context, client *http.Client, rawURL string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return client.Do(req)
}

func statusError(backend string, resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return fmt.Errorf("%s: rate limited (429), try another backend", backend)
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%s: rejected the request (%d)", backend, resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); resp.StatusCode/100 == 3 && loc != "" {
		return fmt.Errorf("%s: sent the query to %s, likely its own !bang (add yours under bangs)", backend, hostOf(loc))
	}
	return fmt.Errorf("%s: http %d", backend, resp.StatusCode)
}

var tagRE = regexp.MustCompile(`<[^>]*>`)

func clean(s string) string {
	s = tagRE.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}
