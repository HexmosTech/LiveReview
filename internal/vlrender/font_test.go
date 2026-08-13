package vlrender

import (
	"encoding/json"
	"testing"
)

// The powerbi theme's config.font is a bare "Segoe UI" with no fallback,
// which vl-convert's SVG renderer draws as invisible glyphs on a server
// without that font installed - the axis titles still show (their font
// setting has a real fallback stack) but every tick/legend/header label goes
// blank. NormalizeVegaLiteSpec must inject a safe font with fallbacks so this
// can't recur regardless of which theme is configured.
func TestNormalizeVegaLiteSpecInjectsSafeFonts(t *testing.T) {
	spec := `{"$schema":"https://vega.github.io/schema/vega-lite/v5.json","mark":"bar",
		"data":{"values":[{"x":"a","y":1}]},
		"encoding":{"x":{"field":"x","type":"nominal"},"y":{"field":"y","type":"quantitative"}}}`

	out, err := NormalizeVegaLiteSpec([]byte(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	config, ok := m["config"].(map[string]any)
	if !ok {
		t.Fatal("no config block was injected")
	}
	if config["font"] != safeFontStack {
		t.Fatalf("config.font = %v, want %q", config["font"], safeFontStack)
	}
	for _, section := range []string{"axis", "legend", "header"} {
		sub, ok := config[section].(map[string]any)
		if !ok {
			t.Fatalf("config.%s was not created", section)
		}
		if sub["labelFont"] != safeFontStack {
			t.Fatalf("config.%s.labelFont = %v, want %q", section, sub["labelFont"], safeFontStack)
		}
	}
	textCfg, ok := config["text"].(map[string]any)
	if !ok || textCfg["font"] != safeFontStack {
		t.Fatalf("config.text.font not set to %q: %v", safeFontStack, config["text"])
	}
}

// A spec that already picked its own fonts must not be overridden - only the
// gap left by the theme's missing fallback should be filled in.
func TestNormalizeVegaLiteSpecPreservesExplicitFonts(t *testing.T) {
	spec := `{"$schema":"https://vega.github.io/schema/vega-lite/v5.json","mark":"bar",
		"config":{"font":"Custom Font","axis":{"labelFont":"Custom Axis Font"}},
		"data":{"values":[{"x":"a","y":1}]},
		"encoding":{"x":{"field":"x","type":"nominal"},"y":{"field":"y","type":"quantitative"}}}`

	out, err := NormalizeVegaLiteSpec([]byte(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	config := m["config"].(map[string]any)
	if config["font"] != "Custom Font" {
		t.Fatalf("explicit config.font was overridden: got %v", config["font"])
	}
	axis := config["axis"].(map[string]any)
	if axis["labelFont"] != "Custom Axis Font" {
		t.Fatalf("explicit config.axis.labelFont was overridden: got %v", axis["labelFont"])
	}
	// legend was never set by the spec, so it should still get the fallback.
	legend, ok := config["legend"].(map[string]any)
	if !ok || legend["labelFont"] != safeFontStack {
		t.Fatalf("config.legend.labelFont was not filled in: %v", config["legend"])
	}
}

// Multi-chart ("reports") specs go through the same normalization per-report
// in the caller, so this just confirms the function itself works on a nested
// spec shape (a "spec" wrapper is common when specs travel inside a report).
func TestNormalizeVegaLiteSpecHandlesWrappedSpec(t *testing.T) {
	spec := `{"title":"t","spec":{"$schema":"https://vega.github.io/schema/vega-lite/v5.json","mark":"bar",
		"data":{"values":[{"x":"a","y":1}]},
		"encoding":{"x":{"field":"x","type":"nominal"},"y":{"field":"y","type":"quantitative"}}}}`

	out, err := NormalizeVegaLiteSpec([]byte(spec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	// The top-level wrapper gets a config block too (harmless - Vega-Lite
	// ignores unknown top-level keys like "title"/"spec" wrappers here since
	// this helper is only ever called with the inner raw spec in practice,
	// but this guards against a caller passing the wrapper by mistake).
	if _, ok := m["config"]; !ok {
		t.Fatal("no config block was injected on the outer object")
	}
}
