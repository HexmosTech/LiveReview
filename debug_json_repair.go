package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/livereview/internal/llm"
)

func main() {
	malformedJSON := `{
		// Comment here
		comments: [
			{'file': 'test.go', 'line': 10,}, // Another comment
		]
	}`

	fmt.Println("Original:")
	fmt.Println(malformedJSON)
	fmt.Println("\n" + strings.Repeat("=", 50))

	repaired, stats, err := llm.RepairJSON(malformedJSON)

	fmt.Printf("Repaired (error: %v):\n", err)
	fmt.Println(repaired)
	fmt.Println("\n" + strings.Repeat("=", 50))

	fmt.Printf("Stats: %+v\n", stats)

	// Test if it's valid JSON
	var result interface{}
	parseErr := json.Unmarshal([]byte(repaired), &result)
	fmt.Printf("Parse error: %v\n", parseErr)

	if parseErr == nil {
		fmt.Printf("Parsed result: %+v\n", result)
	}
}
