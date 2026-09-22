package backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
}

func withCache(b Backend, cfg config.Config) Backend {
	ttl := cfg.CacheTTL()
	if ttl <= 0 {
		return b
	}
	return &cached{Backend: b, dir: config.ResultsCacheDir(), ttl: ttl}
}

type cacheEntry struct {
	Saved   time.Time `json:"saved"`
	Query   string    `json:"query"`
	Results []Result  `json:"results"`
}

func (c *cached) Search(ctx context.Context, query string, page int) ([]Result, error) {
	key := normalizeQuery(query)
	path := filepath.Join(c.dir, c.prefix(page)+"-"+hash(key)[:16]+".json")
	entry, ok := c.read(path)
	if !ok {
		entry, ok = c.closest(key, page)
	}
	if ok {
		for i := range entry.Results {
			entry.Results[i].Cached = true
		}
		return entry.Results, nil
	}

	results, err := c.Backend.Search(ctx, query, page)
	if err != nil {
		return results, err
	}
	c.write(path, key, results)
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

// prefix groups entries of one backend and page so fuzzy lookup only scans those.
func (c *cached) prefix(page int) string {
	return hash(c.Backend.Name() + "\x00" + c.Backend.Label() + "\x00" + strconv.Itoa(page))[:8]
}

// closest returns the fresh entry whose query differs only by word order and small typos.
func (c *cached) closest(key string, page int) (cacheEntry, bool) {
	paths, _ := filepath.Glob(filepath.Join(c.dir, c.prefix(page)+"-*.json"))
	words := strings.Fields(key)
	var best cacheEntry
	bestDist, found := 0, false
	for _, p := range paths {
		entry, ok := c.read(p)
		if !ok {
			continue
		}
		if d, ok := queryDistance(words, strings.Fields(entry.Query)); ok && (!found || d < bestDist) {
			best, bestDist, found = entry, d, true
		}
	}
	return best, found
}

func (c *cached) read(path string) (cacheEntry, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cacheEntry{}, false
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil || time.Since(entry.Saved) >= c.ttl {
		return cacheEntry{}, false
	}
	return entry, true
}

func (c *cached) write(path, key string, results []Result) {
	data, err := json.Marshal(cacheEntry{Saved: time.Now(), Query: key, Results: results})
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

// CleanCache removes expired result entries now and then once per TTL, and never returns.
func CleanCache(cfg config.Config) {
	ttl := cfg.CacheTTL()
	if ttl <= 0 {
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
	words := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	slices.Sort(words)
	return strings.Join(words, " ")
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
