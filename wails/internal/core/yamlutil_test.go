package core

import (
	"math"
	"strings"
	"testing"
)

// Whole numbers decoded from JSON arrive as float64; yaml.Marshal would render
// large ones in exponent notation (1024000 → "1.024e+06"). AnyToNode must put
// them back as integers.
func TestAnyToNodeRendersWholeNumbersAsIntegers(t *testing.T) {
	node, err := AnyToNode(map[string]any{
		"contextWindow": float64(1024000),
		"maxTokens":     float64(384000),
		"exactInt":      float64(1e+06),
		"ratio":         float64(1.5),
		"share":         float64(0.25),
		"zero":          float64(0),
		"negative":      float64(-2048),
		"nested":        map[string]any{"port": float64(8080)},
		"list":          []any{float64(128000), float64(2.5)},
	})
	if err != nil {
		t.Fatalf("AnyToNode: %v", err)
	}
	text, err := MarshalYAML(node)
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	if strings.Contains(text, "e+") {
		t.Errorf("exponent notation leaked into the YAML:\n%s", text)
	}
	for _, want := range []string{
		"contextWindow: 1024000",
		"maxTokens: 384000",
		"exactInt: 1000000",
		"ratio: 1.5",
		"share: 0.25",
		"zero: 0",
		"negative: -2048",
		"port: 8080",
		"- 128000",
		"- 2.5",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestJSONToNodeRendersWholeNumbersAsIntegers(t *testing.T) {
	node, err := JSONToNode([]byte(`{"contextWindow":1024000,"cost":{"input":0.5},"maxTokens":1e6}`))
	if err != nil {
		t.Fatalf("JSONToNode: %v", err)
	}
	text, err := MarshalYAML(node)
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	if strings.Contains(text, "e+") {
		t.Errorf("exponent notation leaked into the YAML:\n%s", text)
	}
	if !strings.Contains(text, "contextWindow: 1024000") || !strings.Contains(text, "maxTokens: 1000000") {
		t.Errorf("whole numbers not normalised:\n%s", text)
	}
	if !strings.Contains(text, "input: 0.5") {
		t.Errorf("fractional numbers must stay floats:\n%s", text)
	}
}

// Values that cannot round-trip through int64 (or are not finite) must be left
// untouched rather than silently mangled.
func TestNormalizeYAMLNumbersLeavesOutOfRangeValuesAlone(t *testing.T) {
	outOfRange := map[string]float64{
		"huge":  1e30,
		"tiny":  -1e30,
		"limit": 9223372036854775808, // 2^63, one past math.MaxInt64
		"nan":   math.NaN(),
		"inf":   math.Inf(1),
	}
	for name, value := range outOfRange {
		got := normalizeYAMLNumbers(value)
		if float, ok := got.(float64); ok && math.IsNaN(value) && math.IsNaN(float) {
			continue
		}
		if got != any(value) {
			t.Errorf("%s: got %v (%T), want the original float64", name, got, got)
		}
	}
	if got := normalizeYAMLNumbers(1.75); got != any(1.75) {
		t.Errorf("fractional value must stay a float64, got %v (%T)", got, got)
	}
}
