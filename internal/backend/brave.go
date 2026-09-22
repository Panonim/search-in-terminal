package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Panonim/search-in-terminal/internal/config"
)

const braveEndpoint = "https://api.search.brave.com/res/v1/web/search"

type brave struct {
	cfg config.Config
	key string
}

func newBrave(cfg config.Config) *brave { return &brave{cfg: cfg, key: cfg.BraveKey()} }

func (b *brave) Name() string  { return "brave" }
func (b *brave) Label() string { return "brave" }

func (b *brave) Ready() (bool, string) {
	if b.key == "" {
		return false, "no API key: set $SIT_BRAVE_API_KEY or backends.brave.api_key"
	}
	return true, ""
}

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
	if ok, why := b.Ready(); !ok {
		return nil, errors.New("brave: " + why)
	}
	count := b.cfg.General.ResultsPerPage
	if count > 20 {
		count = 20
	}
	q := url.Values{}
	q.Set("q", query)
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
