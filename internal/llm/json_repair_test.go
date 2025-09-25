package llm

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestRepairJSON_ValidJSON(t *testing.T) {
	validJSON := `{"comments": [{"file": "test.go", "line": 10, "comment": "Good code"}]}`

	repaired, stats, err := RepairJSON(validJSON)

	if err != nil {
		t.Errorf("Expected no error for valid JSON, got: %v", err)
	}

	if stats.WasRepaired {
		t.Error("Expected WasRepaired to be false for valid JSON")
	}

	if repaired != validJSON {
		t.Error("Expected repaired JSON to be identical to original for valid JSON")
	}

	if stats.OriginalBytes != len(validJSON) || stats.RepairedBytes != len(validJSON) {
		t.Error("Expected byte counts to match original")
	}
}

func TestRepairJSON_TrailingCommas(t *testing.T) {
	malformedJSON := `{"comments": [{"file": "test.go", "line": 10,}]}`
	expected := `{"comments": [{"file": "test.go", "line": 10}]}`

	repaired, stats, err := RepairJSON(malformedJSON)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if !stats.WasRepaired {
		t.Error("Expected WasRepaired to be true")
	}

	if repaired != expected {
		t.Errorf("Expected %s, got %s", expected, repaired)
	}

	if stats.ErrorsFixed != 1 {
		t.Errorf("Expected 1 error fixed, got %d", stats.ErrorsFixed)
	}

	if len(stats.RepairStrategies) == 0 || stats.RepairStrategies[0] != "trailing_commas" {
		t.Error("Expected trailing_commas repair strategy")
	}
}

func TestRepairJSON_IncompleteObject(t *testing.T) {
	malformedJSON := `{"comments": [{"file": "test.go", "line": 10}`
	expected := `{"comments": [{"file": "test.go", "line": 10}]}`

	repaired, stats, err := RepairJSON(malformedJSON)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if !stats.WasRepaired {
		t.Error("Expected WasRepaired to be true")
	}

	if repaired != expected {
		t.Errorf("Expected %s, got %s", expected, repaired)
	}

	// Verify it's valid JSON
	var result interface{}
	if json.Unmarshal([]byte(repaired), &result) != nil {
		t.Error("Repaired JSON should be valid")
	}
}

func TestRepairJSON_Comments(t *testing.T) {
	malformedJSON := `{
		// This is a comment
		"comments": [
			{"file": "test.go", "line": 10} /* inline comment */
		]
	}`

	repaired, stats, err := RepairJSON(malformedJSON)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if !stats.WasRepaired {
		t.Error("Expected WasRepaired to be true")
	}

	if stats.CommentsLost != 2 {
		t.Errorf("Expected 2 comments lost, got %d", stats.CommentsLost)
	}

	// Verify it's valid JSON
	var result interface{}
	if json.Unmarshal([]byte(repaired), &result) != nil {
		t.Error("Repaired JSON should be valid")
	}
}

func TestRepairJSON_UnquotedKeys(t *testing.T) {
	malformedJSON := `{comments: [{"file": "test.go", line: 10}]}`

	repaired, stats, err := RepairJSON(malformedJSON)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if !stats.WasRepaired {
		t.Error("Expected WasRepaired to be true")
	}

	// Verify it's valid JSON
	var result interface{}
	if json.Unmarshal([]byte(repaired), &result) != nil {
		t.Error("Repaired JSON should be valid")
	}
}

func TestRepairJSON_SingleQuotes(t *testing.T) {
	malformedJSON := `{'comments': [{'file': 'test.go', 'line': 10}]}`

	repaired, stats, err := RepairJSON(malformedJSON)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if !stats.WasRepaired {
		t.Error("Expected WasRepaired to be true")
	}

	// Verify it's valid JSON
	var result interface{}
	if json.Unmarshal([]byte(repaired), &result) != nil {
		t.Error("Repaired JSON should be valid")
	}
}

func TestRepairJSON_MultipleStrategies(t *testing.T) {
	malformedJSON := `{
		// Comment here
		comments: [
			{'file': 'test.go', 'line': 10,}, // Another comment
		]
	}`

	repaired, stats, err := RepairJSON(malformedJSON)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if !stats.WasRepaired {
		t.Error("Expected WasRepaired to be true")
	}

	if len(stats.RepairStrategies) < 2 {
		t.Errorf("Expected multiple repair strategies, got %d", len(stats.RepairStrategies))
	}

	if stats.ErrorsFixed < 2 {
		t.Errorf("Expected multiple errors fixed, got %d", stats.ErrorsFixed)
	}

	// Verify it's valid JSON
	var result interface{}
	if json.Unmarshal([]byte(repaired), &result) != nil {
		t.Error("Repaired JSON should be valid")
	}
}

func TestRepairJSON_Performance(t *testing.T) {
	// Test performance with a larger JSON
	largeJSON := `{"comments": [`
	for i := 0; i < 100; i++ {
		if i > 0 {
			largeJSON += ","
		}
		largeJSON += fmt.Sprintf(`{"file": "test.go", "line": %d}`, i+10)
	}
	largeJSON += `]}`

	start := time.Now()
	repaired, stats, err := RepairJSON(largeJSON)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if duration > time.Millisecond*100 {
		t.Errorf("Repair took too long: %v", duration)
	}

	if stats.RepairTime > time.Millisecond*100 {
		t.Errorf("Reported repair time too long: %v", stats.RepairTime)
	}

	if repaired != largeJSON {
		t.Error("Valid JSON should not be modified")
	}
}

func TestRepairJSON_JsonRepairLibrary(t *testing.T) {
	// JSON that our custom strategies might miss but jsonrepair library can handle
	malformedJSON := `{
		"comments": [
			{"file": "test.go", "line": 10, "comment": "This has "embedded" quotes"},
			{"file": "other.go", "line": 20, comment: "unquoted value"}
		],
		"status": incomplete
	}`

	repaired, stats, err := RepairJSON(malformedJSON)

	if err != nil {
		t.Errorf("Expected successful repair with library fallback, got: %v", err)
	}

	if !stats.WasRepaired {
		t.Error("Expected WasRepaired to be true")
	}

	// Should use multiple strategies including the library
	hasLibraryStrategy := false
	for _, strategy := range stats.RepairStrategies {
		if strategy == "jsonrepair_library" {
			hasLibraryStrategy = true
			break
		}
	}

	if !hasLibraryStrategy {
		t.Error("Expected jsonrepair_library strategy to be used")
	}

	// Verify it's valid JSON
	var result interface{}
	if json.Unmarshal([]byte(repaired), &result) != nil {
		t.Error("Repaired JSON should be valid")
	}
}

func TestRepairJSON_RepairableWithLibrary(t *testing.T) {
	// Test that even plain text gets repaired by the library (wrapped in quotes)
	plainText := `this is just plain text with no structure whatsoever`

	repaired, stats, err := RepairJSON(plainText)

	if err != nil {
		t.Errorf("Expected library to repair plain text, got error: %v", err)
	}

	if !stats.WasRepaired {
		t.Error("Expected WasRepaired to be true")
	}

	// Should use the jsonrepair library strategy
	hasLibraryStrategy := false
	for _, strategy := range stats.RepairStrategies {
		if strategy == "jsonrepair_library" {
			hasLibraryStrategy = true
			break
		}
	}

	if !hasLibraryStrategy {
		t.Error("Expected jsonrepair_library strategy to be used for plain text")
	}

	// Verify it's valid JSON (should be wrapped in quotes)
	var result interface{}
	if json.Unmarshal([]byte(repaired), &result) != nil {
		t.Error("Repaired JSON should be valid")
	}
}
