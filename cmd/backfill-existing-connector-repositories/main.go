// Command backfill-existing-connector-repositories is a one-time maintenance
// script that populates the repositories (and queues pull_requests) tables
// for every GitHub/GitLab connector that already existed before that feature
// shipped.
//
// Root cause this fixes: repositories/pull_requests are only ever populated
// by AutoWebhookInstaller.syncRepositoriesAndQueueBackfill, which is only
// triggered at connector-creation time. Connectors created earlier have no
// retroactive path to get backfilled - the periodic reconciliation sweep
// only refreshes PR data for repos that already have a repositories row, it
// never discovers new ones for a connector. This script runs that same
// discovery+backfill logic for every existing connector.
//
// This makes real outbound GitHub/GitLab API calls using each connector's
// stored PAT (rate-limit usage; some old PATs may be stale/revoked - those
// connectors are logged and skipped, not fatal). Safe to re-run: repository
// upserts are idempotent.
//
// Usage:
//
//	go run ./cmd/backfill-existing-connector-repositories               # every github/gitlab connector
//	go run ./cmd/backfill-existing-connector-repositories -connector-id=25  # just one connector
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/livereview/internal/api"
	"github.com/livereview/internal/database"
	"github.com/livereview/internal/jobqueue"
)

func main() {
	connectorID := flag.Int("connector-id", 0, "Only backfill this connector (integration_tokens.id). Omit to backfill every github/gitlab connector.")
	flag.Parse()

	db, err := database.NewDB()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatalf("DATABASE_URL must be set (used to construct the job queue's connection pool)")
	}

	jq, err := jobqueue.NewJobQueue(dbURL, db)
	if err != nil {
		log.Fatalf("failed to initialize job queue: %v", err)
	}

	installer := api.NewAutoWebhookInstaller(db, nil, jq)

	ctx := context.Background()
	if *connectorID > 0 {
		if err := installer.BackfillConnector(ctx, *connectorID); err != nil {
			log.Fatalf("backfill failed for connector %d: %v", *connectorID, err)
		}
	} else {
		if err := installer.BackfillAllConnectors(ctx); err != nil {
			log.Fatalf("backfill failed: %v", err)
		}
	}

	log.Println("Backfill complete. Repository PR/MR data will populate shortly as the queued sync jobs run (requires the worker process to be running).")
}
