package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/livereview/internal/api"
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
				log.Printf("Loaded environment from %s", envFile)
			}

			// Load .env if not loaded already and exists
			_ = LoadEnvFile(".env")

			// Create version info from global variables
			versionInfo := &api.VersionInfo{
				Version:   Version,
				GitCommit: GitCommit,
				BuildTime: BuildTime,
				Dirty:     false,
			}

			// Create server instance optimized for background workers (no Echo routing initialized)
			log.Println("Initializing api worker context and job queue...")
			server, err := api.WorkerContext(versionInfo)
			if err != nil {
				return fmt.Errorf("failed to initialize worker server context: %w", err)
			}
			jq := server.GetJobQueue()

			// Handle OS signals for graceful shutdown
			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

			// Start the job queue workers
			log.Println("Starting background job queue workers...")
			workerCtx, workerCancel := context.WithCancel(context.Background())
			defer workerCancel()

			go func() {
				if err := jq.Start(workerCtx); err != nil {
					log.Printf("Error running job queue: %v", err)
					stop <- syscall.SIGTERM
				}
			}()

			log.Println("Background worker running. Press Ctrl+C to stop.")

			<-stop
			log.Println("Stopping background worker gracefully...")

			// Graceful shutdown with timeout
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()

			if err := jq.Stop(shutdownCtx); err != nil {
				log.Printf("Error stopping job queue: %v", err)
			}

			log.Println("Background worker stopped.")
			return nil
		},
	}
}

