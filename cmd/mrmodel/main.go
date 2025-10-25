package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	if len(os.Args) <= 1 {
		printUsage()
		return
	}

	subcommand := os.Args[1]
	switch subcommand {
	case "dump-db":
		if _, err := dumpIntegrationTokens(); err != nil {
			log.Fatalf("failed to dump integration_tokens: %v", err)
		}
	case "github":
		if err := runGitHub(os.Args[2:]); err != nil {
			log.Fatalf("github command failed: %v", err)
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
}
