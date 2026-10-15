package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const echo = `
manifest = {"name": "echo", "version": "1", "triggers": ["^echo "], "hosts": []}

def answer(query):
    return {"title": "echo", "body": query[5:]}
`

func mustLoad(t *testing.T, src string) *Plugin {
	t.Helper()
	p, err := Load("test.star", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAnswerOnlyForMatchingQueries(t *testing.T) {
	p := mustLoad(t, echo)
	a, err := p.Answer(context.Background(), "echo hi")
	if err != nil || a == nil || a.Body != "hi" || a.Source != "echo" {
		t.Fatalf("got %+v, %v", a, err)
	}
	if a, err := p.Answer(context.Background(), "hello"); a != nil || err != nil {
		t.Errorf("non-matching query reached the plugin: %+v, %v", a, err)
	}
}

func TestManifestValidation(t *testing.T) {
	cases := map[string]string{
		"missing manifest":  "def answer(q): return None",
		"bad name":          `manifest = {"name": "../x", "triggers": ["x"]}` + "\ndef answer(q): return None",
		"no triggers":       `manifest = {"name": "x", "triggers": []}` + "\ndef answer(q): return None",
		"bad regex":         `manifest = {"name": "x", "triggers": ["("]}` + "\ndef answer(q): return None",
		"host with scheme":  `manifest = {"name": "x", "triggers": ["x"], "hosts": ["https://a.io"]}` + "\ndef answer(q): return None",
		"no answer":         `manifest = {"name": "x", "triggers": ["x"]}`,
		"escape in desc":    `manifest = {"name": "x", "description": "\x1b[1A\x1b[2K", "triggers": ["x"]}` + "\ndef answer(q): return None",
		"escape in trigger": `manifest = {"name": "x", "triggers": ["x\x1b[2K"]}` + "\ndef answer(q): return None",
		"bidi in trigger":   `manifest = {"name": "x", "triggers": ["x‮"]}` + "\ndef answer(q): return None",
		"load is refused":   `load("other.star", "x")`,
		"while is refused":  "def f():\n    while True:\n        pass\n",
		"top-level for":     "for i in range(3):\n    pass\n",
		"recursion refused": `manifest = {"name": "x", "triggers": ["x"]}` + "\ndef answer(q): return answer(q)\nanswer(1)",
	}
	for name, src := range cases {
		if _, err := Load("test.star", []byte(src)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestErrorsAreSanitised(t *testing.T) {
	_, err := Load("test.star", []byte(`fail("\x1b]52;c;aGk=\x07boom")`))
	if err == nil || strings.ContainsAny(err.Error(), "\x1b\x07") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("want a sanitised error, got %q", err)
	}
}

func TestStepLimit(t *testing.T) {
	p := mustLoad(t, `
manifest = {"name": "spin", "triggers": ["."]}

def answer(query):
    n = 0
    for i in range(100000000):
        n += i
    return {"body": str(n)}
`)
	if _, err := p.Answer(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "steps") {
		t.Errorf("want a step limit error, got %v", err)
	}
}

func TestOutputCannotEscapeTheTerminal(t *testing.T) {
	p := mustLoad(t, `
manifest = {"name": "evil", "triggers": ["."]}

def answer(query):
    return {"title": "\x1b]52;c;aGk=\x07ok", "body": "a\x1b[2Jb\nc", "url": "javascript:alert(1)"}
`)
	a, err := p.Answer(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(a.Title+a.Body, "\x1b\x07") || a.Body != "a[2Jb\nc" || a.URL != "" {
		t.Errorf("unsanitised output: %+v", a)
	}
}

func netPlugin(t *testing.T, host string) *Plugin {
	return mustLoad(t, `
manifest = {"name": "net", "triggers": ["."], "hosts": ["`+host+`"]}

def answer(query):
    return {"body": http_get(query)}
`)
}

func TestHTTPGetOnlyApprovedHosts(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hop" {
			http.Redirect(w, r, "https://elsewhere.test/", http.StatusFound)
			return
		}
		w.Write([]byte("sunny"))
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "https://")

	p := netPlugin(t, host)
	if _, err := p.Answer(context.Background(), srv.URL); err == nil || !strings.Contains(err.Error(), "network is off") {
		t.Errorf("an uninstalled plugin must not reach the network, got %v", err)
	}

	p.network = true
	p.client = srv.Client()
	p.client.CheckRedirect = p.redirect
	if a, err := p.Answer(context.Background(), srv.URL+"/"); err != nil || a.Body != "sunny" {
		t.Errorf("approved host: %+v, %v", a, err)
	}
	for _, target := range []string{"https://example.com/", strings.Replace(srv.URL, "https", "http", 1), srv.URL + "/hop"} {
		if _, err := p.Answer(context.Background(), target); err == nil {
			t.Errorf("%s should be refused", target)
		}
	}
}

func TestInstallPinsHash(t *testing.T) {
	t.Setenv("SIT_CONFIG_DIR", t.TempDir())
	p := mustLoad(t, echo)
	if err := Install(p, []byte(echo)); err != nil {
		t.Fatal(err)
	}
	if st := Installed(); len(st) != 1 || st[0].Plugin == nil {
		t.Fatalf("installed = %+v", st)
	}
	if err := os.WriteFile(filePath("echo"), []byte(echo+"\n# edited"), 0o600); err != nil {
		t.Fatal(err)
	}
	if st := Installed(); st[0].Plugin != nil || !strings.Contains(st[0].Problem, "changed") {
		t.Errorf("an edited plugin must be disabled, got %+v", st[0])
	}
	if len(Providers()) != 0 {
		t.Error("a disabled plugin must not answer")
	}
	if err := Remove("echo"); err != nil || len(Installed()) != 0 {
		t.Errorf("remove: %v", err)
	}
}
