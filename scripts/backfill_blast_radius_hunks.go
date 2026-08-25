// One-off backfill: computes and stores blast_radius_hunks rows for every
// blast-radius.json artifact already sitting in blob storage from before
// PutDiffReviewArtifact started replicating to Postgres (see
// docs/blast-radius-backend-port-plan.md). Safe to re-run - each review's
// rows are fully replaced (see storage/blastradius.ReplaceHunksForReview),
// so a partial run can just be run again.
//
// Enumerates via *blob.Bucket's native List (gocloud.dev/blob) rather than
// shelling out to the aws CLI - no extra dependency, and it works against
// whichever backend internal/blobstore is configured for (S3, GCS, Azure,
// local filesystem), not just S3.
//
// Usage: go run scripts/backfill_blast_radius_hunks.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"regexp"
	"strconv"
	"sync"

	"github.com/livereview/internal/blastradius"
	"github.com/livereview/internal/blobstore"
	"github.com/livereview/internal/database"
	storageblastradius "github.com/livereview/storage/blastradius"
	"gocloud.dev/blob"
)

// keyPattern matches diffReviewArtifactBlobKey's shape
// (internal/api/diff_review.go): org/<org_id>/review/<review_id>/artifacts/blast-radius.json
var keyPattern = regexp.MustCompile(`^org/(\d+)/review/(\d+)/artifacts/blast-radius\.json$`)

func main() {
	ctx := context.Background()

	db, err := database.NewDB()
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer db.Close()

	var blobCfgJSON []byte
	err = db.QueryRow(`SELECT data FROM system_settings WHERE name = 'blob_storage'`).Scan(&blobCfgJSON)
	if err != nil {
		log.Fatalf("load blob_storage settings: %v", err)
	}
	var cfg blobstore.Config
	if err := json.Unmarshal(blobCfgJSON, &cfg); err != nil {
		log.Fatalf("parse blob_storage settings: %v", err)
	}

	bucket, err := blobstore.OpenBucket(ctx, cfg)
	if err != nil {
		log.Fatalf("open bucket: %v", err)
	}
	defer bucket.Close()

	type target struct {
		key      string
		orgID    int64
		reviewID int64
	}
	var targets []target

	iter := bucket.List(&blob.ListOptions{Prefix: "org/"})
	for {
		obj, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("list bucket: %v", err)
		}
		m := keyPattern.FindStringSubmatch(obj.Key)
		if m == nil {
			continue
		}
		orgID, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			log.Printf("skip %s: bad org id: %v", obj.Key, err)
			continue
		}
		reviewID, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			log.Printf("skip %s: bad review id: %v", obj.Key, err)
			continue
		}
		targets = append(targets, target{key: obj.Key, orgID: orgID, reviewID: reviewID})
	}
	fmt.Printf("found %d blast-radius artifact(s)\n", len(targets))

	// Reviews whose id no longer exists would abort the insert on the FK -
	// filter those out up front instead of failing mid-run.
	existingReviews := make(map[int64]bool)
	rows, err := db.Query(`SELECT id FROM reviews`)
	if err != nil {
		log.Fatalf("load review ids: %v", err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			log.Fatalf("scan review id: %v", err)
		}
		existingReviews[id] = true
	}
	rows.Close()

	store := storageblastradius.NewStore(db)

	var mu sync.Mutex
	var replicated, skippedDeletedReview, skippedParseError, failed, totalHunks int

	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	for _, t := range targets {
		if !existingReviews[t.reviewID] {
			mu.Lock()
			skippedDeletedReview++
			mu.Unlock()
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(t target) {
			defer wg.Done()
			defer func() { <-sem }()

			raw, err := bucket.ReadAll(ctx, t.key)
			if err != nil {
				log.Printf("read %s: %v", t.key, err)
				mu.Lock()
				failed++
				mu.Unlock()
				return
			}
			var report blastradius.Report
			if err := json.Unmarshal(raw, &report); err != nil {
				log.Printf("parse %s: %v", t.key, err)
				mu.Lock()
				skippedParseError++
				mu.Unlock()
				return
			}
			var hunks []blastradius.HunkReport
			for _, f := range report.Files {
				hunks = append(hunks, f.Hunks...)
			}
			if err := store.ReplaceHunksForReview(ctx, t.orgID, t.reviewID, hunks); err != nil {
				log.Printf("replicate review %d: %v", t.reviewID, err)
				mu.Lock()
				failed++
				mu.Unlock()
				return
			}
			mu.Lock()
			replicated++
			totalHunks += len(hunks)
			mu.Unlock()
		}(t)
	}
	wg.Wait()

	fmt.Printf("done: %d review(s) replicated (%d hunks total), %d skipped (deleted review), %d skipped (parse error), %d failed\n",
		replicated, totalHunks, skippedDeletedReview, skippedParseError, failed)
}
