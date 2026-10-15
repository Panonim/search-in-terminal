# Shows the current weather for "weather <city>" using wttr.in.
manifest = {
    "name": "weather",
    "version": "1.0.0",
    "description": "Current weather from wttr.in",
    "triggers": ["(?i)^weather\\s+\\S"],
    "hosts": ["wttr.in"],
}

def answer(query):
    city = query.split(" ", 1)[1].strip()
    city = "".join([c for c in city.codepoints() if c.isalnum() or c in " -"]).strip()
    if not city:
        return None
    page = "https://wttr.in/" + city.replace(" ", "+")
    return {
        "title": "weather in " + city,
        "body": http_get(page + "?format=3").strip(),
        "url": page,
    }
