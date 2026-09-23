// Package config loads and persists sit's TOML configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

const AppName = "sit"

type Config struct {
	General  General  `toml:"general"`
	Theme    Theme    `toml:"theme"`
	Keys     Keys     `toml:"keys"`
	Backends Backends `toml:"backends"`
}

type General struct {
	Backend         string `toml:"backend"`
	ResultsPerPage  int    `toml:"results_per_page"`
	SafeSearch      string `toml:"safe_search"`
	Region          string `toml:"region"`
	TimeoutSeconds  int    `toml:"timeout_seconds"`
	OpenCommand     string `toml:"open_command"`
	Cache           bool   `toml:"cache"`
	CacheTTLSeconds int    `toml:"cache_ttl_seconds"`
}

type Theme struct {
	Accent       string `toml:"accent"`
	Icons        bool   `toml:"icons"`
	IconBackdrop bool   `toml:"icon_backdrop"`
	SnippetLines int    `toml:"snippet_lines"`
	ShowSource   bool   `toml:"show_source"`
}

type Keys struct {
	Quit        string `toml:"quit"`
	Help        string `toml:"help"`
	Settings    string `toml:"settings"`
	Focus       string `toml:"focus"`
	NextBackend string `toml:"next_backend"`
	PrevBackend string `toml:"prev_backend"`
	Open        string `toml:"open"`
	Copy        string `toml:"copy"`
	NextPage    string `toml:"next_page"`
	PrevPage    string `toml:"prev_page"`
	Up          string `toml:"up"`
	Down        string `toml:"down"`
}

type Backends struct {
	Brave   Brave   `toml:"brave"`
	SearXNG SearXNG `toml:"searxng"`
	Degoog  Degoog  `toml:"degoog"`
}

type Brave struct {
	APIKey string `toml:"api_key"`
}

type SearXNG struct {
	Instance  string   `toml:"instance"`
	Fallbacks []string `toml:"fallbacks"`
}

type Degoog struct {
	Instance string   `toml:"instance"`
	APIKey   string   `toml:"api_key"`
	Type     string   `toml:"type"`
	Engines  []string `toml:"engines"`
}

// DefaultSearxFallbacks are public SearXNG instances tried when the configured one fails.
var DefaultSearxFallbacks = []string{
	"https://opnxng.com",
	"https://priv.au",
	"https://search.rhscz.eu",
	"https://searx.tiekoetter.com",
	"https://paulgo.io",
}

func Default() Config {
	return Config{
		General: General{
			Backend:         "ddg",
			ResultsPerPage:  20,
			SafeSearch:      "moderate",
			Region:          "",
			TimeoutSeconds:  12,
			Cache:           true,
			CacheTTLSeconds: 1800,
		},
		Theme: Theme{
			Accent:       "#5f9ea0",
			Icons:        true,
			IconBackdrop: true,
			SnippetLines: 2,
			ShowSource:   false,
		},
		Keys: Keys{
			Quit:        "q",
			Help:        "?",
			Settings:    ",",
			Focus:       "/",
			NextBackend: "tab",
			PrevBackend: "shift+tab",
			Open:        "enter",
			Copy:        "y",
			NextPage:    "n",
			PrevPage:    "p",
			Up:          "k",
			Down:        "j",
		},
		Backends: Backends{
			SearXNG: SearXNG{Instance: "http://localhost:8080", Fallbacks: DefaultSearxFallbacks},
			Degoog:  Degoog{Instance: "http://localhost:4444", Type: "web"},
		},
	}
}

func (c Config) Timeout() time.Duration {
	if c.General.TimeoutSeconds <= 0 {
		return 12 * time.Second
	}
	return time.Duration(c.General.TimeoutSeconds) * time.Second
}

// CacheTTL is how long a search result page stays cached; 0 means caching is off.
func (c Config) CacheTTL() time.Duration {
	if !c.General.Cache || c.General.CacheTTLSeconds <= 0 {
		return 0
	}
	return time.Duration(c.General.CacheTTLSeconds) * time.Second
}

// BraveKey prefers the environment so keys need not be written to disk.
func (c Config) BraveKey() string {
	return envOr(c.Backends.Brave.APIKey, "SIT_BRAVE_API_KEY", "BRAVE_API_KEY")
}

func (c Config) DegoogKey() string {
	return envOr(c.Backends.Degoog.APIKey, "SIT_DEGOOG_API_KEY", "DEGOOG_API_KEY")
}

func (c Config) DegoogInstance() string {
	return envOr(c.Backends.Degoog.Instance, "SIT_DEGOOG_URL", "DEGOOG_URL")
}

func envOr(fallback string, envs ...string) string {
	for _, env := range envs {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return fallback
}

func Dir() string {
	if v := os.Getenv("SIT_CONFIG_DIR"); v != "" {
		return v
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, AppName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", AppName)
	}
	return filepath.Join(home, ".config", AppName)
}

func Path() string { return filepath.Join(Dir(), "config.toml") }

func CacheDir() string {
	if v := os.Getenv("SIT_CACHE_DIR"); v != "" {
		return v
	}
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, AppName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".cache", AppName)
	}
	return filepath.Join(home, ".cache", AppName)
}

func FaviconDir() string { return filepath.Join(CacheDir(), "favicons") }

func ResultsCacheDir() string { return filepath.Join(CacheDir(), "results") }

// Load reads the config file, falling back to defaults when it does not exist.
func Load() (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("%s: %w", Path(), err)
	}
	cfg.normalize()
	return cfg, nil
}

func (c *Config) normalize() {
	d := Default()
	if c.General.Backend == "" {
		c.General.Backend = d.General.Backend
	}
	if c.General.ResultsPerPage <= 0 {
		c.General.ResultsPerPage = d.General.ResultsPerPage
	}
	if c.General.SafeSearch == "" {
		c.General.SafeSearch = d.General.SafeSearch
	}
	if c.Theme.Accent == "" {
		c.Theme.Accent = d.Theme.Accent
	}
	if c.Theme.SnippetLines <= 0 {
		c.Theme.SnippetLines = d.Theme.SnippetLines
	}
	if c.Backends.SearXNG.Instance == "" {
		c.Backends.SearXNG.Instance = d.Backends.SearXNG.Instance
	}
	if c.Backends.Degoog.Type == "" {
		c.Backends.Degoog.Type = d.Backends.Degoog.Type
	}
	k, dk := &c.Keys, d.Keys
	for _, pair := range [][2]*string{
		{&k.Quit, &dk.Quit}, {&k.Help, &dk.Help}, {&k.Settings, &dk.Settings},
		{&k.Focus, &dk.Focus}, {&k.NextBackend, &dk.NextBackend}, {&k.PrevBackend, &dk.PrevBackend},
		{&k.Open, &dk.Open}, {&k.Copy, &dk.Copy}, {&k.NextPage, &dk.NextPage},
		{&k.PrevPage, &dk.PrevPage}, {&k.Up, &dk.Up}, {&k.Down, &dk.Down},
	} {
		if *pair[0] == "" {
			*pair[0] = *pair[1]
		}
	}
}

// Save writes the config atomically, creating the directory when missing.
func (c Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(Dir(), ".config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	enc := toml.NewEncoder(tmp)
	if err := enc.Encode(c); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), Path())
}

func Exists() bool {
	_, err := os.Stat(Path())
	return err == nil
}
