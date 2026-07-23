package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/livereview/internal/jobqueue"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func main() {
	_ = godotenv.Load("../.env")
	dbURL := os.Getenv("DATABASE_URL")
	
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		panic(err)
	}

	args := jobqueue.ToolReviewOrchestratorJobArgs{
		ReviewID:        395, 
		OrgID:           3,
		PRURL:           "https://github.com/Amazing-Stardom/ai-industrial-safety/pull/5",
		ConnectorID:     8,
		Provider:        "github",
		TotalMultiplier: 1.0,
	}

	_, err = riverClient.Insert(ctx, args, nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Successfully enqueued ToolReviewOrchestratorJobArgs for Review %d!\n", args.ReviewID)
}
