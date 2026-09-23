# Configuration

The config file lives at `~/.config/sit/config.toml` (or `$XDG_CONFIG_HOME/sit/config.toml`; override
the whole directory with `$SIT_CONFIG_DIR`). It is optional - sit runs on defaults until you write one
with `sit config init`. Favicons and search results are cached under `~/.cache/sit/` (`$XDG_CACHE_HOME`, or `$SIT_CACHE_DIR`).

```toml
[general]
# Backend used on startup: ddg | degoog | searxng | brave
backend = "ddg"
# Results requested per page.
results_per_page = 20
# Safe search level: off | moderate | strict
safe_search = "moderate"
# Region/language hint, e.g. "us", "de", "pl". Empty means let the backend decide.
region = ""
# HTTP timeout for a single search, in seconds.
timeout_seconds = 12
# Override the browser opener. Empty uses xdg-open / open / rundll32.
# The URL is appended as the last argument, e.g. "firefox --new-tab".
open_command = ""
# Cache search results and favicons on disk. Turning it off also clears saved results on the next start.
cache = true
# How long a cached result page is reused, in seconds.
cache_ttl_seconds = 1800

[theme]
# Accent colour, hex or an ANSI palette index.
accent = "#5f9ea0"
# Draw favicons when the terminal supports a graphics protocol.
icons = true
# Put a soft circle behind favicons that would blend into the terminal background.
icon_backdrop = true
# Snippet lines shown under each result.
snippet_lines = 2
# Show which engine produced each result.
show_source = false

[keys]
focus = "/"
next_backend = "tab"
prev_backend = "shift+tab"
open = "enter"
copy = "y"
next_page = "n"
prev_page = "p"
up = "k"
down = "j"
help = "?"
settings = ","
quit = "q"

[backends.brave]
# Prefer the environment; $SIT_BRAVE_API_KEY and $BRAVE_API_KEY both win over this value.
api_key = ""

[backends.degoog]
# Your Degoog instance; $SIT_DEGOOG_URL and $DEGOOG_URL win over this value.
instance = "http://localhost:4444"
# Only needed when the instance is password protected; $SIT_DEGOOG_API_KEY and $DEGOOG_API_KEY win.
api_key = ""
# Search tab to query: web, images, news, … (see /api/search-tabs on your instance).
type = "web"
# Pin specific engine extension IDs; empty means the instance decides.
engines = []

[backends.searxng]
# Your own instance, or any public one with the JSON API enabled.
instance = "http://localhost:8080"
# Tried in order when the instance above fails or returns nothing.
fallbacks = [
  "https://opnxng.com",
  "https://priv.au",
  "https://search.rhscz.eu",
  "https://searx.tiekoetter.com",
  "https://paulgo.io",
]
```

Everything here is also reachable from the in-app settings panel (<kbd>,</kbd>) and from
`sit config set`, e.g. `sit config set general.backend searxng`. Run `sit config keys` for the full list.

## Degoog

