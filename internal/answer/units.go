package answer

import (
	"regexp"
	"strconv"
	"strings"
)

type unit struct {
	dim    string
	factor float64
}

var units = map[string]unit{}

func define(dim string, factor float64, names ...string) {
	for _, n := range names {
		units[n] = unit{dim, factor}
	}
}

func init() {
	define("length", 0.001, "mm", "millimeter", "millimeters", "millimetre", "millimetres")
	define("length", 0.01, "cm", "centimeter", "centimeters", "centimetre", "centimetres")
	define("length", 1, "m", "meter", "meters", "metre", "metres")
	define("length", 1000, "km", "kilometer", "kilometers", "kilometre", "kilometres")
	define("length", 0.0254, "in", "inch", "inches")
	define("length", 0.3048, "ft", "foot", "feet")
	define("length", 0.9144, "yd", "yard", "yards")
	define("length", 1609.344, "mi", "mile", "miles")
	define("length", 1852, "nmi")
	define("mass", 1e-6, "mg")
	define("mass", 1e-3, "g", "gram", "grams")
	define("mass", 1, "kg", "kilogram", "kilograms", "kilo", "kilos")
	define("mass", 1000, "t", "tonne", "tonnes")
	define("mass", 0.028349523125, "oz", "ounce", "ounces")
	define("mass", 0.45359237, "lb", "lbs", "pound", "pounds")
	define("mass", 6.35029318, "st", "stone")
	define("volume", 0.001, "ml", "milliliter", "milliliters")
	define("volume", 1, "l", "liter", "liters", "litre", "litres")
	define("volume", 3.785411784, "gal", "gallon", "gallons")
	define("speed", 1, "m/s")
	define("speed", 1/3.6, "km/h", "kmh", "kph")
	define("speed", 0.44704, "mph")
	define("speed", 1852/3600.0, "kn", "knot", "knots")
	define("time", 0.001, "ms")
	define("time", 1, "s", "sec", "secs", "second", "seconds")
	define("time", 60, "min", "mins", "minute", "minutes")
	define("time", 3600, "h", "hr", "hrs", "hour", "hours")
	define("time", 86400, "d", "day", "days")
	define("time", 604800, "week", "weeks")
	define("time", 31557600, "yr", "year", "years")
	define("data", 1, "b", "byte", "bytes")
	define("data", 1e3, "kb")
	define("data", 1e6, "mb")
	define("data", 1e9, "gb")
	define("data", 1e12, "tb")
	define("data", 1<<10, "kib")
	define("data", 1<<20, "mib")
	define("data", 1<<30, "gib")
	define("data", 1<<40, "tib")
	define("temperature", 0, "c", "°c", "celsius", "f", "°f", "fahrenheit", "k", "kelvin")
}

var convertRE = regexp.MustCompile(`^(-?\d+(?:\.\d+)?)\s*([a-z°/]+)\s+(?:in|to|as)\s+([a-z°/]+)$`)

// Convert answers "<number> <unit> in <unit>", e.g. "5 km in mi"; anything else returns nil.
func Convert(query string) *Answer {
	m := convertRE.FindStringSubmatch(strings.ToLower(strings.TrimSpace(query)))
	if m == nil {
		return nil
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	from, ok1 := units[m[2]]
	to, ok2 := units[m[3]]
	if !ok1 || !ok2 || from.dim != to.dim {
		return nil
	}
	var out float64
	if from.dim == "temperature" {
		out = fromKelvin(toKelvin(v, m[2]), m[3])
	} else {
		out = v * from.factor / to.factor
	}
	return &Answer{Title: m[1] + " " + m[2] + " in " + m[3], Body: format(out, 6) + " " + m[3], Source: "units"}
}

func toKelvin(v float64, name string) float64 {
	switch strings.TrimPrefix(name, "°")[0] {
	case 'c':
		return v + 273.15
	case 'f':
		return (v-32)*5/9 + 273.15
	}
	return v
}

func fromKelvin(v float64, name string) float64 {
	switch strings.TrimPrefix(name, "°")[0] {
	case 'c':
		return v - 273.15
	case 'f':
		return (v-273.15)*9/5 + 32
	}
	return v
}
