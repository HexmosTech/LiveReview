package aisanitize

import (
	"context"
	"strings"
	"testing"
)

func TestSanitizationPostflight_NeutralizesRawHTML(t *testing.T) {
	input := "Please avoid <script>alert('x')</script> in output"
	out, _ := SanitizationPostflight(context.Background(), input)

	if strings.Contains(strings.ToLower(out), "<script") {
		t.Fatalf("expected raw html tags to be neutralized, got: %s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Fatalf("expected escaped script tag marker, got: %s", out)
	}
}

func TestSanitizationPostflight_NeutralizesUnsafeMarkdownLink(t *testing.T) {
	input := "Click [here](javascript:alert(1)) for details"
	out, _ := SanitizationPostflight(context.Background(), input)

	if strings.Contains(strings.ToLower(out), "javascript:") {
		t.Fatalf("expected unsafe javascript scheme to be removed, got: %s", out)
	}
	if !strings.Contains(out, "[here](#)") {
		t.Fatalf("expected unsafe markdown link to be neutralized, got: %s", out)
	}
}

func TestSanitizationPostflight_PreservesSafeMarkdownLink(t *testing.T) {
	input := "See [docs](https://example.com/security)"
	out, _ := SanitizationPostflight(context.Background(), input)

	if out != input {
		t.Fatalf("expected safe markdown link to remain unchanged, got: %s", out)
	}
}

func TestSanitizationPostflight_PreservesSafeMarkdownLinkWithNestedParentheses(t *testing.T) {
	input := "Read [guide](https://example.com/path_(nested)/index.html)"
	out, _ := SanitizationPostflight(context.Background(), input)

	if out != input {
		t.Fatalf("expected safe markdown link with nested parentheses to remain unchanged, got: %s", out)
	}
}

func TestSanitizationPostflight_NeutralizesUnsafeMarkdownLinkWithWhitespaceLabel(t *testing.T) {
	input := "Use [bad label text](javascript:alert(1))"
	out, _ := SanitizationPostflight(context.Background(), input)

	if strings.Contains(strings.ToLower(out), "javascript:") {
		t.Fatalf("expected javascript link to be removed, got: %s", out)
	}
	if !strings.Contains(out, "[bad label text](#)") {
		t.Fatalf("expected unsafe markdown link with whitespace label to be neutralized, got: %s", out)
	}
}
