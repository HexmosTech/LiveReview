package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/livereview/internal/jobqueue"
	"github.com/livereview/internal/license"
	"github.com/urfave/cli/v2"
)

// WorkerCommand returns the CLI command for starting the River job queue worker
func WorkerCommand() *cli.Command {
	return &cli.Command{
		Name:  "worker",
		Usage: "Start the LiveReview background job queue worker",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "env-file",
				Usage: "Path to .env file to load (overwrites existing variables)",
			},
		},
		Action: func(c *cli.Context) error {
			if envFile := c.String("env-file"); envFile != "" {
				if err := LoadEnvFile(envFile); err != nil {
					return fmt.Errorf("failed to load env file %s: %w", envFile, err)
				}
				fmt.Printf("Loaded environment from %s\n", envFile)
			}

			// Load .env if not loaded already and exists
			_ = LoadEnvFile(".env")

			// Check database URL
			dbURL := os.Getenv("DATABASE_URL")
			if dbURL == "" {
				return fmt.Errorf("DATABASE_URL environment variable is not set")
			}

			// Open database connection
			db, err := sql.Open("postgres", dbURL)
			if err != nil {
				return fmt.Errorf("failed to open database connection: %w", err)
			}
			defer db.Close()

			// Ping database to verify connection
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := db.PingContext(ctx); err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}

			// Sync plan definitions
			planCatalogPath := strings.TrimSpace(os.Getenv("LIVEREVIEW_PLAN_CATALOG_PATH"))
			if planCatalogPath == "" {
				planCatalogPath = license.DefaultPlanCatalogPath
			}
			if err := license.SyncPlanDefinitionsFromCatalog(planCatalogPath); err != nil {
				return fmt.Errorf("failed to sync plan catalog from %s: %w", planCatalogPath, err)
			}

			// Initialize job queue
			fmt.Println("Initializing job queue...")
			jq, err := jobqueue.NewJobQueue(dbURL, db)
			if err != nil {
				return fmt.Errorf("failed to initialize job queue: %w", err)
			}

			// Handle OS signals for graceful shutdown
			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

			// Start the job queue workers
			fmt.Println("Starting background job queue workers...")
			workerCtx, workerCancel := context.WithCancel(context.Background())
			defer workerCancel()

			go func() {
				if err := jq.Start(workerCtx); err != nil {
					fmt.Printf("Error running job queue: %v\n", err)
					stop <- syscall.SIGTERM
				}
			}()

			fmt.Println("Background worker running. Press Ctrl+C to stop.")

			<-stop
			fmt.Println("Stopping background worker gracefully...")

			// Graceful shutdown with timeout
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()

			if err := jq.Stop(shutdownCtx); err != nil {
				fmt.Printf("Error stopping job queue: %v\n", err)
			}

			fmt.Println("Background worker stopped.")
			return nil
		},
	}
}
