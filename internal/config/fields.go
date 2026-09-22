package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Kind int

const (
	KindString Kind = iota
	KindInt
	KindBool
	KindEnum
	KindList
)

// Field describes one editable setting, shared by the settings panel and `sit config set`.
type Field struct {
	Key     string
	Desc    string
	Kind    Kind
	Options []string
	Secret  bool
	get     func(*Config) string
	set     func(*Config, string) error
}

func (f Field) Get(c *Config) string { return f.get(c) }

func (f Field) Set(c *Config, v string) error {
	switch f.Kind {
	case KindInt:
		if _, err := strconv.Atoi(strings.TrimSpace(v)); err != nil {
			return fmt.Errorf("%s: want a number, got %q", f.Key, v)
		}
	case KindBool:
		if _, err := strconv.ParseBool(strings.TrimSpace(v)); err != nil {
			return fmt.Errorf("%s: want true or false, got %q", f.Key, v)
		}
	case KindEnum:
		if !contains(f.Options, strings.TrimSpace(v)) {
			return fmt.Errorf("%s: want one of %s, got %q", f.Key, strings.Join(f.Options, ", "), v)
		}
	}
	return f.set(c, strings.TrimSpace(v))
}

// Display masks secrets so keys never reach the screen or a log.
func (f Field) Display(c *Config) string {
	v := f.get(c)
	if f.Secret && v != "" {
		return strings.Repeat("•", 8) + " (set)"
	}
	if v == "" {
		return "-"
	}
	return v
}

func (f Field) Cycle(c *Config, dir int) error {
	switch f.Kind {
	case KindBool:
		b, _ := strconv.ParseBool(f.get(c))
		return f.set(c, strconv.FormatBool(!b))
	case KindEnum:
		cur := f.get(c)
		idx := 0
		for i, o := range f.Options {
			if o == cur {
				idx = i
			}
		}
		idx = (idx + dir + len(f.Options)) % len(f.Options)
		return f.set(c, f.Options[idx])
	}
	return nil
}

func str(get func(*Config) *string) (func(*Config) string, func(*Config, string) error) {
	return func(c *Config) string { return *get(c) },
		func(c *Config, v string) error { *get(c) = v; return nil }
}

func num(get func(*Config) *int) (func(*Config) string, func(*Config, string) error) {
	return func(c *Config) string { return strconv.Itoa(*get(c)) },
		func(c *Config, v string) error {
			n, err := strconv.Atoi(v)
			if err != nil {
				return err
			}
			*get(c) = n
			return nil
		}
}

func boolean(get func(*Config) *bool) (func(*Config) string, func(*Config, string) error) {
	return func(c *Config) string { return strconv.FormatBool(*get(c)) },
		func(c *Config, v string) error {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return err
			}
			*get(c) = b
			return nil
		}
}

func list(get func(*Config) *[]string) (func(*Config) string, func(*Config, string) error) {
	return func(c *Config) string { return strings.Join(*get(c), ", ") },
		func(c *Config, v string) error {
			var out []string
			for _, part := range strings.Split(v, ",") {
				if p := strings.TrimSpace(part); p != "" {
					out = append(out, p)
				}
			}
			*get(c) = out
			return nil
		}
}

func field(key, desc string, kind Kind, get func(*Config) string, set func(*Config, string) error, opts ...string) Field {
	return Field{Key: key, Desc: desc, Kind: kind, Options: opts, get: get, set: set}
}

