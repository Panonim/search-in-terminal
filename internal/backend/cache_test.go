package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Panonim/search-in-terminal/internal/config"
)

type countingBackend struct {
	calls int
	empty bool
}

func (c *countingBackend) Name() string          { return "fake" }
func (c *countingBackend) Label() string         { return "fake" }
func (c *countingBackend) Ready() (bool, string) { return true, "" }
func (c *countingBackend) Search(ctx context.Context, query string, page int) ([]Result, error) {
	c.calls++
	if c.empty {
		return nil, nil
	}
	return []Result{{Title: query, URL: "https://example.com"}}, nil
}

func TestCachedSearchServesRepeatQueryFromDisk(t *testing.T) {
	t.Setenv("SIT_CACHE_DIR", t.TempDir())

	cfg := testConfig()
	cfg.General.CacheTTLSeconds = 300
	inner := &countingBackend{}
	b := withCache(inner, cfg)

	for i := 0; i < 3; i++ {
		res, err := b.Search(context.Background(), "rust lifetimes", 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 || res[0].Title != "rust lifetimes" {
			t.Fatalf("unexpected results: %+v", res)
		}
	}
	if inner.calls != 1 {
		t.Errorf("want engine hit once, got %d", inner.calls)
	}

	if res, err := b.Search(context.Background(), "other query", 1); err != nil || len(res) != 1 {
		t.Fatalf("different query should still succeed: %+v, %v", res, err)
	}
	if inner.calls != 2 {
		t.Errorf("want a new engine hit for a different query, got %d calls", inner.calls)
	}
}

func TestCachedSearchMatchesReorderedAndMistypedQuery(t *testing.T) {
	t.Setenv("SIT_CACHE_DIR", t.TempDir())

	cfg := testConfig()
	cfg.General.CacheTTLSeconds = 300
	inner := &countingBackend{}
	b := withCache(inner, cfg)

	b.Search(context.Background(), "best place to eat in berlin", 1)
	res, err := b.Search(context.Background(), "pest aet place to in Berlin?", 1)
	if err != nil || len(res) != 1 || !res[0].Cached {
		t.Fatalf("want cached hit, got %+v, %v", res, err)
	}
	if inner.calls != 1 {
		t.Errorf("want engine hit once, got %d", inner.calls)
	}

	for _, q := range []string{"best place to eat in paris", "best place to eat in berlin 2024", "best place to eat in berlin", "best place to eat in berlin"} {
		b.Search(context.Background(), q, 2)
	}
	if inner.calls != 4 {
		t.Errorf("want different queries and pages to miss, got %d calls", inner.calls)
	}
}

func TestCachedSearchIgnoresSpacingAndApostrophes(t *testing.T) {
	t.Setenv("SIT_CACHE_DIR", t.TempDir())
	cfg := testConfig()
	cfg.General.CacheTTLSeconds = 300
	inner := &countingBackend{}
	b := withCache(inner, cfg)

	for _, pair := range [][2]string{{"speed test", "speedtest"}, {"type script generics", "typescript generics"}, {"what's new in go", "whats new in go"}} {
		b.Search(context.Background(), pair[0], 1)
		if res, _ := b.Search(context.Background(), pair[1], 1); len(res) != 1 || !res[0].Cached {
			t.Errorf("%q should reuse the %q entry", pair[1], pair[0])
		}
	}
	if inner.calls != 3 {
		t.Errorf("want one engine hit per pair, got %d", inner.calls)
	}
}

func TestCacheServesNothingStale(t *testing.T) {
	t.Setenv("SIT_CACHE_DIR", t.TempDir())
	cfg := testConfig()
	cfg.General.CacheTTLSeconds = 300
	inner := &countingBackend{}
	withCache(inner, cfg).Search(context.Background(), "q", 1)

	cfg.General.Region = "de"
	if res, _ := withCache(inner, cfg).Search(context.Background(), "q", 1); inner.calls != 2 || res[0].Cached {
		t.Errorf("changed settings must not reuse results made with the old ones, %d calls", inner.calls)
	}

	inner.empty = true
	withCache(inner, cfg).Search(context.Background(), "nothing", 1)
	withCache(inner, cfg).Search(context.Background(), "nothing", 1)
	if inner.calls != 4 {
		t.Errorf("empty pages should be retried, not cached, got %d calls", inner.calls)
	}

	c := &cached{Backend: inner, dir: config.ResultsCacheDir(), ttl: time.Millisecond}
	inner.empty = false
	c.Search(context.Background(), "old", 1)
	time.Sleep(5 * time.Millisecond)
	paths, _ := filepath.Glob(filepath.Join(c.dir, c.prefix(1)+"-*.json"))
	if len(paths) != 1 {
		t.Fatalf("want one entry, got %v", paths)
	}
	if _, ok := c.read(paths[0]); ok {
		t.Error("expired entry should not be served")
	}
	if _, err := os.Stat(paths[0]); !os.IsNotExist(err) {
		t.Error("expired entry should be deleted once seen")
	}
}

func TestPruneCacheRemovesOnlyExpired(t *testing.T) {
	dir := t.TempDir()
	old, fresh := filepath.Join(dir, "old.json"), filepath.Join(dir, "fresh.json")
	os.WriteFile(old, nil, 0o644)
	os.WriteFile(fresh, nil, 0o644)
	os.Chtimes(old, time.Now(), time.Now().Add(-time.Hour))

	pruneCache(dir, time.Minute)

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("expired entry should be removed")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("fresh entry should be kept")
	}
}

func TestCacheDisabledWhenTTLIsZero(t *testing.T) {
	t.Setenv("SIT_CACHE_DIR", t.TempDir())

	cfg := testConfig()
	cfg.General.CacheTTLSeconds = 0
	inner := &countingBackend{}
	b := withCache(inner, cfg)

	if _, err := b.Search(context.Background(), "q", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Search(context.Background(), "q", 1); err != nil {
		t.Fatal(err)
	}
	if inner.calls != 2 {
		t.Errorf("want no caching with ttl=0, got %d calls", inner.calls)
	}
}

func TestCacheSwitchedOff(t *testing.T) {
	t.Setenv("SIT_CACHE_DIR", t.TempDir())
	cfg := testConfig()
	cfg.General.CacheTTLSeconds = 300
	withCache(&countingBackend{}, cfg).Search(context.Background(), "q", 1)

	cfg.General.Cache = false
	inner := &countingBackend{}
	if b := withCache(inner, cfg); b != Backend(inner) {
		t.Error("cache = false should skip the cache wrapper")
	}
	CleanCache(cfg)
	if _, err := os.Stat(config.ResultsCacheDir()); !os.IsNotExist(err) {
		t.Error("saved results should be dropped when caching is off")
	}
}
