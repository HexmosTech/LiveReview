package chatexport

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func fixturePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fixture png: %v", err)
	}
	return buf.Bytes()
}

func fixtureExportDoc(t *testing.T, title string, includeDebug bool) *ExportDoc {
	t.Helper()
	doc := &ExportDoc{
		Conversation: ExportConversation{
			Title:     title,
			CreatedAt: time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 1, 5, 10, 5, 0, 0, time.UTC),
		},
		Turns: []ExportTurn{
			{
				Seq:       1,
				Role:      "user",
				CreatedAt: time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC),
				Text:      "What's our current billing status\nand total usage?",
			},
			{
				Seq:       2,
				Role:      "assistant",
				CreatedAt: time.Date(2026, 1, 5, 10, 5, 0, 0, time.UTC),
				Text:      "Here is the breakdown for [Q1] usage.",
				Charts: []ExportChart{
					{Title: "Usage by [team]", Description: "Weekly totals", PNG: fixturePNG(t)},
				},
				Files: []ExportFile{
					{Filename: "usage.csv", Kind: "csv", Rows: 42},
				},
			},
		},
	}
	if includeDebug {
		doc.Turns[1].DebugArtifacts = []byte(`{"query":"select 1","rows":1}`)
	}
	return doc
}

func fixtureCompiledDoc(t *testing.T, n int, includeDebug bool) *CompiledDoc {
	t.Helper()
	doc := &CompiledDoc{Title: "Q1 Review", Subtitle: "Compiled for the board"}
	for i := 0; i < n; i++ {
		doc.Conversations = append(doc.Conversations, *fixtureExportDoc(t, "Conversation "+string(rune('A'+i)), includeDebug))
	}
	return doc
}

func TestRenderPDF(t *testing.T) {
	doc := fixtureCompiledDoc(t, 1, true)
	var buf bytes.Buffer
	if err := RenderPDF(context.Background(), doc, &buf); err != nil {
		t.Fatalf("RenderPDF: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("RenderPDF produced empty output")
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		n := 16
		if buf.Len() < n {
			n = buf.Len()
		}
		t.Fatalf("RenderPDF output does not look like a PDF, starts with %q", buf.Bytes()[:n])
	}
}

func TestRenderPDF_CompiledMultipleConversationsPageBreakAndBookmarks(t *testing.T) {
	doc := fixtureCompiledDoc(t, 2, false)
	var buf bytes.Buffer
	if err := RenderPDF(context.Background(), doc, &buf); err != nil {
		t.Fatalf("RenderPDF: %v", err)
	}

	pageCount := len(regexp.MustCompile(`/Type\s*/Page\b`).FindAll(buf.Bytes(), -1))
	if pageCount < 2 {
		t.Errorf("expected at least 2 pages for 2 compiled conversations, got %d", pageCount)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/Outlines")) {
		t.Error("expected a /Outlines dictionary (PDF bookmarks sidebar) in the output")
	}

	// gofpdf UTF-16BE-encodes a Bookmark's /Title whenever the last-set font
	// was added via AddUTF8FontFromBytes - true throughout this document
	// (see registerFonts/exportStyles, which embed Liberation Sans/Mono
	// specifically so typographic punctuation in real message text renders
	// correctly instead of as mojibake). Unlike the page content streams
	// (Flate compressed, not substring-searchable this way), bookmark
	// objects are never compressed, so searching for the precomputed
	// UTF-16BE encoding is a reliable, direct check.
	for _, want := range []string{"Q1 Review", "Conversation A", "Conversation B", "Turn 1", "Turn 2"} {
		if !bytes.Contains(buf.Bytes(), utf16BEText(want)) {
			t.Errorf("expected the PDF's /Outlines bookmark titles to contain %q", want)
		}
	}
}

// utf16BEText encodes s as PDF text strings encode it (UTF-16BE, no BOM
// once inside the search window) - used to confirm a bookmark title landed
// in the output without needing to parse PDF /Title object syntax back out
// (Go's regexp, unlike Python's, does not reliably match arbitrary binary
// byte runs with "." under invalid-UTF-8 input, so byte-substring search
// against a precomputed encoding is the robust check here).
func utf16BEText(s string) []byte {
	var buf bytes.Buffer
	for _, r := range utf16.Encode([]rune(s)) {
		buf.WriteByte(byte(r >> 8))
		buf.WriteByte(byte(r))
	}
	return buf.Bytes()
}

func TestRenderHTML(t *testing.T) {
	doc := fixtureCompiledDoc(t, 1, true)
	var buf bytes.Buffer
	if err := RenderHTML(doc, &buf); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	out := buf.String()
	if out == "" {
		t.Fatal("RenderHTML produced empty output")
	}
	for _, want := range []string{
		`id="conversation-1"`,
		`id="conversation-1-turn-1"`,
		`id="conversation-1-turn-2"`,
		`data:image/png;base64,`,
		"Debug artifacts",
		"usage.csv",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderHTML output missing %q", want)
		}
	}
	if strings.Contains(out, "<script") {
		t.Error("RenderHTML output must never contain a <script tag from message content")
	}
}

func TestRenderHTML_ExcludesDebugArtifactsWhenNotRequested(t *testing.T) {
	doc := fixtureCompiledDoc(t, 1, false)
	var buf bytes.Buffer
	if err := RenderHTML(doc, &buf); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	if strings.Contains(buf.String(), "Debug artifacts") {
		t.Error("RenderHTML must not include a debug section when DebugArtifacts is unset")
	}
}

func TestRenderHTML_CompiledMultipleConversations(t *testing.T) {
	doc := fixtureCompiledDoc(t, 2, false)
	var buf bytes.Buffer
	if err := RenderHTML(doc, &buf); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	out := buf.String()

	if got := strings.Count(out, `class="conversation"`); got != 2 {
		t.Errorf("expected 2 conversation sections, got %d", got)
	}
	for _, want := range []string{
		"Conversation 1 — Conversation A",
		"Conversation 2 — Conversation B",
		`id="conversation-1-turn-1"`,
		`id="conversation-2-turn-1"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderHTML compiled output missing %q", want)
		}
	}
}

func TestDescribeChartShape(t *testing.T) {
	cases := []struct {
		name string
		spec string
		want string
	}{
		{"bare mark", `{"mark":"bar"}`, "bar"},
		{"object mark", `{"mark":{"type":"line","point":true}}`, "line"},
		{"layered", `{"layer":[{"mark":"bar"},{"mark":{"type":"line"}}]}`, "layered (bar + line)"},
		{"faceted", `{"facet":{"field":"team"},"spec":{"mark":"point"}}`, "faceted (point)"},
		{"missing mark", `{}`, "unknown"},
		{"invalid json", `not json`, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DescribeChartShape([]byte(tc.spec))
			if got != tc.want {
				t.Errorf("DescribeChartShape(%s) = %q, want %q", tc.spec, got, tc.want)
			}
		})
	}
}
