package config

import (
	"os"
	"strings"
	"testing"
)

func TestNoBangsByDefault(t *testing.T) {
	cfg := Default()
	if _, ok := cfg.Bang("!w rust"); ok || len(cfg.BangFields()) != 0 {
		t.Error("bangs must be configured by the user, none ship built in")
	}
}

func TestBang(t *testing.T) {
	cfg := Default()
	cfg.Bangs = map[string]string{"gh": "https://github.com/search?q=%s", "w": "https://en.wikipedia.org/wiki/%s"}
	for query, want := range map[string]string{
		"!gh bubble tea":     "https://github.com/search?q=bubble%20tea",
		"c++ & go !GH":       "https://github.com/search?q=c%2B%2B%20%26%20go",
		"!w Rust (language)": "https://en.wikipedia.org/wiki/Rust%20%28language%29",
		"!gh":                "https://github.com/search?q=",
	} {
		if got, ok := cfg.Bang(query); !ok || got != want {
			t.Errorf("Bang(%q) = %q, %v; want %q", query, got, ok, want)
		}
	}
	for _, query := range []string{"!nope rust", "gh rust", "rust!gh"} {
		if got, ok := cfg.Bang(query); ok {
			t.Errorf("Bang(%q) = %q, want no bang", query, got)
		}
	}
}

func TestBangRows(t *testing.T) {
	cfg := Default()
	if err := AddBang().Set(&cfg, "!GH = https://github.com/search?q=%s"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetValue("bangs.w", "https://en.wikipedia.org/wiki/%s"); err != nil {
		t.Fatal(err)
	}
	rows := cfg.BangFields()
	if len(rows) != 2 || rows[0].Key != "bangs.!gh" || rows[1].Key != "bangs.!w" || rows[0].Get(&cfg) != "https://github.com/search?q=%s" {
		t.Fatalf("rows = %+v", rows)
	}
	if err := rows[1].Set(&cfg, ""); err != nil || len(cfg.Bangs) != 1 {
		t.Errorf("clearing a row must remove its bang: %v, %v", err, cfg.Bangs)
	}
	if v, err := cfg.GetValue("bangs.!gh"); err != nil || v != "https://github.com/search?q=%s" {
		t.Errorf("get = %q, %v", v, err)
	}
	if _, err := cfg.GetValue("bangs.nope"); err == nil {
		t.Error("want an error for a missing bang")
	}
}

func TestBangValidation(t *testing.T) {
	cfg := Default()
	for _, bad := range []string{"gh", "=https://x", "gh=github.com", "gh=", "two words=https://x"} {
		if err := AddBang().Set(&cfg, bad); err == nil {
			t.Errorf("want an error for %q", bad)
		}
	}
	if len(cfg.Bangs) != 0 {
		t.Errorf("bangs = %v", cfg.Bangs)
	}
}

func TestSetBangLeavesOriginalMapAlone(t *testing.T) {
	live := Default()
	live.Bangs = map[string]string{"gh": "https://github.com/search?q=%s"}
	draft := live
	draft.SetValue("bangs.gh", "")
	draft.SetValue("bangs.w", "https://en.wikipedia.org/wiki/%s")
	if len(live.Bangs) != 1 || live.Bangs["gh"] == "" {
		t.Errorf("unsaved edits leaked into the live config: %v", live.Bangs)
	}
}

func TestBangsRoundTrip(t *testing.T) {
	t.Setenv("SIT_CONFIG_DIR", t.TempDir())
	cfg := Default()
	cfg.Bangs = map[string]string{"gh": "https://github.com/search?q=%s"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(Path())
	if !strings.Contains(string(data), "\n[bangs]\n") {
		t.Errorf("bangs should be their own table:\n%s", data)
	}
	loaded, err := Load()
	if err != nil || loaded.Bangs["gh"] != "https://github.com/search?q=%s" {
		t.Errorf("bangs = %v, %v", loaded.Bangs, err)
	}
}
