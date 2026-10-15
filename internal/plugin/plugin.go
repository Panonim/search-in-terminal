// Package plugin runs instant-answer plugins written in Starlark inside a sandbox: no files, no
// commands, no environment, and network only to the hosts the user approved at install time.
package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/Panonim/search-in-terminal/internal/answer"
)

const (
	maxSteps     = 5_000_000
	callTimeout  = 5 * time.Second
	maxHTTPCalls = 3
	maxBody      = 1 << 20
	maxTitle     = 200
	maxText      = 2000
)

var (
	nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	hostRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?(:\d{1,5})?$`)
)

type Plugin struct {
	Name        string
	Version     string
	Description string
	Triggers    []string
	Hosts       []string
	SHA256      string

	triggers []*regexp.Regexp
	answer   starlark.Callable
	// network is set only for installed plugins whose hosts the user approved.
	network bool
	client  *http.Client
}

// Load runs src in the sandbox and validates its manifest, without network access.
func Load(filename string, src []byte) (*Plugin, error) {
	sum := sha256.Sum256(src)
	p := &Plugin{SHA256: hex.EncodeToString(sum[:])}
	globals, err := starlark.ExecFileOptions(&syntax.FileOptions{}, newThread(), filename, src, p.predeclared())
	if err != nil {
		return nil, explain(err)
	}
	globals.Freeze()
	if err := p.readManifest(globals["manifest"]); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	fn, ok := globals["answer"].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("%s: missing def answer(query)", filename)
	}
	p.answer = fn
	return p, nil
}

func (p *Plugin) readManifest(v starlark.Value) error {
	m, ok := v.(*starlark.Dict)
	if !ok {
		return errors.New("missing manifest = {...}")
	}
	str := func(key string) string {
		v, _, _ := m.Get(starlark.String(key))
		s, _ := starlark.AsString(v)
		return strings.TrimSpace(s)
	}
	p.Name, p.Version, p.Description = str("name"), str("version"), str("description")
	if !nameRE.MatchString(p.Name) {
		return fmt.Errorf("manifest name %q must be 1-32 lowercase letters, digits or dashes", p.Name)
	}
	var err error
	if p.Triggers, err = strList(m, "triggers"); err != nil {
		return err
	}
	// The install warning prints these, so escape sequences in them could disguise what is being approved.
	for _, s := range append([]string{p.Version, p.Description}, p.Triggers...) {
		if strings.IndexFunc(s, hidden) >= 0 || !utf8.ValidString(s) {
			return fmt.Errorf("manifest value %q contains control characters", s)
		}
	}
	if utf8.RuneCountInString(p.Version) > 32 || utf8.RuneCountInString(p.Description) > maxTitle {
		return errors.New("manifest version or description too long")
	}
	if len(p.Triggers) == 0 {
		return errors.New("manifest needs at least one trigger regex")
	}
	for _, t := range p.Triggers {
		re, err := regexp.Compile(t)
		if err != nil {
			return fmt.Errorf("trigger %q: %w", t, err)
		}
		p.triggers = append(p.triggers, re)
	}
	if p.Hosts, err = strList(m, "hosts"); err != nil {
		return err
	}
	for i, h := range p.Hosts {
		p.Hosts[i] = strings.ToLower(h)
		if !hostRE.MatchString(p.Hosts[i]) {
			return fmt.Errorf("host %q must be a plain hostname such as wttr.in", h)
		}
	}
	return nil
}

func strList(m *starlark.Dict, key string) ([]string, error) {
	v, found, _ := m.Get(starlark.String(key))
	if !found {
		return nil, nil
	}
	l, ok := v.(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("manifest %s must be a list of strings", key)
	}
	out := make([]string, 0, l.Len())
	for i := range l.Len() {
		s, ok := starlark.AsString(l.Index(i))
		if !ok {
			return nil, fmt.Errorf("manifest %s must be a list of strings", key)
		}
		out = append(out, s)
	}
	return out, nil
}

// Matches reports whether query hits a trigger; plugins never see other queries.
func (p *Plugin) Matches(query string) bool {
	return slices.ContainsFunc(p.triggers, func(re *regexp.Regexp) bool { return re.MatchString(query) })
}

// Answer calls the plugin's answer(query) under the step limit and timeout.
func (p *Plugin) Answer(ctx context.Context, query string) (*answer.Answer, error) {
	if !p.Matches(query) {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	thread := newThread()
	thread.SetLocal("ctx", ctx)
	stop := context.AfterFunc(ctx, func() { thread.Cancel("timed out") })
	defer stop()

	v, err := starlark.Call(thread, p.answer, starlark.Tuple{starlark.String(query)}, nil)
	if err != nil {
		return nil, explain(err)
	}
	if v == starlark.None {
		return nil, nil
	}
	d, ok := v.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("answer() must return None or a dict, got %s", v.Type())
	}
	field := func(key string, limit int) string {
		v, _, _ := d.Get(starlark.String(key))
		s, _ := starlark.AsString(v)
		return sanitize(s, limit)
	}
	a := &answer.Answer{Title: field("title", maxTitle), Body: field("body", maxText), URL: field("url", maxTitle), Source: p.Name}
	if u, err := url.Parse(a.URL); err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		a.URL = ""
	}
	if a.Title == "" && a.Body == "" {
		return nil, nil
	}
	return a, nil
}

func newThread() *starlark.Thread {
	t := &starlark.Thread{Name: "plugin", Print: func(*starlark.Thread, string) {}}
	t.SetMaxExecutionSteps(maxSteps)
	return t
}

func (p *Plugin) predeclared() starlark.StringDict {
	return starlark.StringDict{
		"json":     json.Module,
		"http_get": starlark.NewBuiltin("http_get", p.httpGet),
	}
}

func (p *Plugin) httpGet(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var raw string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "url", &raw); err != nil {
		return nil, err
	}
	ctx, ok := thread.Local("ctx").(context.Context)
	if !ok {
		return nil, errors.New("http_get is only available inside answer()")
	}
	if !p.network {
		return nil, errors.New("http_get: network is off for plugins that are not installed with approved hosts")
	}
	calls, _ := thread.Local("calls").(int)
	if calls >= maxHTTPCalls {
		return nil, fmt.Errorf("http_get: at most %d requests per answer", maxHTTPCalls)
	}
	thread.SetLocal("calls", calls+1)
	u, err := url.Parse(raw)
	if err != nil || !p.approved(u) {
		return nil, fmt.Errorf("http_get: %q is not an https URL on an approved host (%s)", raw, strings.Join(p.Hosts, ", "))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sit-plugin/"+p.Name)
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("http_get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("http_get: http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("http_get: %w", err)
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("http_get: response larger than %d bytes", maxBody)
	}
	return starlark.String(body), nil
}

func (p *Plugin) httpClient() *http.Client {
	if p.client != nil {
		return p.client
	}
	return &http.Client{Timeout: callTimeout, CheckRedirect: p.redirect}
}

func (p *Plugin) approved(u *url.URL) bool {
	return u.Scheme == "https" && slices.Contains(p.Hosts, strings.ToLower(u.Host))
}

// redirect keeps a plugin on its approved hosts, so a server cannot bounce it somewhere else.
func (p *Plugin) redirect(req *http.Request, via []*http.Request) error {
	if !p.approved(req.URL) {
		return fmt.Errorf("redirect to unapproved host %s", req.URL.Host)
	}
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	return nil
}

// sanitize drops control characters, including ESC, so plugin output cannot drive the terminal.
func sanitize(s string, limit int) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.ToValidUTF8(s, "") {
		if r == '\t' {
			r = ' '
		}
		if (r < 0x20 && r != '\n') || (r >= 0x7f && r < 0xa0) || r == utf8.RuneError {
			continue
		}
		if n++; n > limit {
			b.WriteString("…")
			break
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// hidden matches control and invisible format characters such as bidi overrides, which could disguise a manifest at review.
func hidden(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }

// explain sanitises errors too, since fail() messages and response bodies inside them come from the plugin.
func explain(err error) error {
	var e *starlark.EvalError
	if errors.As(err, &e) {
		return errors.New(sanitize(e.Backtrace(), maxText))
	}
	return errors.New(sanitize(err.Error(), maxText))
}
