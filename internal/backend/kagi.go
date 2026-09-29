package backend

import (
	"bytes"
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

// The endpoints are vars so tests can point them at a local server.
var (
	kagiAPIEndpoint = "https://kagi.com/api/v1/search"
	kagiWebEndpoint = "https://kagi.com/html/search"
)

// Without an API key, kagi scrapes the no-JS results page with the subscription's session token.
type kagi struct {
	cfg     config.Config
	key     string
	session string
}

func newKagi(cfg config.Config) *kagi {
	return &kagi{cfg: cfg, key: cfg.KagiKey(), session: cfg.KagiSession()}
}

func (k *kagi) Name() string { return "kagi" }

func (k *kagi) Label() string {
	if k.key == "" {
		return "kagi (web)"
	}
	return "kagi"
}

func (k *kagi) Ready() (bool, string) {
	if k.key == "" && k.session == "" {
		return false, "no credentials: set backends.kagi.api_key or backends.kagi.session_token"
	}
	return true, ""
}

func (k *kagi) Search(ctx context.Context, query string, page int) ([]Result, error) {
	if ok, why := k.Ready(); !ok {
		return nil, errors.New("kagi: " + why)
	}
	if k.key == "" {
		return k.searchWeb(ctx, query, page)
	}
	return k.searchAPI(ctx, query, page)
}

type kagiResponse struct {
	Data struct {
		Search []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
		} `json:"search"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (k *kagi) searchAPI(ctx context.Context, query string, page int) ([]Result, error) {
	// The API serves at most ten pages.
	if page > 10 {
		return nil, nil
	}
	pq := parseQuery(query)
	fields := map[string]any{
		"query":       pq.text,
		"page":        max(1, page),
		"limit":       k.cfg.General.ResultsPerPage,
		"safe_search": k.cfg.General.SafeSearch != "off",
	}
	if pq.after != "" || pq.before != "" {
		fields["filters"] = struct {
			After  string `json:"after,omitempty"`
			Before string `json:"before,omitempty"`
		}{pq.after, pq.before}
	}
	body, _ := json.Marshal(fields)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kagiAPIEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+k.key)
	resp, err := httpClient(k.cfg).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var parsed kagiResponse
	decodeErr := json.NewDecoder(resp.Body).Decode(&parsed)
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("kagi: API key rejected (401)")
	}
	if len(parsed.Errors) > 0 && parsed.Errors[0].Message != "" {
		return nil, fmt.Errorf("kagi: %s", parsed.Errors[0].Message)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("kagi", resp)
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("kagi: %w", decodeErr)
	}
	out := make([]Result, 0, len(parsed.Data.Search))
	for _, r := range parsed.Data.Search {
		out = append(out, Result{Title: clean(r.Title), URL: r.URL, Snippet: clean(r.Snippet), Source: "kagi"})
	}
	return out, nil
}

// Each organic result, grouped or not, is a block classed search-result or __srgi.
var kagiScraper = scraper{
	source: "kagi",
	own:    []string{"kagi.com"},
	block:  regexp.MustCompile(`(?i)\bclass\s*=\s*["'](?:[^"']*\s)?(?:search-result|__srgi)["'\s]`),
}

func (k *kagi) searchWeb(ctx context.Context, query string, page int) ([]Result, error) {
	pq := parseQuery(query)
	q := url.Values{}
	q.Set("q", pq.text)
	if page > 1 {
		q.Set("batch", strconv.Itoa(page))
	}
	if pq.after != "" {
		q.Set("from_date", pq.after)
	}
	if pq.before != "" {
		q.Set("to_date", pq.before)
	}
	resp, err := get(ctx, httpClient(k.cfg), kagiWebEndpoint+"?"+q.Encode(), map[string]string{
		"Accept": "text/html",
		"Cookie": "kagi_session=" + k.session,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// A dead session is redirected to the signup page instead of failing.
	if !strings.HasSuffix(resp.Request.URL.Path, "/html/search") || resp.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("kagi: session token invalid or expired, copy a fresh session link")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("kagi", resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return kagiScraper.parse(string(raw)), nil
}
