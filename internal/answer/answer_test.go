package answer

import (
	"context"
	"errors"
	"testing"
)

func TestCalc(t *testing.T) {
	cases := map[string]string{
		"2^10 * 3":      "= 3072",
		"1 + 2 * 3":     "= 7",
		"(1 + 2) * 3 =": "= 9",
		"-2^2":          "= -4",
		"2^3^2":         "= 512",
		"10 % 4":        "= 2",
		"3 x 4":         "= 12",
		"1/3":           "= 0.333333333333",
		"2 * pi":        "= 6.28318530718",
	}
	for q, want := range cases {
		if a := Calc(q); a == nil || a.Body != want {
			t.Errorf("Calc(%q) = %+v, want %s", q, a, want)
		}
	}
	for _, q := range []string{"42", "-5", "2024-01-01", "1/0", "go 1+1", "1 +", "(2", "pie"} {
		if a := Calc(q); a != nil {
			t.Errorf("Calc(%q) = %+v, want nil", q, a)
		}
	}
}

func TestConvert(t *testing.T) {
	cases := map[string]string{
		"5 km in mi":       "3.10686 mi",
		"100 C to F":       "212 f",
		"0 f in c":         "-17.7778 c",
		"1 GiB in MB":      "1073.74 mb",
		"2.5 hours in min": "150 min",
		"60 mph in km/h":   "96.5606 km/h",
	}
	for q, want := range cases {
		if a := Convert(q); a == nil || a.Body != want {
			t.Errorf("Convert(%q) = %+v, want %s", q, a, want)
		}
	}
	for _, q := range []string{"5 km in kg", "km in mi", "5 parsecs in mi", "convert 5 km to mi"} {
		if a := Convert(q); a != nil {
			t.Errorf("Convert(%q) = %+v, want nil", q, a)
		}
	}
}

type stub struct {
	a   *Answer
	err error
}

func (s stub) Answer(context.Context, string) (*Answer, error) { return s.a, s.err }

func TestFindPrefersBuiltinsThenFirstWorkingProvider(t *testing.T) {
	ps := []Provider{stub{err: errors.New("boom")}, stub{a: &Answer{Body: "plugin"}}}
	if a := Find(context.Background(), "1+1", ps); a == nil || a.Source != "calculator" {
		t.Errorf("builtin = %+v", a)
	}
	if a := Find(context.Background(), "weather", ps); a == nil || a.Body != "plugin" {
		t.Errorf("provider = %+v", a)
	}
	if a := Find(context.Background(), "weather", nil); a != nil {
		t.Errorf("none = %+v", a)
	}
}
