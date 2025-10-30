package main

import (
	"fmt"
	"log"
	"os"

	"github.com/livereview/cmd/mrmodel/lib"
)

func main() {
	if len(os.Args) <= 1 {
		printUsage()
		return
	}

	subcommand := os.Args[1]
	switch subcommand {
	case "dump-db":
		if _, err := lib.DumpIntegrationTokens(); err != nil {
			log.Fatalf("failed to dump integration_tokens: %v", err)
		}
	case "github":
		if err := runGitHub(os.Args[2:]); err != nil {
			log.Fatalf("github command failed: %v", err)
		}
	case "bitbucket":
		if err := runBitbucket(os.Args[2:]); err != nil {
			log.Fatalf("bitbucket command failed: %v", err)
		}
	case "gitlab":
		if err := runGitLab(os.Args[2:]); err != nil {
			log.Fatalf("gitlab command failed: %v", err)
		}
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: go run ./cmd/mrmodel <command> [options]")
	fmt.Println("Commands:")
	fmt.Println("  dump-db             Dump integration_tokens as JSON")
	fmt.Println("  github              Export GitHub PR timeline/comment tree (see --help)")
	fmt.Println("  gitlab              Export GitLab MR timeline/comment tree (see --help)")
	fmt.Println("  bitbucket           Export Bitbucket PR timeline/comment tree (see --help)")
}
