package backend

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fake struct {
	name  string
	res   []Result
	err   error
	ready bool
}

func (f fake) Name() string          { return f.name }
func (f fake) Label() string         { return f.name }
func (f fake) Ready() (bool, string) { return f.ready, "not set up" }
func (f fake) Search(context.Context, string, int) ([]Result, error) {
	return f.res, f.err
}

func hit(url, source, snippet string) Result {
	return Result{Title: url, URL: url, Source: source, Snippet: snippet}
}

func TestFanoutInterleavesAndMerges(t *testing.T) {
	f := &fanout{engines: []Backend{
		fake{name: "a", ready: true, res: []Result{hit("https://go.dev/", "a", ""), hit("https://a.example/", "a", "")}},
		fake{name: "b", ready: true, res: []Result{hit("http://www.go.dev", "b", "from b"), hit("https://b.example/", "b", ""), hit("https://b2.example/", "b", "")}},
		fake{name: "c", ready: false, err: errors.New("never called")},
	}}
	got, err := f.Search(context.Background(), "go", 1)
	if err != nil {
		t.Fatal(err)
	}
	var urls []string
	for _, r := range got {
		urls = append(urls, r.URL)
	}
	if strings.Join(urls, " ") != "https://go.dev/ https://a.example/ https://b.example/ https://b2.example/" {
		t.Errorf("order = %v", urls)
	}
	if got[0].Source != "a+b" || got[0].Snippet != "from b" {
		t.Errorf("merged = %+v", got[0])
	}
}

func TestMergeRanksByFusedScore(t *testing.T) {
	a := []Result{hit("https://a1/", "a", ""), hit("https://shared/", "a", ""), hit("https://a3/", "a", "")}
	b := []Result{hit("https://b1/", "b", ""), hit("https://b2/", "b", ""), hit("https://shared/?utm_source=x", "b", "")}
	urls := func(rs []Result) string {
		var out []string
		for _, r := range rs {
			out = append(out, r.URL)
		}
		return strings.Join(out, " ")
	}
	// Found by both engines, the shared page outranks every single-engine hit.
	if got := urls(merge([][]Result{a, b}, nil)); got != "https://shared/ https://a1/ https://b1/ https://b2/ https://a3/" {
		t.Errorf("unweighted = %s", got)
	}
	if got := urls(merge([][]Result{a, b}, []float64{1, 3})); got != "https://shared/ https://b1/ https://b2/ https://a1/ https://a3/" {
		t.Errorf("weighted = %s", got)
	}
}

func TestURLKey(t *testing.T) {
	same := [][2]string{
		{"https://www.go.dev/doc/", "http://go.dev/doc#intro"},
		{"https://x.io/p?b=2&a=1", "https://x.io/p?a=1&b=2&utm_medium=feed&fbclid=z"},
	}
	for _, p := range same {
		if URLKey(p[0]) != URLKey(p[1]) {
			t.Errorf("%s and %s should match: %s vs %s", p[0], p[1], URLKey(p[0]), URLKey(p[1]))
		}
	}
	if URLKey("https://x.io/p?id=1") == URLKey("https://x.io/p?id=2") {
		t.Error("real parameters must stay")
	}
}

func TestFanoutKeepsResultsWhenOneEngineFails(t *testing.T) {
	f := &fanout{engines: []Backend{
		fake{name: "a", ready: true, err: errors.New("a: rate limited")},
		fake{name: "b", ready: true, res: []Result{hit("https://b.example/", "b", "")}},
	}}
	if got, err := f.Search(context.Background(), "q", 1); err != nil || len(got) != 1 {
		t.Errorf("got %+v, %v", got, err)
	}
	f.engines[1] = fake{name: "b", ready: true, err: errors.New("b: http 500")}
	if _, err := f.Search(context.Background(), "q", 1); err == nil || !strings.Contains(err.Error(), "a: rate limited") || !strings.Contains(err.Error(), "b: http 500") {
		t.Errorf("want both errors, got %v", err)
	}
}

func TestNewFanoutFromConfig(t *testing.T) {
	cfg := testConfig()
	b, err := New("fanout", cfg)
	if err != nil || b.Label() != "ddg+brave" {
		t.Fatalf("fanout = %v, %v", b, err)
	}
	cfg.Backends.Fanout.Engines = []string{"ddg", "fanout"}
	if _, err := New("fanout", cfg); err == nil {
		t.Error("fanout must not nest itself")
	}
	cfg.Backends.Fanout.Engines = nil
	b, _ = New("fanout", cfg)
	if ok, _ := b.Ready(); ok {
		t.Error("an empty fanout is not ready")
	}
}