[Degoog](https://github.com/degoog-org/degoog) is a self-hosted search aggregator: it queries every
engine you have enabled, merges and scores the results, and serves them over a JSON API. sit talks to
that API directly, so the `degoog` backend gives you your own engine mix, your own filters and no third
party in the middle.

Run an instance:

```sh
docker run -d --name degoog \
  -p 4444:4444 \
  -v ./data:/app/data \
  -e DEGOOG_SETTINGS_PASSWORDS=change-me \
  ghcr.io/degoog-org/degoog:latest
```

Point sit at it:

```sh
sit config set backends.degoog.instance http://localhost:4444
sit config set general.backend degoog
```

What the integration uses:

- `GET /api/search` for results, `POST /api/search` when you pin engines with
  `sit config set backends.degoog.engines "degoog-org-official-extensions-brave-engine"`.
- `backends.degoog.type` selects the search tab (`web`, `images`, `news`, … - your instance lists them at
  `/api/search-tabs`).
- `GET /api/suggest` for autocomplete: suggestions appear as you type and are accepted with
  <kbd>Ctrl+e</kbd> or <kbd>→</kbd>. The same endpoint backs `sit suggest <query>`.
- `Authorization: Bearer …` when the instance is protected with `DEGOOG_SETTINGS_PASSWORDS`. Set the key
  in the environment so it stays off disk:

```sh
export SIT_DEGOOG_API_KEY="…"   # or DEGOOG_API_KEY
```

`$SIT_DEGOOG_URL` (or `$DEGOOG_URL`) overrides the instance the same way, which is handy for switching
between a laptop tunnel and a LAN address. A protected instance answers `401`, and sit says so rather
than silently returning nothing.

Degoog can also serve SearXNG-shaped JSON ("Serve the SearXNG API shape" in its settings). If you turn
that on, the `searxng` backend works against it too - but the native `degoog` backend gives you scoring,
merged sources and suggestions, so prefer it.

## SearXNG

The `searxng` backend talks to a single instance of your choosing. It defaults to `http://localhost:8080`,
the standard local address for a self-hosted instance; point it elsewhere with:

```sh
sit config set backends.searxng.instance https://searx.example.org
sit config set general.backend searxng
```

sit uses the instance's **JSON API** (`/search?format=json`), not the HTML page. That API is disabled by
default in SearXNG, so a self-hosted instance needs this in its `settings.yml`:

```yaml
search:
  formats:
    - html
    - json
```

Older releases use `search.default_format` alongside the format list; keeping `json` in `formats` is the
part that matters. Restart SearXNG afterwards and check it with:

```sh
curl -s 'https://searx.example.org/search?q=test&format=json' | head -c 200
```

If you get HTML back, JSON is still off and sit will report `JSON API disabled on this instance`.

Many public instances deliberately disable JSON or rate-limit it aggressively, so the backend also keeps
a `fallbacks` list and walks it until one answers. Edit it with
`sit config set backends.searxng.fallbacks "https://a.example, https://b.example"`, or empty it to pin a
single instance. Public instances come and go - run your own or use Degoog if you need something dependable.

## Brave Search

The Brave backend uses the official [Brave Search API](https://brave.com/search/api/). Create a key,
then prefer the environment so it never touches disk:

```sh
export SIT_BRAVE_API_KEY="…"   # or BRAVE_API_KEY
sit config set general.backend brave
```

If you would rather store it, `backends.brave.api_key` works too; the config file is written with `0600`
permissions and the key is masked in `sit config show` and in the settings panel. The key is sent in the
`X-Subscription-Token` header only - never in a URL, never logged.

Without a key, the backend scrapes `search.brave.com` instead and shows as `brave (web)`. Like `ddg`, this is
unofficial and best-effort: it can break when the markup changes, and Brave rate-limits it after a handful of
quick searches. Region is ignored in this mode. Set a key if you need something dependable.

## DuckDuckGo

The `ddg` backend has no API key and no setup: it scrapes DuckDuckGo's Lite/HTML endpoints. This is
unofficial and best-effort. It can break without notice when the markup changes, and heavy use may earn
you a rate-limit page. Use SearXNG or Brave if you need something dependable.

## Terminal graphics

Favicons are fetched once, downscaled and cached, then drawn with whichever protocol your terminal
speaks. Protocol detection is purely environment-based - sit never queries the terminal for it, because
the reply would corrupt the TUI's input stream. The one exception is the background colour, asked once
before the TUI starts, so `theme.icon_backdrop` can put a light circle behind dark icons on a dark
terminal and a dark circle behind light icons on a light one.

| Terminal | Protocol | Detected from |
| --- | --- | --- |
| kitty, Ghostty, WezTerm | kitty graphics | `TERM` contains `kitty`, `KITTY_WINDOW_ID`, `TERM_PROGRAM=ghostty`, `GHOSTTY_RESOURCES_DIR`, `WEZTERM_EXECUTABLE` |
| iTerm2 | iTerm2 inline images | `TERM_PROGRAM=iTerm.app`, `LC_TERMINAL=iTerm2` |
| mintty, WezTerm, xterm with sixel | sixel | `TERM` contains `sixel`, `TERM_PROGRAM=mintty`/`WezTerm`, `COLORTERM` contains `sixel` |
| anything else | monogram fallback | - |

The fallback is a single accent-coloured letter taken from the site's domain, so the layout is identical
either way. Inside tmux the escape sequences are wrapped in a passthrough; tmux needs
`set -g allow-passthrough on` for that to reach the terminal.

Overrides:

| Variable | Effect |
| --- | --- |
| `SIT_GRAPHICS` | force `kitty`, `iterm2`, `sixel`, `none`, or `auto` (the default) |
| `SIT_CELL_PX` | cell size in pixels as `WxH`, default `10x20`; fix it if icons look squashed |

`theme.icons = false` turns icons off entirely. `sit doctor` prints what was detected, and
`sit cache clear` drops the cached icons.
