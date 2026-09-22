// Command sit is a keyboard-driven web search browser for the terminal.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Panonim/search-in-terminal/internal/backend"
	"github.com/Panonim/search-in-terminal/internal/config"
	img "github.com/Panonim/search-in-terminal/internal/image"
	"github.com/Panonim/search-in-terminal/internal/open"
	"github.com/Panonim/search-in-terminal/internal/ui"
	"github.com/Panonim/search-in-terminal/internal/update"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "sit: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sit: "+err.Error()+" (using defaults)")
	}
	go backend.CleanCache(cfg)

	if len(args) == 0 {
		return ui.Run(cfg, version, "")
	}
	switch args[0] {
	case "help", "--help", "-h":
		usage()
		return nil
	case "version", "--version", "-v":
		fmt.Printf("sit %s (%s, %s, %s/%s)\n", version, commit, date, runtime.GOOS, runtime.GOARCH)
		return nil
	case "search":
		return cmdSearch(cfg, args[1:])
	case "open":
		return cmdOpen(cfg, args[1:])
	case "suggest":
		return cmdSuggest(cfg, args[1:])
	case "config":
		return cmdConfig(cfg, args[1:])
	case "backends":
		return cmdBackends(cfg)
	case "update":
		return cmdUpdate(args[1:])
	case "cache":
		return cmdCache(args[1:])
	case "doctor":
		return cmdDoctor(cfg)
	}
	if strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("unknown flag %q, see `sit help`", args[0])
	}
	return ui.Run(cfg, version, strings.Join(args, " "))
}

func usage() {
	fmt.Print(`sit - search the web without leaving your terminal

usage:
  sit                       open the search UI
  sit <query…>              open the UI with a query already running
  sit search <query…>       print results without the UI
  sit open <query…>         open the first result in your browser
  sit suggest <query…>      print autocomplete suggestions (Degoog instances)
  sit backends              list backends and whether they are ready
  sit config <subcommand>   inspect and change settings
  sit update [--check]      update sit to the latest release
  sit cache [info|clear]    inspect or drop the favicon and search result cache
  sit doctor                check config, network and terminal support
  sit version               print version information

search flags:
  -b, --backend <name>      ddg | degoog | searxng | brave
  -n, --limit <count>       maximum results to print
  -p, --page <number>       result page
      --json                print JSON instead of text

config subcommands:
  sit config show           print the effective configuration
  sit config path           print the config file path
  sit config init           write a config file with the current settings
  sit config edit           open the config file in $EDITOR
  sit config keys           list every setting key
  sit config get <key>      print one setting
  sit config set <key> <v>  change one setting and save
`)
}

func cmdSearch(cfg config.Config, args []string) error {
	var (
		name    = cfg.General.Backend
		limit   = cfg.General.ResultsPerPage
		page    = 1
		asJSON  bool
		queryIn []string
	)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-b", "--backend":
			if i++; i >= len(args) {
				return fmt.Errorf("--backend needs a value")
			}
			name = args[i]
		case "-n", "--limit":
			if i++; i >= len(args) {
				return fmt.Errorf("--limit needs a value")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return err
			}
			limit = n
		case "-p", "--page":
			if i++; i >= len(args) {
				return fmt.Errorf("--page needs a value")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return err
			}
			page = n
		case "--json":
			asJSON = true
		default:
			queryIn = append(queryIn, args[i])
		}
	}
	query := strings.Join(queryIn, " ")
	if query == "" {
		return fmt.Errorf("usage: sit search <query>")
	}

	results, err := searchOnce(cfg, name, query, page, limit)
	if err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}
	if len(results) == 0 {
		fmt.Printf("no results for %q\n", query)
		return nil
	}
	for i, r := range results {
		fmt.Printf("%2d. %s\n    %s\n", i+1, r.Title, r.URL)
		if r.Snippet != "" {
			fmt.Printf("    %s\n", r.Snippet)
		}
		fmt.Println()
	}
	return nil
}

func cmdOpen(cfg config.Config, args []string) error {
	query := strings.Join(args, " ")
	if query == "" {
		return fmt.Errorf("usage: sit open <query>")
	}
	results, err := searchOnce(cfg, cfg.General.Backend, query, 1, 1)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("no results for %q", query)
	}
	fmt.Println(results[0].URL)
	return open.URL(results[0].URL, cfg.General.OpenCommand)
}

func cmdSuggest(cfg config.Config, args []string) error {
	query := strings.Join(args, " ")
	if query == "" {
		return fmt.Errorf("usage: sit suggest <query>")
	}
	b, err := backend.New(cfg.General.Backend, cfg)
	if err != nil {
		return err
	}
	s, ok := b.(backend.Suggester)
	if !ok {
		return fmt.Errorf("backend %s has no suggestion API, try `sit config set general.backend degoog`", b.Name())
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout())
	defer cancel()
	for _, item := range s.Suggest(ctx, query) {
		fmt.Println(item)
	}
	return nil
}

