// One-off backfill: computes internal/chatstats KPI chips for every
// chat_charts row persisted before that computation moved server-side (see
// db/migrations/20260824190000_add_chat_charts_stats.sql). Safe to re-run -
// it only touches rows where stats IS NULL, so a partial run (or a chart
// shape chatstats doesn't recognize, which stays NULL on purpose) can be
// resumed by running it again.
//
// Usage: go run scripts/backfill_chat_chart_stats.go
package main

import (
	"fmt"
	"log"

	"github.com/livereview/internal/chatstats"
	"github.com/livereview/internal/database"
)

func main() {
	db, err := database.NewDB()
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT id, vega_spec FROM chat_charts WHERE stats IS NULL ORDER BY id`)
	if err != nil {
		log.Fatalf("select chat_charts: %v", err)
	}

	type pending struct {
		id   int64
		spec []byte
	}
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.spec); err != nil {
			log.Fatalf("scan: %v", err)
		}
		todo = append(todo, p)
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("rows: %v", err)
	}
	rows.Close()

	fmt.Printf("found %d chat_charts row(s) with no stats\n", len(todo))

	var updated, skipped, failed int
	for _, p := range todo {
		stats, err := chatstats.ComputeAllStats(p.spec)
		if err != nil {
			log.Printf("chart %d: compute failed: %v", p.id, err)
			failed++
			continue
		}
		if stats == nil {
			// Not a shape chatstats recognizes (e.g. layered/faceted) - leave
			// NULL, same as a chart built after this shipped would get.
			skipped++
			continue
		}
		if _, err := db.Exec(`UPDATE chat_charts SET stats = $1 WHERE id = $2`, []byte(stats), p.id); err != nil {
			log.Printf("chart %d: update failed: %v", p.id, err)
			failed++
			continue
		}
		updated++
	}

	fmt.Printf("done: %d updated, %d skipped (unrecognized shape), %d failed\n", updated, skipped, failed)
}
