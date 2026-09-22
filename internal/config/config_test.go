package config

import (
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIT_CONFIG_DIR", dir)

	cfg := Default()
	cfg.General.Backend = "degoog"
	cfg.Theme.Accent = "#ff8800"
	cfg.Backends.Degoog.Engines = []string{"a", "b"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if Path() != filepath.Join(dir, "config.toml") {
		t.Fatalf("path = %s", Path())
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.General.Backend != "degoog" || loaded.Theme.Accent != "#ff8800" {
		t.Errorf("round trip lost values: %+v", loaded.General)
	}
	if len(loaded.Backends.Degoog.Engines) != 2 {
		t.Errorf("engines = %v", loaded.Backends.Degoog.Engines)
	}
}

func TestLoadWithoutFileUsesDefaults(t *testing.T) {
	t.Setenv("SIT_CONFIG_DIR", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.General.Backend != Default().General.Backend {
		t.Errorf("backend = %q", cfg.General.Backend)
	}
}

func TestSetValue(t *testing.T) {
	cfg := Default()
	if err := cfg.SetValue("general.results_per_page", "5"); err != nil {
		t.Fatal(err)
	}
	if cfg.General.ResultsPerPage != 5 {
		t.Errorf("results_per_page = %d", cfg.General.ResultsPerPage)
	}
	if err := cfg.SetValue("general.safe_search", "banana"); err == nil {
		t.Error("want an error for an invalid enum value")
	}
	if err := cfg.SetValue("theme.icons", "nope"); err == nil {
		t.Error("want an error for an invalid bool")
	}
	if err := cfg.SetValue("nope.nope", "x"); err == nil {
		t.Error("want an error for an unknown key")
	}
}

func TestSecretFieldsAreMasked(t *testing.T) {
	cfg := Default()
	cfg.Backends.Brave.APIKey = "super-secret"
	for _, f := range Fields() {
		if f.Key != "backends.brave.api_key" {
			continue
		}
		if got := f.Display(&cfg); got == "super-secret" {
			t.Error("api key must not be displayed in clear text")
		}
	}
}

func TestEnvOverridesKeys(t *testing.T) {
	t.Setenv("SIT_BRAVE_API_KEY", "from-env")
	t.Setenv("SIT_DEGOOG_URL", "http://degoog.local:4444")
	cfg := Default()
	cfg.Backends.Brave.APIKey = "from-file"
	if cfg.BraveKey() != "from-env" {
		t.Errorf("brave key = %q", cfg.BraveKey())
	}
	if cfg.DegoogInstance() != "http://degoog.local:4444" {
		t.Errorf("degoog instance = %q", cfg.DegoogInstance())
	}
}
