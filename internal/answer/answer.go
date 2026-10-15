// Package answer finds instant answers shown above the results: built-in calculator and unit
// conversion first, then plugins.
package answer

import (
	"context"
	"math"
	"strconv"
)

type Answer struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url,omitempty"`
	Source string `json:"source"`
}

// Provider is an answer source outside this package, such as a plugin.
type Provider interface {
	Answer(ctx context.Context, query string) (*Answer, error)
}

// Find returns the first answer for query, or nil; failing providers are skipped.
func Find(ctx context.Context, query string, providers []Provider) *Answer {
	if a := Calc(query); a != nil {
		return a
	}
	if a := Convert(query); a != nil {
		return a
	}
	for _, p := range providers {
		if ctx.Err() != nil {
			return nil
		}
		if a, err := p.Answer(ctx, query); err == nil && a != nil {
			return a
		}
	}
	return nil
}

// format rounds v to sig significant digits without falling back to exponent notation for everyday sizes.
func format(v float64, sig int) string {
	r, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'g', sig, 64), 64)
	if a := math.Abs(r); a != 0 && (a >= 1e15 || a < 1e-6) {
		return strconv.FormatFloat(r, 'g', -1, 64)
	}
	return strconv.FormatFloat(r, 'f', -1, 64)
}
