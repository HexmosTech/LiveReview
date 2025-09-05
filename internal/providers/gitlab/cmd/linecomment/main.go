package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"

	gl "github.com/livereview/internal/providers/gitlab"
)

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing required env %s", k)
	}
	return v
}

// extract project path and IID from MR URL
func extractMR(mrURL string) (string, int, error) {
	re := regexp.MustCompile(`https?://[^/]+/([^/]+/[^/]+)/-/merge_requests/(\d+)`)
	m := re.FindStringSubmatch(mrURL)
	if len(m) != 3 {
		return "", 0, fmt.Errorf("could not parse MR URL: %s", mrURL)
	}
	iid, err := strconv.Atoi(m[2])
	if err != nil {
		return "", 0, err
	}
	return m[1], iid, nil
}

func main() {
	baseURL := mustEnv("GL_URL")
	token := mustEnv("GL_TOKEN")
	mrURL := mustEnv("MR_URL")
	filePath := mustEnv("FILE_PATH")
	lineStr := mustEnv("LINE")
	comment := os.Getenv("COMMENT")
	if comment == "" {
		comment = "Linecomment CLI: hello from Go"
	}
	side := os.Getenv("SIDE") // "old" or "new" optional

	line, err := strconv.Atoi(lineStr)
	if err != nil {
		log.Fatalf("invalid LINE: %v", err)
	}

	project, mrIID, err := extractMR(mrURL)
	if err != nil {
		log.Fatalf("extract MR: %v", err)
	}

	client := gl.NewHTTPClient(baseURL, token)

	// Prefer classification logic in CreateMRLineComment; pass flag only if SIDE=old
	isDeleted := false
	if side == "old" {
		isDeleted = true
	}

	if err := client.CreateMRLineComment(project, mrIID, filePath, line, comment, isDeleted); err != nil {
		log.Fatalf("CreateMRLineComment: %v", err)
	}
	fmt.Println("OK: comment created")
}
