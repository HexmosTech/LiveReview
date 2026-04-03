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

func TestSanitizationPostflight_PreservesComplexMarkdown(t *testing.T) {
	input := "# Refactor Blocking LED Control to State Machines\n\n" +
		"## Overview\n" +
		"This change converts blocking LED blink operations to non-blocking, state-machine-driven tasks. " +
		"It eliminates `delay()` calls that previously halted the task scheduler. " +
		"New tasks now manage LED states using `millis()` timers and dedicated update functions.\n\n" +
		"## Technical Highlights\n" +
		"- **mcu/basic/TaskExample/src/taskLibrary.cpp**: Shifted LED control from blocking `delay()` to non-blocking `millis()` state machines.\n" +
		"- **mcu/basic/TaskExample/src/taskLibrary.cpp**: Introduced new `Task` instances (`pulseTask`, `blink5Task`) for continuous LED state updates every 10ms.\n" +
		"- **mcu/basic/TaskExample/src/taskLibrary.cpp**: Implemented global state variables (`pulseActive`, `blink5Active`) and dedicated update functions to manage LED sequences.\n" +
		"- **mcu/basic/TaskExample/src/taskLibrary.cpp**: Split and renamed original `Callback` functions to `Callback25` and `Callback50` to trigger non-blocking sequences.\n\n" +
		"## Impact\n" +
		"- **Functionality**: The system can now execute multiple tasks concurrently without LED animations blocking the main loop.\n" +
		"- **Risk**: Increased complexity in state management requires thorough testing of all LED pattern interactions and transitions."
	out, _ := SanitizationPostflight(context.Background(), input)

	if out != input {
		t.Fatalf("expected input to be preserved during postflight, got: %s", out)
	}
}
