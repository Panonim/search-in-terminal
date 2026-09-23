# SIT - Search in terminal

## Install

### The quick way

By default `sit` goes into `~/.local/bin`. If you run the installer as root it uses `/usr/local/bin`,
and on Windows it uses `%LOCALAPPDATA%\Programs\sit`. Want it somewhere else? Pass `-dir <path>`.
If that folder isn't on your `PATH` yet, the installer tells you how to add it.

On Linux, macOS or FreeBSD:

```sh
curl -fsSLo sit-install "https://github.com/Panonim/search-in-terminal/releases/latest/download/sit-install_$(uname -s | tr A-Z a-z)_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
chmod +x sit-install && ./sit-install && rm sit-install
```

On Windows, in PowerShell:

```powershell
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
Invoke-WebRequest "https://github.com/Panonim/search-in-terminal/releases/latest/download/sit-install_windows_$arch.exe" -OutFile sit-install.exe
.\sit-install.exe; Remove-Item sit-install.exe
```

#### If you already have Go, you can skip the download and run the installer from source:

```sh
go run github.com/Panonim/search-in-terminal@latest
```

### With go install

You'll need Go 1.26 or newer:

```sh
go install github.com/Panonim/search-in-terminal/cmd/sit@latest
```

### Staying up to date

`sit` can update itself:

```sh
sit update --check   # see if there's a newer release, without changing anything
sit update           # download it, check it and swap in the new binary
```

It needs to be able to write to the folder `sit` lives in. If you installed it somewhere owned by
root, run `sudo sit update` or just run the installer again.

## Quick start

```sh
sit                          # open the UI
sit bubble tea tutorial      # open the UI with the query already running
sit search -n 5 "go generics"
sit search --json "ripgrep" | jq -r '.[].URL'
sit open "go module proxy"   # open the first hit in your browser
sit backends                 # which backends are configured and ready
sit doctor                   # config, cache, graphics support and a live probe of each backend
```

<details>
<summary><strong>Key bindings &amp; commands</strong></summary>
<br>

## Key bindings

Defaults. Anything with a setting name can be rebound under `[keys]` in the config file.

| Key | Action | Setting |
| --- | --- | --- |
| <kbd>/</kbd> | focus the search input | `keys.focus` |
| <kbd>Enter</kbd> | search, or open the selected result | `keys.open` |
| <kbd>Tab</kbd> / <kbd>Shift+Tab</kbd> | cycle backend and re-run the query | `keys.next_backend`, `keys.prev_backend` |
| <kbd>j</kbd> <kbd>k</kbd> / <kbd>↓</kbd> <kbd>↑</kbd> | move the selection | `keys.down`, `keys.up` |
| <kbd>Ctrl+d</kbd> / <kbd>Ctrl+u</kbd> | page through results | - |
| <kbd>g</kbd> / <kbd>G</kbd> | jump to first / last result | - |
| <kbd>n</kbd> / <kbd>p</kbd> | next / previous result page | `keys.next_page`, `keys.prev_page` |
| <kbd>y</kbd> | copy the selected URL to the clipboard | `keys.copy` |
| <kbd>Ctrl+e</kbd> / <kbd>→</kbd> | accept the autocomplete suggestion (Degoog) | - |
| <kbd>r</kbd> | retry the current search | - |
| <kbd>,</kbd> | open the settings panel | `keys.settings` |
| <kbd>?</kbd> | toggle help | `keys.help` |
| <kbd>q</kbd> / <kbd>Ctrl+c</kbd> | quit | `keys.quit` |

Below the last result sits a **load more results** button: select it and press <kbd>Enter</kbd>, or click it, to append the next page. Results already on screen are skipped, so only new links are added.

Inside the settings panel:

| Key | Action |
| --- | --- |
| <kbd>j</kbd> <kbd>k</kbd> / <kbd>↑</kbd> <kbd>↓</kbd> | move between settings |
| <kbd>h</kbd> <kbd>l</kbd> / <kbd>←</kbd> <kbd>→</kbd> | move to the neighbouring column |
| <kbd>1</kbd>–<kbd>6</kbd> | jump to a numbered section |
| <kbd>Tab</kbd> / <kbd>Shift+Tab</kbd> | jump to the next / previous section |
| <kbd>Space</kbd> | cycle a boolean or enum value |
| <kbd>Enter</kbd> | edit a text value, or toggle a boolean/enum |
| <kbd>Ctrl+s</kbd> or <kbd>w</kbd> | save to the config file |
| <kbd>Ctrl+r</kbd> | restore defaults (not saved until you save) |
| <kbd>Esc</kbd> / <kbd>,</kbd> | back to the results |

## Commands

| Command | What it does |
| --- | --- |
| `sit` | open the search UI |
| `sit <query…>` | open the UI with a query already running |
| `sit search <query…>` | print results without the UI |
| `sit open <query…>` | open the first result in your browser |
| `sit suggest <query…>` | print autocomplete suggestions (Degoog instances) |
| `sit backends` | list backends and whether they are ready |
| `sit config <subcommand>` | inspect and change settings |
| `sit update [--check]` | update sit to the latest release |
| `sit cache [info\|clear]` | inspect or drop the favicon cache |
| `sit doctor` | check config, network and terminal support |
| `sit version` | print version, commit and build date |
| `sit help` | usage summary |

`sit search` flags:

| Flag | Meaning |
| --- | --- |
| `-b`, `--backend <name>` | `ddg`, `degoog`, `searxng` or `brave` |
| `-n`, `--limit <count>` | maximum results to print |
| `-p`, `--page <number>` | result page |
| `--json` | print JSON instead of text |

`sit config` subcommands:

| Command | What it does |
| --- | --- |
| `sit config show` | print the effective configuration |
| `sit config path` | print the config file path |
| `sit config init` | write a config file with the current settings |
| `sit config edit` | open the config file in `$EDITOR` |
| `sit config keys` | list every setting key |
| `sit config get <key>` | print one setting |
| `sit config set <key> <value>` | change one setting and save |

</details>

## Configuration

See [docs/config.md](docs/config.md) for the config file, backend setup (Degoog, SearXNG, Brave,
DuckDuckGo) and terminal graphics support.
