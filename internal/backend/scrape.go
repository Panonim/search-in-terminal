package backend

import (
	"html"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// scraper finds results by shape (outbound links followed by prose) rather than exact markup.
type scraper struct {
	source string
	own    []string
	unwrap func(string) string
	// block marks where each result starts; when it matches nothing the whole page is scanned.
	block *regexp.Regexp
}

var (
	noiseRE  = regexp.MustCompile(`(?is)<!--.*?-->|<script\b.*?</script\s*>|<style\b.*?</style\s*>|<noscript\b.*?</noscript\s*>|<svg\b.*?</svg\s*>|<template\b.*?</template\s*>`)
	anchorRE = regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a\s*>`)
	attrRE   = regexp.MustCompile(`(?s)([\w:-]+)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)
	// inlineRE matches formatting tags inside text, so dropping them keeps sentences whole.
	inlineRE    = regexp.MustCompile(`(?i)</?(?:b|strong|em|i|u|mark|span|small|abbr|code|sup|sub|time|font)\b[^>]*>|<wbr\s*/?>`)
	titleHintRE = regexp.MustCompile(`(?is)<[a-z][^>]*\bclass\s*=\s*["']?[^"'>]*title[^>]*>([^<]+)`)
	imgRE       = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	urlishRE    = regexp.MustCompile(`(?i)^(?:https?://)?[\w-]+(?:\.[\w-]+)*\.[a-z]{2,}(?:[/?#:]\S*)?$`)
	// moreRE catches "More on reddit.com" style links that point into a result rather than being one.
	moreRE = regexp.MustCompile(`(?i)^(?:more (?:on|from|results|about)|(?:see|read|view|show) (?:more|all))\b`)
)

type anchor struct {
	start, end       int
	href, url, title string
}

func (s scraper) parse(body string) []Result {
	body = noiseRE.ReplaceAllString(body, "")
	if s.block != nil {
		var starts []int
		for _, m := range s.block.FindAllStringIndex(body, -1) {
			// The marker may sit inside a tag, so split at that tag's start.
			starts = append(starts, max(0, strings.LastIndex(body[:m[0]], "<")))
		}
		var out []Result
		seen := map[string]bool{}
		for i, st := range starts {
			end := len(body)
			if i+1 < len(starts) {
				end = starts[i+1]
			}
			out = append(out, s.links(body[st:end], seen, 1)...)
		}
		if len(out) > 0 {
			return out
		}
	}
	out := s.links(body, map[string]bool{}, -1)
	// On a page whose results have snippets, bare links trailing it are footer navigation.
	if slices.ContainsFunc(out, func(r Result) bool { return r.Snippet != "" }) {
		for out[len(out)-1].Snippet == "" && out[len(out)-1].FaviconURL == "" {
			out = out[:len(out)-1]
		}
	}
	return out
}

func (s scraper) links(body string, seen map[string]bool, limit int) []Result {
	var anchors []anchor
	for _, m := range anchorRE.FindAllStringSubmatchIndex(body, -1) {
		attrs, inner := body[m[2]:m[3]], body[m[4]:m[5]]
		a := anchor{start: m[0], end: m[1], href: html.UnescapeString(attr(attrs, "href"))}
		if a.href == "" {
			continue
		}
		a.url = s.resolve(a.href)
		a.title = linkTitle(inner)
		if a.title == "" {
			a.title = clean(attr(attrs, "title") + " " + attr(attrs, "aria-label"))
		}
		anchors = append(anchors, a)
	}

	var out []Result
	for i, a := range anchors {
		if len(out) == limit {
			break
		}
		if a.url == "" || a.title == "" || seen[a.url] || moreRE.MatchString(a.title) {
			continue
		}
		seen[a.url] = true
		// The result's area runs until the next link elsewhere; repeats of its own URL (icon, url line) stay in.
		end := len(body)
		for _, b := range anchors[i+1:] {
			if b.href != a.href && b.url != a.url && b.title != "" {
				end = b.start
				break
			}
		}
		out = append(out, Result{
			Title:      a.title,
			URL:        a.url,
			Snippet:    prose(body[a.end:end], a.title),
			FaviconURL: favicon(body[a.start:end]),
			Source:     s.source,
		})
	}
	return out
}

func (s scraper) resolve(href string) string {
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	if s.unwrap != nil {
		href = s.unwrap(href)
	}
	u, err := url.Parse(href)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	for _, d := range s.own {
		if host == d || strings.HasSuffix(host, "."+d) {
			return ""
		}
	}
	return u.String()
}

func attr(tag, name string) string {
	for _, m := range attrRE.FindAllStringSubmatch(tag, -1) {
		if strings.EqualFold(m[1], name) {
			return m[2] + m[3] + m[4]
		}
	}
	return ""
}

func texts(s string) []string {
	var out []string
	for _, t := range tagRE.Split(inlineRE.ReplaceAllString(s, ""), -1) {
		if t = clean(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func linkTitle(inner string) string {
	inner = inlineRE.ReplaceAllString(inner, "")
	if m := titleHintRE.FindStringSubmatch(inner); m != nil {
		if t := clean(m[1]); t != "" {
			return t
		}
	}
	best, fallback := "", ""
	for _, t := range texts(inner) {
		if len(t) > len(fallback) {
			fallback = t
		}
		if len(t) > len(best) && !looksLikeURL(t) {
			best = t
		}
	}
	if best == "" {
		return fallback
	}
	return best
}

func prose(area, title string) string {
	best := ""
	for _, t := range texts(area) {
		if len(t) > len(best) && t != title && strings.Count(t, " ") >= 2 && !looksLikeURL(t) {
			best = t
		}
	}
	return best
}

func favicon(area string) string {
	for _, tag := range imgRE.FindAllString(area, -1) {
		if !strings.Contains(strings.ToLower(tag), "icon") {
			continue
		}
		src := html.UnescapeString(attr(tag, "src"))
		if strings.HasPrefix(src, "//") {
			src = "https:" + src
		}
		if strings.HasPrefix(src, "http") {
			return src
		}
	}
	return ""
}

func looksLikeURL(s string) bool {
	return strings.Contains(s, "›") || urlishRE.MatchString(s)
}
