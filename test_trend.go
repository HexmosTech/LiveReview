//go:build ignore
// +build ignore

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/livereview/storage/reviews"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	store := reviews.NewTaxonomyReportStore(db)

	// Test with org 4, date range that has data
	f := reviews.TaxonomyFilter{
		OrgID: 4,
		Since: time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC),
	}

	fmt.Println("=== Testing GetTrend ===")
	rows, err := store.GetTrend(context.Background(), "day", f)
	if err != nil {
		log.Fatalf("GetTrend failed: %v", err)
	}

	fmt.Printf("Got %d rows from GetTrend\n", len(rows))
	for _, r := range rows {
		fmt.Printf("  Bucket: %s, Count: %d, ReviewCount: %d\n", r.Bucket, r.Count, r.ReviewCount)
	}

	// Simulate what the handler does
	fmt.Println("\n=== Simulating Handler Response ===")
	payload := map[string]interface{}{
		"grain": "day",
		"rows":  rows,
	}
	fmt.Printf("Payload keys: %v\n", getMapKeys(payload))
	if rowsVal, ok := payload["rows"]; ok {
		fmt.Printf("'rows' type: %T\n", rowsVal)
		fmt.Printf("'rows' value: %+v\n", rowsVal)
	}

	// Marshal to JSON to see what the client would receive
	jsonBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		log.Fatalf("JSON marshal failed: %v", err)
	}
	fmt.Printf("\nJSON output:\n%s\n", string(jsonBytes))

	// Parse back to verify
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonBytes, &parsed)
	if err != nil {
		log.Fatalf("JSON unmarshal failed: %v", err)
	}
	if rowsData, ok := parsed["rows"]; ok {
		fmt.Printf("\nParsed 'rows' type: %T\n", rowsData)
		if rowsArray, ok := rowsData.([]interface{}); ok {
			if len(rowsArray) > 0 {
				if firstRow, ok := rowsArray[0].(map[string]interface{}); ok {
					fmt.Printf("First row keys: %v\n", getMapKeys(firstRow))
					fmt.Printf("First row review_count: %v\n", firstRow["review_count"])
				}
			}
		}
	}

	fmt.Println("\n=== Testing GetSummary ===")
	summary, err := store.GetSummary(context.Background(), f)
	if err != nil {
		log.Fatalf("GetSummary failed: %v", err)
	}
	fmt.Printf("Summary: TotalFindings=%d, TotalReviews=%d\n", summary.TotalFindings, summary.TotalReviews)
}

func getMapKeys(m map[string]interface{}) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
