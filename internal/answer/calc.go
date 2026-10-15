package answer

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var dateLike = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Calc evaluates an arithmetic query such as "2^10 * 3"; anything else returns nil.
func Calc(query string) *Answer {
	expr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(query), "="))
	if expr == "" || dateLike.MatchString(expr) {
		return nil
	}
	p := &parser{src: expr}
	v, err := p.parse()
	if err != nil || !p.op || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &Answer{Title: expr, Body: "= " + format(v, 12), Source: "calculator"}
}

var errSyntax = errors.New("not an expression")

// parser is a recursive-descent evaluator; op records whether a binary operator was seen, so bare numbers are not answers.
type parser struct {
	src string
	pos int
	op  bool
}

func (p *parser) parse() (float64, error) {
	v, err := p.expr()
	if err != nil {
		return 0, err
	}
	if p.skip(); p.pos != len(p.src) {
		return 0, errSyntax
	}
	return v, nil
}

func (p *parser) skip() {
	for p.pos < len(p.src) && p.src[p.pos] == ' ' {
		p.pos++
	}
}

func (p *parser) peek() byte {
	p.skip()
	if p.pos < len(p.src) {
		return p.src[p.pos]
	}
	return 0
}

func (p *parser) expr() (float64, error) {
	v, err := p.term()
	for err == nil {
		c := p.peek()
		if c != '+' && c != '-' {
			break
		}
		p.pos++
		p.op = true
		var r float64
		r, err = p.term()
		if c == '+' {
			v += r
		} else {
			v -= r
		}
	}
	return v, err
}

func (p *parser) term() (float64, error) {
	v, err := p.unary()
	for err == nil {
		c := p.peek()
		if c == 'x' || c == 'X' {
			c = '*'
		}
		if c != '*' && c != '/' && c != '%' {
			break
		}
		p.pos++
		p.op = true
		var r float64
		r, err = p.unary()
		switch c {
		case '*':
			v *= r
		case '/':
			v /= r
		case '%':
			v = math.Mod(v, r)
		}
	}
	return v, err
}

func (p *parser) unary() (float64, error) {
	switch p.peek() {
	case '-':
		p.pos++
		v, err := p.unary()
		return -v, err
	case '+':
		p.pos++
		return p.unary()
	}
	return p.power()
}

func (p *parser) power() (float64, error) {
	v, err := p.primary()
	if err != nil || p.peek() != '^' {
		return v, err
	}
	p.pos++
	p.op = true
	e, err := p.unary()
	return math.Pow(v, e), err
}

func (p *parser) primary() (float64, error) {
	c := p.peek()
	if c == '(' {
		p.pos++
		v, err := p.expr()
		if err != nil || p.peek() != ')' {
			return 0, errSyntax
		}
		p.pos++
		return v, nil
	}
	start := p.pos
	for p.pos < len(p.src) && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9' || p.src[p.pos] == '.') {
		p.pos++
	}
	if p.pos > start {
		return strconv.ParseFloat(p.src[start:p.pos], 64)
	}
	rest := strings.ToLower(p.src[p.pos:])
	switch {
	case strings.HasPrefix(rest, "pi"):
		p.pos += 2
		return math.Pi, nil
	case strings.HasPrefix(rest, "e"):
		p.pos++
		return math.E, nil
	}
	return 0, errSyntax
}