// Fields returns every editable setting in display order.
func Fields() []Field {
	g1, s1 := str(func(c *Config) *string { return &c.General.Backend })
	g2, s2 := num(func(c *Config) *int { return &c.General.ResultsPerPage })
	g3, s3 := str(func(c *Config) *string { return &c.General.SafeSearch })
	g4, s4 := str(func(c *Config) *string { return &c.General.Region })
	g5, s5 := num(func(c *Config) *int { return &c.General.TimeoutSeconds })
	g6, s6 := str(func(c *Config) *string { return &c.General.OpenCommand })
	g7, s7 := num(func(c *Config) *int { return &c.General.CacheTTLSeconds })
	t1, u1 := str(func(c *Config) *string { return &c.Theme.Accent })
	t2, u2 := boolean(func(c *Config) *bool { return &c.Theme.Icons })
	t3, u3 := num(func(c *Config) *int { return &c.Theme.SnippetLines })
	t4, u4 := boolean(func(c *Config) *bool { return &c.Theme.ShowSource })
	b1, v1 := str(func(c *Config) *string { return &c.Backends.Brave.APIKey })
	b2, v2 := str(func(c *Config) *string { return &c.Backends.SearXNG.Instance })
	b3, v3 := list(func(c *Config) *[]string { return &c.Backends.SearXNG.Fallbacks })
	d1, w1 := str(func(c *Config) *string { return &c.Backends.Degoog.Instance })
	d2, w2 := str(func(c *Config) *string { return &c.Backends.Degoog.APIKey })
	d3, w3 := str(func(c *Config) *string { return &c.Backends.Degoog.Type })
	d4, w4 := list(func(c *Config) *[]string { return &c.Backends.Degoog.Engines })

	fields := []Field{
		field("general.backend", "Backend used on startup", KindEnum, g1, s1, "ddg", "degoog", "searxng", "brave"),
		field("general.results_per_page", "Results requested per page", KindInt, g2, s2),
		field("general.safe_search", "Safe search level", KindEnum, g3, s3, "off", "moderate", "strict"),
		field("general.region", "Region hint, e.g. us, de, pl (empty = auto)", KindString, g4, s4),
		field("general.timeout_seconds", "HTTP timeout per search", KindInt, g5, s5),
		field("general.open_command", "Override the browser opener command", KindString, g6, s6),
		field("general.cache_ttl_seconds", "How long search results stay cached, 0 disables", KindInt, g7, s7),
		field("theme.accent", "Accent colour (hex or ANSI index)", KindString, t1, u1),
		field("theme.icons", "Render favicons when the terminal supports it", KindBool, t2, u2),
		field("theme.snippet_lines", "Snippet lines per result", KindInt, t3, u3),
		field("theme.show_source", "Show the backend name on each result", KindBool, t4, u4),
		field("backends.degoog.instance", "Degoog instance URL (or $SIT_DEGOOG_URL)", KindString, d1, w1),
		field("backends.degoog.type", "Degoog search tab, e.g. web, images, news", KindString, d3, w3),
		field("backends.degoog.engines", "Degoog engine IDs to pin, comma separated (empty = instance default)", KindList, d4, w4),
		field("backends.searxng.instance", "SearXNG instance URL", KindString, b2, v2),
		field("backends.searxng.fallbacks", "Public SearXNG instances to try next, comma separated", KindList, b3, v3),
	}
	brave := field("backends.brave.api_key", "Brave Search API key (or $SIT_BRAVE_API_KEY)", KindString, b1, v1)
	brave.Secret = true
	degoogKey := field("backends.degoog.api_key", "Degoog API key when the instance is protected (or $SIT_DEGOOG_API_KEY)", KindString, d2, w2)
	degoogKey.Secret = true
	fields = append(fields, brave, degoogKey)

	for _, k := range keyFieldOrder() {
		fields = append(fields, k)
	}
	return fields
}

func keyFieldOrder() []Field {
	specs := []struct {
		key, desc string
		ptr       func(*Config) *string
	}{
		{"keys.focus", "Focus the search input", func(c *Config) *string { return &c.Keys.Focus }},
		{"keys.next_backend", "Cycle to the next backend", func(c *Config) *string { return &c.Keys.NextBackend }},
		{"keys.prev_backend", "Cycle to the previous backend", func(c *Config) *string { return &c.Keys.PrevBackend }},
		{"keys.open", "Open the selected result", func(c *Config) *string { return &c.Keys.Open }},
		{"keys.copy", "Copy the selected URL", func(c *Config) *string { return &c.Keys.Copy }},
		{"keys.next_page", "Fetch the next page", func(c *Config) *string { return &c.Keys.NextPage }},
		{"keys.prev_page", "Go to the previous page", func(c *Config) *string { return &c.Keys.PrevPage }},
		{"keys.up", "Move selection up", func(c *Config) *string { return &c.Keys.Up }},
		{"keys.down", "Move selection down", func(c *Config) *string { return &c.Keys.Down }},
		{"keys.help", "Toggle help", func(c *Config) *string { return &c.Keys.Help }},
		{"keys.settings", "Toggle settings", func(c *Config) *string { return &c.Keys.Settings }},
		{"keys.quit", "Quit", func(c *Config) *string { return &c.Keys.Quit }},
	}
	out := make([]Field, 0, len(specs))
	for _, s := range specs {
		g, st := str(s.ptr)
		out = append(out, field(s.key, s.desc, KindString, g, st))
	}
	return out
}

func FieldByKey(key string) (Field, bool) {
	for _, f := range Fields() {
		if f.Key == key {
			return f, true
		}
	}
	return Field{}, false
}

func FieldKeys() []string {
	var keys []string
	for _, f := range Fields() {
		keys = append(keys, f.Key)
	}
	sort.Strings(keys)
	return keys
}

func (c *Config) SetValue(key, value string) error {
	f, ok := FieldByKey(key)
	if !ok {
		return fmt.Errorf("unknown setting %q (see `sit config keys`)", key)
	}
	return f.Set(c, value)
}

func (c *Config) GetValue(key string) (string, error) {
	f, ok := FieldByKey(key)
	if !ok {
		return "", fmt.Errorf("unknown setting %q (see `sit config keys`)", key)
	}
	return f.Get(c), nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
