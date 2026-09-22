package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Panonim/search-in-terminal/internal/config"
)

// Degoog is a self-hosted search aggregator; sit talks to its /api/search endpoint.
type degoog struct {
	cfg      config.Config
	instance string
	key      string
}

func newDegoog(cfg config.Config) *degoog {
	return &degoog{
		cfg:      cfg,
		instance: strings.TrimSuffix(strings.TrimSpace(cfg.DegoogInstance()), "/"),
		key:      cfg.DegoogKey(),
	}
}

func (d *degoog) Name() string { return "degoog" }

func (d *degoog) Label() string {
	if h := hostOf(d.instance); h != "" {
		return "degoog:" + h
	}
	return "degoog"
}

func (d *degoog) Ready() (bool, string) {
	if d.instance == "" {
		return false, "no instance: set backends.degoog.instance or $SIT_DEGOOG_URL"
	}
	return true, ""
}

type degoogResponse struct {
	Error   string `json:"error"`
	Results []struct {
		Title   string   `json:"title"`
		URL     string   `json:"url"`
		Snippet string   `json:"snippet"`
		Content string   `json:"content"`
		Source  string   `json:"source"`
		Sources []string `json:"sources"`
	} `json:"results"`
}

func (d *degoog) Search(ctx context.Context, query string, page int) ([]Result, error) {
	if ok, why := d.Ready(); !ok {
		return nil, errors.New("degoog: " + why)
	}
	page = max(1, min(page, 10))

	resp, err := d.request(ctx, query, page)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return nil, errors.New("degoog: instance is password protected, set backends.degoog.api_key")
	default:
		return nil, statusError("degoog", resp)
	}

	var parsed degoogResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("degoog: %w", err)
	}
	if parsed.Error != "" {
		return nil, fmt.Errorf("degoog: %s", parsed.Error)
	}

	limit := d.cfg.General.ResultsPerPage
	out := make([]Result, 0, len(parsed.Results))
	for i, r := range parsed.Results {
		if limit > 0 && i >= limit {
			break
		}
		snippet := r.Snippet
		if snippet == "" {
			snippet = r.Content
		}
		source := r.Source
		if len(r.Sources) > 1 {
			source = strings.Join(r.Sources, "+")
		}
		out = append(out, Result{
			Title:   clean(r.Title),
			URL:     r.URL,
			Snippet: clean(snippet),
			Source:  source,
		})
	}
	return out, nil
}

// request uses the POST form only when specific engines are pinned, since GET cannot express an engine list.
func (d *degoog) request(ctx context.Context, query string, page int) (*http.Response, error) {
	searchType := d.cfg.Backends.Degoog.Type
	if searchType == "" {
		searchType = "web"
	}

	var req *http.Request
	var err error
	if engines := d.cfg.Backends.Degoog.Engines; len(engines) > 0 {
		body, _ := json.Marshal(map[string]any{
			"query":    query,
			"type":     searchType,
			"page":     page,
			"lang":     d.cfg.General.Region,
			"safeMode": d.cfg.General.SafeSearch,
			"engines":  engines,
		})
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, d.instance+"/api/search", bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	} else {
		q := url.Values{}
		q.Set("q", query)
		q.Set("type", searchType)
		q.Set("page", strconv.Itoa(page))
		q.Set("safeMode", d.cfg.General.SafeSearch)
		if r := d.cfg.General.Region; r != "" {
			q.Set("lang", r)
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, d.instance+"/api/search?"+q.Encode(), nil)
	}
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if d.key != "" {
		req.Header.Set("Authorization", "Bearer "+d.key)
	}
	return httpClient(d.cfg).Do(req)
}

// Suggest asks the instance for autocomplete suggestions, returning nothing when unavailable.
func (d *degoog) Suggest(ctx context.Context, query string) []string {
	if ok, _ := d.Ready(); !ok || strings.TrimSpace(query) == "" {
		return nil
	}
	headers := map[string]string{"Accept": "application/json"}
	if d.key != "" {
		headers["Authorization"] = "Bearer " + d.key
	}
	resp, err := get(ctx, httpClient(d.cfg), d.instance+"/api/suggest?q="+url.QueryEscape(query), headers)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var items []struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it.Text != "" {
			out = append(out, it.Text)
		}
	}
	return out
}
