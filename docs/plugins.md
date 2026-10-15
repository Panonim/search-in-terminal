# Plugins

sit shows an **instant answer** card above the results when it can answer a query itself. The
calculator (`2^10 * 3`) and unit conversion (`5 km in mi`, `100 c to f`, `1 GiB in MB`) are built in.
Plugins add more, written in [Starlark](https://github.com/bazelbuild/starlark/blob/master/spec.md),
a small Python dialect built for running untrusted code.

Turn every answer off with `sit config set general.instant_answers false`.

## Writing a plugin

A plugin is one `.star` file with a `manifest` and an `answer` function:

```python
manifest = {
    "name": "weather",                       # 1-32 lowercase letters, digits or dashes
    "version": "1.0.0",
    "description": "Current weather from wttr.in",
    "triggers": ["(?i)^weather\\s+\\S"],     # Go regexes; only matching queries reach answer()
    "hosts": ["wttr.in"],                    # optional: hosts http_get may contact
}

def answer(query):
    city = query.split(" ", 1)[1].strip()
    return {
        "title": "weather in " + city,
        "body": http_get("https://wttr.in/" + city + "?format=3"),
        "url": "https://wttr.in/" + city,    # optional
    }
```

`answer` returns `None` for no answer, or a dict with `title`, `body` (newlines allowed) and an optional
`url`. The full example is [plugins/weather.star](plugins/weather.star).

Available beyond the Starlark built-ins:

| Name | What it does |
| --- | --- |
| `http_get(url)` | GET an `https://` URL on a host listed in `hosts`, returns the body as a string |
| `json.decode(s)`, `json.encode(v)` | parse and produce JSON |

Try a file while you write it, with the network off:

```sh
sit plugin test ./weather.star "weather berlin"
```

Errors print a Starlark traceback with the line number.

## Installing

```sh
sit plugin add ./weather.star     # shows what the plugin can do, then asks y/N
sit plugin list                   # installed plugins and whether they work
sit plugin test weather "weather berlin"   # run the installed copy, network on
sit plugin remove weather
```

`sit plugin add` only takes local files. Before installing it shows the plugin's name, version and
SHA-256, every trigger (the queries it will see), and every host it can send them to. Nothing is
installed unless you answer `y`. The file is copied to `~/.config/sit/plugins/` and its hash and hosts
are pinned in `approved.json`. If the file changes afterwards, sit disables the plugin until you run
`sit plugin add` on it again. `sit plugin list` and `sit doctor` say when that happens.

## The sandbox

| Plugins can | Plugins cannot |
| --- | --- |
| read queries that match their triggers | see any other query |
| compute with Starlark's built-ins and `json` | read or write files, run commands, read environment variables |
| `http_get` the hosts you approved, over https, at most 3 requests per answer, 1 MiB each | reach any other host, or be redirected to one |
| return text for the card | send terminal escape sequences (control characters are stripped) |

`load()`, `while` loops and recursion are disabled. Each answer runs under a step limit and a 5 second
timeout. Memory use cannot be capped, so only install plugins you have read.
