package backend

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
)

const dateLayout = "2006-01-02"

// query is a search split into the text sent to engines and the operators sit checks results against.
type query struct {
	text          string
	after, before string
	rules         []rule
	or            bool
}

// rule is one operator; op is site, intitle, inurl, filetype, or empty for a word or phrase.
type rule struct {
	op, value string
	not       bool
}

var ops = map[string]string{"site": "site", "intitle": "intitle", "inurl": "inurl", "filetype": "filetype", "ext": "filetype"}

// parseQuery lifts after:/before: out of the text, as engines take dates as parameters, and leaves every other operator in it.
func parseQuery(raw string) query {
	var q query
	var words []string
	for _, w := range fields(raw) {
		name, val, _ := strings.Cut(strings.ToLower(w), ":")
		if _, err := time.Parse(dateLayout, val); err == nil && (name == "after" || name == "before") {
			if name == "after" {
				q.after = val
			} else {
				q.before = val
			}
			continue
		}
		words = append(words, w)
		if w == "OR" || w == "|" {
			q.or = true
			continue
		}
		value, not := strings.CutPrefix(strings.ToLower(w), "-")
		r := rule{value: value, not: not}
		if op, val, ok := strings.Cut(value, ":"); ok && ops[op] != "" {
			r.op, r.value = ops[op], val
		}
		quoted := strings.HasPrefix(r.value, `"`)
		r.value = strings.Trim(r.value, `"`)
		// A bare word is left to the engine, which may match its stem or a synonym.
		bare := r.op == "" && !quoted && !r.not
		if !bare && len(splitWords(r.value)) > 0 {
			q.rules = append(q.rules, r)
		}
	}
	q.text = strings.Join(words, " ")
	return q
}

// fields splits on spaces outside double quotes, so a quoted phrase stays one token.
func fields(s string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range s {
		if r == '"' {
			quoted = !quoted
		}
		if unicode.IsSpace(r) && !quoted {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// span joins the date range with sep, filling an open end for engines that need both.
func (q query) span(sep string) string {
	if q.after == "" && q.before == "" {
		return ""
	}
	from, to := q.after, q.before
	if from == "" {
		from = "1970-01-01"
	}
	if to == "" {
		to = time.Now().Format(dateLayout)
	}
	return from + sep + to
}

// recent picks the tightest of day, week, month or year covering after:, for engines without custom ranges.
func (q query) recent() string {
	t, err := time.Parse(dateLayout, q.after)
	if err != nil {
		return ""
	}
	age := time.Since(t)
	for _, r := range []struct {
		name string
		days time.Duration
	}{{"day", 1}, {"week", 7}, {"month", 31}, {"year", 366}} {
		if age <= r.days*24*time.Hour {
			return r.name
		}
	}
	return ""
}

// keep checks a result against every rule, where several site: rules, or any rules joined by OR, need only one match.
func (q query) keep(r Result) bool {
	var wantSite, onAnySite, wantAny, anyHit bool
	for _, rl := range q.rules {
		hit := rl.match(r)
		switch {
		case rl.not:
			if hit {
				return false
			}
		case rl.op == "site":
			wantSite, onAnySite = true, onAnySite || hit
		case q.or:
			wantAny, anyHit = true, anyHit || hit
		case !hit:
			return false
		}
	}
	return (!wantSite || onAnySite) && (!wantAny || anyHit)
}

func (rl rule) match(r Result) bool {
	u, err := url.Parse(r.URL)
	if err != nil {
		return false
	}
	switch rl.op {
	case "site":
		return onSite(u, rl.value)
	case "inurl":
		return strings.Contains(strings.ToLower(u.String()), rl.value)
	case "filetype":
		return strings.HasSuffix(strings.ToLower(u.Path), "."+rl.value)
	case "intitle":
		return hasPhrase(r.Title, rl.value)
	}
	return hasPhrase(r.Title+" "+r.Snippet+" "+r.URL, rl.value)
}

// hasPhrase matches whole words in order, so "java" is not found in "javascript".
func hasPhrase(text, phrase string) bool {
	return strings.Contains(" "+strings.Join(splitWords(text), " ")+" ", " "+strings.Join(splitWords(phrase), " ")+" ")
}

// onSite matches a site: value against a URL's host and, when the value has one, its path.
func onSite(u *url.URL, site string) bool {
	domain, path, _ := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(site, "https://"), "http://"), "/")
	host := strings.ToLower(u.Hostname())
	onDomain := host == domain || strings.HasSuffix(host, "."+domain)
	return onDomain && strings.HasPrefix(strings.ToLower(strings.TrimPrefix(u.Path, "/")), path)
}

func splitWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// filtered drops results breaking the query's operators, since not every engine honours them.
type filtered struct{ Backend }

func (f filtered) Search(ctx context.Context, raw string, page int) ([]Result, error) {
	results, err := f.Backend.Search(ctx, raw, page)
	q := parseQuery(raw)
	return slices.DeleteFunc(results, func(r Result) bool { return !q.keep(r) }), err
}

func (f filtered) Suggest(ctx context.Context, query string) []string {
	if s, ok := f.Backend.(Suggester); ok {
		return s.Suggest(ctx, query)
	}
	return nil
}
