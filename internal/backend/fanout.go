package backend

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/Panonim/search-in-terminal/internal/config"
)

// fanout queries several backends at once and merges their results into one list.
type fanout struct {
	engines []Backend
}

func newFanout(cfg config.Config) (*fanout, error) {
	f := &fanout{}
	for _, name := range cfg.Backends.Fanout.Engines {
		b, err := newCached(name, cfg)
		if err != nil {
			return nil, err
		}
		f.engines = append(f.engines, b)
	}
	return f, nil
}

func (f *fanout) Name() string { return "fanout" }

func (f *fanout) Label() string {
	names := make([]string, len(f.engines))
	for i, b := range f.engines {
		names[i] = b.Name()
	}
	return strings.Join(names, "+")
}

func (f *fanout) Ready() (bool, string) {
	for _, b := range f.engines {
		if ok, _ := b.Ready(); ok {
			return true, ""
		}
	}
	return false, "no ready backend in backends.fanout.engines"
}

// Search reports errors only when no engine returned anything, so one blocked engine does not hide the rest.
func (f *fanout) Search(ctx context.Context, query string, page int) ([]Result, error) {
	if ok, why := f.Ready(); !ok {
		return nil, errors.New("fanout: " + why)
	}
	lists := make([][]Result, len(f.engines))
	errs := make([]error, len(f.engines))
	var wg sync.WaitGroup
	for i, b := range f.engines {
		if ok, _ := b.Ready(); ok {
			wg.Go(func() { lists[i], errs[i] = b.Search(ctx, query, page) })
		}
	}
	wg.Wait()
	if out := merge(lists); len(out) > 0 {
		return out, nil
	}
	return nil, errors.Join(errs...)
}

// merge interleaves the lists by rank and folds a page found by several engines into its first copy.
func merge(lists [][]Result) []Result {
	var out []Result
	at := map[string]int{}
	for rank := 0; ; rank++ {
		more := false
		for _, list := range lists {
			if rank >= len(list) {
				continue
			}
			more = true
			r := list[rank]
			key := urlKey(r.URL)
			i, dup := at[key]
			if !dup {
				at[key] = len(out)
				out = append(out, r)
				continue
			}
			first := &out[i]
			if !slices.Contains(strings.Split(first.Source, "+"), r.Source) {
				first.Source += "+" + r.Source
			}
			if first.Snippet == "" {
				first.Snippet = r.Snippet
			}
			if first.FaviconURL == "" {
				first.FaviconURL = r.FaviconURL
			}
		}
		if !more {
			return out
		}
	}
}

// urlKey ignores scheme, www., fragment and a trailing slash, which engines report inconsistently.
func urlKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	return host + strings.TrimSuffix(u.EscapedPath(), "/") + "?" + u.RawQuery
}
