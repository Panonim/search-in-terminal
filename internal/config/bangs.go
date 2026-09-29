package config

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
)

// Bang turns a query holding a configured !bang into that site's search URL.
func (c Config) Bang(query string) (string, bool) {
	words := strings.Fields(query)
	for i, w := range words {
		name, isBang := strings.CutPrefix(w, "!")
		target, ok := c.Bangs[strings.ToLower(name)]
		if !isBang || !ok {
			continue
		}
		rest := strings.Join(slices.Delete(words, i, i+1), " ")
		// %20 rather than + keeps spaces right in URL paths as well as query strings.
		return strings.ReplaceAll(target, "%s", strings.ReplaceAll(url.QueryEscape(rest), "+", "%20")), true
	}
	return "", false
}

// BangFields gives each bang its own editable row, in name order.
func (c Config) BangFields() []Field {
	var out []Field
	for _, name := range slices.Sorted(maps.Keys(c.Bangs)) {
		out = append(out, field("bangs.!"+name, "Search URL for !"+name+", %s marks the query; clear it to remove the bang", KindString,
			func(c *Config) string { return c.Bangs[name] },
			func(c *Config, v string) error { return c.setBang(name, v) }))
	}
	return out
}

// AddBang is the row that creates a bang from name=url.
func AddBang() Field {
	return field("bangs.+ add", "New bang as name=url, e.g. gh=https://github.com/search?q=%s", KindString,
		func(*Config) string { return "" },
		func(c *Config, v string) error {
			name, target, ok := strings.Cut(v, "=")
			if !ok || strings.TrimSpace(target) == "" {
				return fmt.Errorf("want name=url, got %q", v)
			}
			return c.setBang(name, target)
		})
}

// setBang copies the map before changing it, so an unsaved settings panel never edits the live config.
func (c *Config) setBang(name, target string) error {
	name, target = bangName(name), strings.TrimSpace(target)
	if name == "" || strings.ContainsAny(name, " =") {
		return fmt.Errorf("bang name %q: want a single word", name)
	}
	bangs := maps.Clone(c.Bangs)
	if bangs == nil {
		bangs = map[string]string{}
	}
	switch {
	case target == "":
		delete(bangs, name)
	case strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://"):
		bangs[name] = target
	default:
		return fmt.Errorf("!%s: want an http(s) URL with %%s for the query, got %q", name, target)
	}
	c.Bangs = bangs
	return nil
}

func bangName(name string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), "!"))
}
