package chatexport

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"
)

func fixtureDoc(t *testing.T, includeDebug bool) *ExportDoc {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatalf("encode fixture png: %v", err)
	}

	doc := &ExportDoc{
		Conversation: ExportConversation{
			Title:     "Billing status Q1",
			CreatedAt: time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 1, 5, 10, 5, 0, 0, time.UTC),
		},
		Turns: []ExportTurn{
			{
				Seq:  1,
				Role: "user",
				Text: "What's our current billing status\nand total usage?",
			},
			{
				Seq:  2,
				Role: "assistant",
				Text: "Here is the breakdown for [Q1] usage.",
				Charts: []ExportChart{
					{Title: "Usage by [team]", Description: "Weekly totals", PNG: pngBuf.Bytes()},
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

func TestRenderPDF(t *testing.T) {
	doc := fixtureDoc(t, true)
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

func TestRenderHTML(t *testing.T) {
	doc := fixtureDoc(t, true)
	var buf bytes.Buffer
	if err := RenderHTML(doc, &buf); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	out := buf.String()
	if out == "" {
		t.Fatal("RenderHTML produced empty output")
	}
	for _, want := range []string{
		`id="turn-1"`,
		`id="turn-2"`,
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
	doc := fixtureDoc(t, false)
	var buf bytes.Buffer
	if err := RenderHTML(doc, &buf); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	if strings.Contains(buf.String(), "Debug artifacts") {
		t.Error("RenderHTML must not include a debug section when DebugArtifacts is unset")
	}
}