func searchOnce(cfg config.Config, name, query string, page, limit int) ([]backend.Result, error) {
	b, err := backend.New(name, cfg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout())
	defer cancel()
	results, err := b.Search(ctx, query, page)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func cmdConfig(cfg config.Config, args []string) error {
	if len(args) == 0 {
		args = []string{"show"}
	}
	switch args[0] {
	case "path":
		fmt.Println(config.Path())
		return nil
	case "show":
		for _, f := range config.Fields() {
			fmt.Printf("%-30s %s\n", f.Key, f.Display(&cfg))
		}
		return nil
	case "keys":
		for _, k := range config.FieldKeys() {
			fmt.Println(k)
		}
		return nil
	case "init":
		if config.Exists() {
			fmt.Printf("%s already exists\n", config.Path())
			return nil
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", config.Path())
		return nil
	case "edit":
		if !config.Exists() {
			if err := cfg.Save(); err != nil {
				return err
			}
		}
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		parts := strings.Fields(editor)
		cmd := exec.Command(parts[0], append(parts[1:], config.Path())...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("usage: sit config get <key>")
		}
		v, err := cfg.GetValue(args[1])
		if err != nil {
			return err
		}
		fmt.Println(v)
		return nil
	case "set":
		if len(args) < 3 {
			return fmt.Errorf("usage: sit config set <key> <value>")
		}
		if err := cfg.SetValue(args[1], strings.Join(args[2:], " ")); err != nil {
			return err
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Printf("%s = %s\n", args[1], mustGet(cfg, args[1]))
		return nil
	}
	return fmt.Errorf("unknown config subcommand %q", args[0])
}

func mustGet(cfg config.Config, key string) string {
	v, _ := cfg.GetValue(key)
	return v
}

func cmdBackends(cfg config.Config) error {
	for _, b := range backend.All(cfg) {
		status := "ready"
		if ok, why := b.Ready(); !ok {
			status = why
		}
		marker := " "
		if b.Name() == cfg.General.Backend {
			marker = "*"
		}
		fmt.Printf("%s %-10s %-22s %s\n", marker, b.Name(), b.Label(), status)
	}
	return nil
}

func cmdUpdate(args []string) error {
	checkOnly := len(args) > 0 && (args[0] == "--check" || args[0] == "-c")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	rel, err := update.Latest(ctx)
	if err != nil {
		return err
	}
	if !update.NewerThan(rel.Version(), version) {
		fmt.Printf("sit %s is already the latest release\n", version)
		return nil
	}
	fmt.Printf("%s → %s  (%s)\n", version, rel.Version(), rel.HTMLURL)
	if checkOnly {
		return nil
	}
	if err := update.Apply(ctx, rel, os.Stdout); err != nil {
		return err
	}
	fmt.Printf("updated to %s\n", rel.Version())
	return nil
}

func cmdCache(args []string) error {
	dir := config.CacheDir()
	action := "info"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "info":
		var files int
		var bytes int64
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			files++
			if info, err := d.Info(); err == nil {
				bytes += info.Size()
			}
			return nil
		})
		fmt.Printf("%s\n%d files, %.1f KiB\n", dir, files, float64(bytes)/1024)
		return nil
	case "clear":
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		fmt.Printf("cleared %s\n", dir)
		return nil
	}
	return fmt.Errorf("usage: sit cache [info|clear]")
}

func cmdDoctor(cfg config.Config) error {
	fmt.Printf("sit %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	fmt.Printf("config     %s (%s)\n", config.Path(), existsLabel(config.Exists()))
	fmt.Printf("cache      %s\n", config.CacheDir())
	fmt.Printf("graphics   %s\n", img.Detect())
	fmt.Printf("degoog     %s\n", cfg.DegoogInstance())
	fmt.Printf("terminal   TERM=%s TERM_PROGRAM=%s\n", os.Getenv("TERM"), os.Getenv("TERM_PROGRAM"))
	fmt.Println()

	for _, b := range backend.All(cfg) {
		ok, why := b.Ready()
		if !ok {
			fmt.Printf("%-10s skipped: %s\n", b.Name(), why)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout())
		start := time.Now()
		res, err := b.Search(ctx, "hello world", 1)
		cancel()
		switch {
		case err != nil:
			fmt.Printf("%-10s error: %s\n", b.Name(), err)
		default:
			noun := "results"
			if len(res) == 1 {
				noun = "result"
			}
			fmt.Printf("%-10s %d %s in %s\n", b.Name(), len(res), noun, time.Since(start).Round(time.Millisecond))
		}
	}
	return nil
}

func existsLabel(ok bool) string {
	if ok {
		return "found"
	}
	return "using defaults"
}
