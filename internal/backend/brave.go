package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Panonim/search-in-terminal/internal/config"
)

const braveEndpoint = "https://api.search.brave.com/res/v1/web/search"

// braveWebEndpoint is a var so tests can point it at a local server.
var braveWebEndpoint = "https://search.brave.com/search"

// Without an API key, brave falls back to scraping search.brave.com on a best-effort basis.
type brave struct {
	cfg config.Config
	key string
}

func newBrave(cfg config.Config) *brave { return &brave{cfg: cfg, key: cfg.BraveKey()} }

func (b *brave) Name() string { return "brave" }

func (b *brave) Label() string {
	if b.key == "" {
		return "brave (web)"
	}
	return "brave"
}

func (b *brave) Ready() (bool, string) { return true, "" }

type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
			MetaURL     struct {
				Favicon string `json:"favicon"`
			} `json:"meta_url"`
			Profile struct {
				Img string `json:"img"`
			} `json:"profile"`
		} `json:"results"`
	} `json:"web"`
}

func (b *brave) Search(ctx context.Context, query string, page int) ([]Result, error) {
	if b.key == "" {
		return b.searchWeb(ctx, query, page)
	}
	return b.searchAPI(ctx, query, page)
}

func (b *brave) searchAPI(ctx context.Context, query string, page int) ([]Result, error) {
	count := b.cfg.General.ResultsPerPage
	if count > 20 {
		count = 20
	}
	pq := parseQuery(query)
	q := url.Values{}
	q.Set("q", pq.text)
	if span := pq.span("to"); span != "" {
		q.Set("freshness", span)
	}
	q.Set("count", strconv.Itoa(count))
	q.Set("offset", strconv.Itoa(max(0, page-1)))
	q.Set("safesearch", braveSafe(b.cfg.General.SafeSearch))
	if r := b.cfg.General.Region; r != "" {
		q.Set("country", r)
	}

	// The key travels in a header only, never in the URL or an error string.
	resp, err := get(ctx, httpClient(b.cfg), braveEndpoint+"?"+q.Encode(), map[string]string{
		"Accept":               "application/json",
		"X-Subscription-Token": b.key,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("brave: API key rejected (%d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("brave", resp)
	}

	var parsed braveResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(parsed.Web.Results))
	for _, r := range parsed.Web.Results {
		favicon := r.MetaURL.Favicon
		if favicon == "" {
			favicon = r.Profile.Img
		}
		out = append(out, Result{
			Title:      clean(r.Title),
			URL:        r.URL,
			Snippet:    clean(r.Description),
			FaviconURL: favicon,
			Source:     "brave",
		})
	}
	return out, nil
}

var braveScraper = scraper{
	source: "brave",
	own:    []string{"brave.com", "brave.app"},
	block:  regexp.MustCompile(`(?i)\bdata-type\s*=\s*["']?web\b`),
}

func (b *brave) searchWeb(ctx context.Context, query string, page int) ([]Result, error) {
	pq := parseQuery(query)
	q := url.Values{}
	q.Set("q", pq.text)
	q.Set("source", "web")
	if span := pq.span("to"); span != "" {
		q.Set("tf", span)
	}
	if page > 1 {
		q.Set("offset", strconv.Itoa(page-1))
	}
	resp, err := get(ctx, httpClient(b.cfg), braveWebEndpoint+"?"+q.Encode(), map[string]string{
		"Accept":          "text/html",
		"Accept-Language": "en-US,en;q=0.9",
		"Cookie":          "safesearch=" + braveSafe(b.cfg.General.SafeSearch),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || strings.Contains(resp.Request.URL.Path, "captcha") {
		return nil, errors.New("brave: rate limited, set an API key or switch backend")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("brave", resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return braveScraper.parse(string(raw)), nil
}

func braveSafe(level string) string {
	switch level {
	case "off":
		return "off"
	case "strict":
		return "strict"
	default:
		return "moderate"
	}
}
