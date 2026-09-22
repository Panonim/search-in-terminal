package backend

import (
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

type searxng struct {
	cfg       config.Config
	name      string
	instances []string
}

func newSearXNG(cfg config.Config) *searxng {
	instances := append([]string{cfg.Backends.SearXNG.Instance}, cfg.Backends.SearXNG.Fallbacks...)
	return &searxng{cfg: cfg, name: "searxng", instances: instances}
}

func (s *searxng) Name() string { return s.name }

func (s *searxng) Label() string {
	if h := hostOf(s.instances[0]); h != "" {
		return "searxng:" + h
	}
	return "searxng"
}

func (s *searxng) Ready() (bool, string) {
	for _, in := range s.instances {
		if strings.TrimSpace(in) != "" {
			return true, ""
		}
	}
	return false, "no instance configured: set backends.searxng.instance"
}

type searxResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
		Engine  string `json:"engine"`
	} `json:"results"`
}

func (s *searxng) Search(ctx context.Context, query string, page int) ([]Result, error) {
	if ok, why := s.Ready(); !ok {
		return nil, errors.New(s.name + ": " + why)
	}
	var lastErr error
	for _, instance := range s.instances {
		instance = strings.TrimSpace(strings.TrimSuffix(instance, "/"))
		if instance == "" {
			continue
		}
		results, err := s.query(ctx, instance, query, page)
		if err != nil {
			lastErr = err
			continue
		}
		if len(results) > 0 || len(s.instances) == 1 {
			return results, nil
		}
	}
	if lastErr != nil {
		if len(s.instances) > 1 {
			return nil, fmt.Errorf("searxng: no instance answered (last: %w)", lastErr)
		}
		return nil, lastErr
	}
	return nil, nil
}

func (s *searxng) query(ctx context.Context, instance, query string, page int) ([]Result, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("format", "json")
	q.Set("pageno", strconv.Itoa(max(1, page)))
	q.Set("safesearch", searxSafe(s.cfg.General.SafeSearch))
	if r := s.cfg.General.Region; r != "" {
		q.Set("language", r)
	}

	resp, err := get(ctx, httpClient(s.cfg), instance+"/search?"+q.Encode(), map[string]string{"Accept": "application/json"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError(hostOf(instance), resp)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "json") {
		return nil, fmt.Errorf("%s: JSON API disabled on this instance", hostOf(instance))
	}

	var parsed searxResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("%s: %w", hostOf(instance), err)
	}
	limit := s.cfg.General.ResultsPerPage
	out := make([]Result, 0, len(parsed.Results))
	for i, r := range parsed.Results {
		if limit > 0 && i >= limit {
			break
		}
		out = append(out, Result{
			Title:   clean(r.Title),
			URL:     r.URL,
			Snippet: clean(r.Content),
			Source:  r.Engine,
		})
	}
	return out, nil
}

func searxSafe(level string) string {
	switch level {
	case "off":
		return "0"
	case "strict":
		return "2"
	default:
		return "1"
	}
}
