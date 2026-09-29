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
