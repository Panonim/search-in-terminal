package backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Panonim/search-in-terminal/internal/config"
)

// cached wraps a Backend with an on-disk result cache so repeat searches within
// the TTL window are served locally instead of hitting the engine again.
type cached struct {
	Backend
	dir string
	ttl time.Duration
	// settings folds result-shaping options into the key, so edited settings never serve old results.
	settings string
}

func withCache(b Backend, cfg config.Config) Backend {
	ttl := cfg.CacheTTL()
	if ttl <= 0 {
		return b
	}
	g, d := cfg.General, cfg.Backends.Degoog
	settings := fmt.Sprint(g.Region, g.SafeSearch, g.ResultsPerPage, d.Type, d.Engines)
	return &cached{Backend: b, dir: config.ResultsCacheDir(), ttl: ttl, settings: settings}
}

type cacheEntry struct {
	Saved   time.Time `json:"saved"`
	Query   string    `json:"query"`
	Compact string    `json:"compact"`
	Results []Result  `json:"results"`
	Next    string    `json:"next,omitempty"`
}

func (c *cached) Search(ctx context.Context, query string, page int) ([]Result, error) {
	key := normalizeQuery(query)
	// compact keeps word order but drops spacing, so "speed test" finds "speedtest" and "what's" finds "whats".
	compact := strings.Join(queryWords(query), "")
	path := filepath.Join(c.dir, c.prefix(page)+"-"+hash(key)[:16]+".json")
	entry, ok := c.read(path)
	if !ok {
		entry, ok = c.closest(key, compact, page)
	}
	pager, paged := c.Backend.(Pager)
	if ok {
		// Restoring the saved token lets the next page load without refetching this one.
		if paged && entry.Next != "" {
			pager.SetCursor(query, page+1, entry.Next)
		}
		for i := range entry.Results {
			entry.Results[i].Cached = true
		}
		return entry.Results, nil
	}

	results, err := c.Backend.Search(ctx, query, page)
	if err != nil {
		return results, err
	}
	next := ""
	if paged {
		next = pager.Cursor(query, page+1)
	}
	// An empty page is often a transient engine hiccup, so it is retried rather than cached.
	if len(results) > 0 {
		c.write(path, cacheEntry{Saved: time.Now(), Query: key, Compact: compact, Results: results, Next: next})
	}
	return results, nil
}

// Suggest forwards to the wrapped backend when it supports suggestions, keeping
// the Suggester type assertion in the UI layer working through the wrapper.
func (c *cached) Suggest(ctx context.Context, query string) []string {
	if s, ok := c.Backend.(Suggester); ok {
		return s.Suggest(ctx, query)
	}
	return nil
}

// prefix groups entries of one backend, settings and page so fuzzy lookup only scans those.
func (c *cached) prefix(page int) string {
	return hash(c.Backend.Name() + "\x00" + c.Backend.Label() + "\x00" + c.settings + "\x00" + strconv.Itoa(page))[:8]
}

// closest returns the fresh entry whose query differs only by word order, spacing and small typos.
func (c *cached) closest(key, compact string, page int) (cacheEntry, bool) {
	paths, _ := filepath.Glob(filepath.Join(c.dir, c.prefix(page)+"-*.json"))
	words := strings.Fields(key)
	var best cacheEntry
	bestDist, found := 0, false
	for _, p := range paths {
		entry, ok := c.read(p)
		if !ok {
			continue
		}
		d, near := queryDistance(words, strings.Fields(entry.Query))
		if entry.Compact == compact {
			d, near = 0, true
		}
		if near && (!found || d < bestDist) {
			best, bestDist, found = entry, d, true
		}
	}
	return best, found
}

// read deletes expired or corrupt entries on sight, so stale results never linger on disk.
func (c *cached) read(path string) (cacheEntry, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cacheEntry{}, false
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil || time.Since(entry.Saved) >= c.ttl {
		os.Remove(path)
		return cacheEntry{}, false
	}
	return entry, true
}

func (c *cached) write(path string, entry cacheEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(c.dir, ".result-*.tmp")
	if err != nil {
		return
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return
	}
	tmp.Close()
	os.Rename(name, path)
}

// CleanCache removes expired result entries now and then once per TTL, and never returns; with caching off it drops them all.
func CleanCache(cfg config.Config) {
	ttl := cfg.CacheTTL()
	if ttl <= 0 {
		os.RemoveAll(config.ResultsCacheDir())
		return
	}
	tick := time.NewTicker(max(ttl, time.Minute))
	defer tick.Stop()
	for {
		pruneCache(config.ResultsCacheDir(), ttl)
		<-tick.C
	}
}

func pruneCache(dir string, ttl time.Duration) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) >= ttl {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// normalizeQuery lowercases, strips punctuation and sorts words so reordered queries share a key.
func normalizeQuery(q string) string {
	words := queryWords(q)
	slices.Sort(words)
	return strings.Join(words, " ")
}

func queryWords(q string) []string {
	return strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// queryDistance pairs every word with a close unused word in the other query and sums the edits.
func queryDistance(a, b []string) (int, bool) {
	if len(a) != len(b) {
		return 0, false
	}
	used := make([]bool, len(b))
	total := 0
	for _, w := range a {
		pick, pickDist := -1, 0
		for i, v := range b {
			if used[i] {
				continue
			}
			if d := editDistance(w, v); d <= allowedEdits(w) && (pick < 0 || d < pickDist) {
				pick, pickDist = i, d
			}
		}
		if pick < 0 {
			return 0, false
		}
		used[pick] = true
		total += pickDist
	}
	return total, true
}

// allowedEdits keeps short words and numbers exact so "2024" never matches "2025".
func allowedEdits(w string) int {
	n := len([]rune(w))
	switch {
	case strings.ContainsFunc(w, unicode.IsDigit), n <= 2:
		return 0
	case n <= 5:
		return 1
	}
	return 2
}

// editDistance is Levenshtein with adjacent swaps counted as one edit.
func editDistance(a, b string) int {
	s, t := []rune(a), []rune(b)
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(s); i++ {
		for j := 1; j <= len(t); j++ {
			cost := 1
			if s[i-1] == t[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && s[i-1] == t[j-2] && s[i-2] == t[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(s)][len(t)]
}
