package backend

import (
	"cmp"
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
	weights []float64
}

func newFanout(cfg config.Config) (*fanout, error) {
	f := &fanout{}
	weights := cfg.FanoutWeights()
	for _, name := range cfg.Backends.Fanout.Engines {
		b, err := newCached(name, cfg)
		if err != nil {
			return nil, err
		}
		w, ok := weights[strings.ToLower(name)]
		if !ok {
			w = 1
		}
		f.engines = append(f.engines, b)
		f.weights = append(f.weights, w)
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
	if out := merge(lists, f.weights); len(out) > 0 {
		return out, nil
	}
	return nil, errors.Join(errs...)
}

// rrfK damps the gap between top ranks, the usual constant for reciprocal rank fusion.
const rrfK = 60

// merge ranks results by reciprocal rank fusion and folds a page found by several engines into its first copy.
func merge(lists [][]Result, weights []float64) []Result {
	var out []Result
	var score []float64
	at := map[string]int{}
	for rank := 0; ; rank++ {
		more := false
		for e, list := range lists {
			if rank >= len(list) {
				continue
			}
			more = true
			r := list[rank]
			w := 1.0
			if e < len(weights) {
				w = weights[e]
			}
			s := w / float64(rrfK+rank)
			key := URLKey(r.URL)
			i, dup := at[key]
			if !dup {
				at[key] = len(out)
				out = append(out, r)
				score = append(score, s)
				continue
			}
			score[i] += s
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
			break
		}
	}
	idx := make([]int, len(out))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int { return cmp.Compare(score[b], score[a]) })
	sorted := make([]Result, len(out))
	for i, j := range idx {
		sorted[i] = out[j]
	}
	return sorted
}

// URLKey ignores scheme, www., fragment, a trailing slash, tracking parameters and parameter order, which engines report inconsistently.
func URLKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	q := u.Query()
	for k := range q {
		if tracking(k) {
			q.Del(k)
		}
	}
	return host + strings.TrimSuffix(u.EscapedPath(), "/") + "?" + q.Encode()
}

func tracking(param string) bool {
	p := strings.ToLower(param)
	return strings.HasPrefix(p, "utm_") || strings.HasPrefix(p, "mc_") ||
		p == "fbclid" || p == "gclid" || p == "msclkid" || p == "ref"
}
