package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
	"github.com/livereview/internal/blobstore"
)

type jobItem struct {
	reviewID       int64
	orgID          int64
	preloadedBytes []byte
}

func main() {
	dbURLFlag := flag.String("db", "", "PostgreSQL connection string (default: DATABASE_URL env)")
	workersFlag := flag.Int("workers", 20, "Number of concurrent upload workers")
	dryRunFlag := flag.Bool("dry-run", false, "Simulate migration without modifying DB or Blob Storage")
	modeFlag := flag.String("mode", "full", "Migration mode: 'copy' (copy to blob storage), 'clean' (strip preloaded_changes from DB after verification), or 'full' (copy and clean)")
	flag.Parse()

	dbURL := *dbURLFlag
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		log.Fatalf("[FATAL] Database connection string missing. Provide -db flag or set DATABASE_URL environment variable.\nExample: go run scripts/migrate_diffs_to_blobstore.go -db=\"postgres://user:password@localhost:5432/dbname?sslmode=disable\"")
	}

	mode := *modeFlag
	if mode != "copy" && mode != "clean" && mode != "full" {
		log.Fatalf("[FATAL] Invalid mode %q. Allowed modes: 'copy', 'clean', 'full'", mode)
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("[FATAL] Failed to connect to database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(*workersFlag + 5)
	db.SetMaxIdleConns(*workersFlag + 5)

	if err := db.Ping(); err != nil {
		log.Fatalf("[FATAL] Database ping failed: %v", err)
	}

	ctx := context.Background()

	// Verify Blob Store is reachable and reuse bucket across workers
	bucket, err := blobstore.OpenBucketFromDB(ctx, db)
	if err != nil {
		log.Fatalf("[FATAL] Failed to open configured blob store: %v", err)
	}
	defer bucket.Close()

	log.Printf("[MIGRATION] Successfully connected to PostgreSQL & Blob Store backend. Mode: %s", mode)

	// Count total reviews needing migration for progress reporting
	var totalCount int64
	countQuery := `SELECT COUNT(*) FROM reviews WHERE metadata ? 'preloaded_changes'`
	if err := db.QueryRowContext(ctx, countQuery).Scan(&totalCount); err != nil {
		log.Fatalf("[FATAL] Count query failed: %v", err)
	}

	log.Printf("[MIGRATION] Found %d reviews containing preloaded_changes in metadata.", totalCount)

	if totalCount == 0 {
		log.Printf("[MIGRATION] No reviews require migration. Done!")
		return
	}

	if *dryRunFlag {
		log.Printf("[DRY-RUN] Dry run complete. Would process %d reviews using %d workers in mode %q.", totalCount, *workersFlag, mode)
		return
	}

	// Producer-consumer channel bounded to (workers * 2) to prevent loading all rows into RAM at once (OOM prevention)
	bufferSize := *workersFlag * 2
	jobChan := make(chan jobItem, bufferSize)

	// Producer goroutine streams rows from DB into channel
	go func() {
		defer close(jobChan)

		query := `
			SELECT id, COALESCE(org_id, 0), metadata->'preloaded_changes'
			FROM reviews
			WHERE metadata ? 'preloaded_changes'
			ORDER BY id ASC
		`

		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			log.Fatalf("[FATAL] Query failed: %v", err)
		}
		defer rows.Close()

		for rows.Next() {
			var item jobItem
			var rawData []byte
			if err := rows.Scan(&item.reviewID, &item.orgID, &rawData); err != nil {
				log.Printf("[ERROR] Scan error for row: %v", err)
				continue
			}
			item.preloadedBytes = rawData
			jobChan <- item
		}
		if err := rows.Err(); err != nil {
			log.Printf("[ERROR] Rows iteration error: %v", err)
		}
	}()

	var migratedCount int64
	var failedCount int64
	startTime := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < *workersFlag; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobChan {
				key := blobstore.DiffReviewArtifactBlobKey(job.orgID, job.reviewID, blobstore.ArtifactPreloadedChanges)

				// 1. Copy phase: Write diff payload to Blob Storage if in copy or full mode
				if mode == "copy" || mode == "full" {
					if err := bucket.WriteAll(ctx, key, job.preloadedBytes, nil); err != nil {
						log.Printf("[WARN] [Worker %d] Failed to write review %d (org %d) to Blob Storage: %v", workerID, job.reviewID, job.orgID, err)
						atomic.AddInt64(&failedCount, 1)
						continue
					}
				}

				// 2. Clean phase: Remove preloaded_changes from metadata in Postgres if in clean or full mode
				if mode == "clean" || mode == "full" {
					// In clean mode, verify object exists in Blob Storage first before deleting from DB
					if mode == "clean" {
						if _, err := bucket.Attributes(ctx, key); err != nil {
							log.Printf("[ERROR] [Worker %d] Cannot clean review %d: diff artifact missing from Blob Storage: %v", workerID, job.reviewID, err)
							atomic.AddInt64(&failedCount, 1)
							continue
						}
					}

					updateQuery := `UPDATE reviews SET metadata = metadata - 'preloaded_changes' WHERE id = $1`
					if _, err := db.ExecContext(ctx, updateQuery, job.reviewID); err != nil {
						log.Printf("[ERROR] [Worker %d] Failed to strip preloaded_changes from DB for review %d: %v", workerID, job.reviewID, err)
						if mode == "full" {
							// Rollback written blob in full mode to keep DB and storage consistent
							if delErr := bucket.Delete(ctx, key); delErr != nil && !blobstore.IsNotExist(delErr) {
								log.Printf("[WARN] [Worker %d] Failed to cleanup orphaned blob %s after DB update failure: %v", workerID, key, delErr)
							}
						}
						atomic.AddInt64(&failedCount, 1)
						continue
					}
				}

				curr := atomic.AddInt64(&migratedCount, 1)
				if curr%1000 == 0 || curr == totalCount {
					elapsed := time.Since(startTime)
					var rate float64
					var remaining time.Duration
					if elapsed.Seconds() > 0 {
						rate = float64(curr) / elapsed.Seconds()
						if rate > 0 {
							remaining = time.Duration(float64(totalCount-curr)/rate) * time.Second
						}
					}
					log.Printf("[PROGRESS] Processed %d/%d reviews (%.1f%%) | Speed: %.1f reviews/sec | ETA: %s",
						curr, totalCount, float64(curr)/float64(totalCount)*100, rate, remaining.Round(time.Second))
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	log.Printf("==========================================================================")
	log.Printf("MIGRATION COMPLETE (Mode: %s)", mode)
	log.Printf("==========================================================================")
	log.Printf("Total Reviews Processed : %d", totalCount)
	log.Printf("Successfully Processed  : %d", migratedCount)
	log.Printf("Failed                  : %d", failedCount)
	log.Printf("Total Time Elapsed      : %s", duration.Round(time.Millisecond))
	if duration.Seconds() > 0 {
		log.Printf("Average Rate            : %.1f reviews/sec", float64(migratedCount)/duration.Seconds())
	}
	log.Printf("==========================================================================")
}
