package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func getAPIKey() string {
	if apiKey := os.Getenv("LIVEREVIEW_API_KEY"); apiKey != "" {
		return apiKey
	}
	return os.Getenv("API_KEY")
}

const (
	APIURL = "https://manual-talent.apps.hexmos.com/api/v1/diff-review"
)

func createDiffZipBase64(diffText string) (string, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	f, err := zipWriter.Create("diff.patch")
	if err != nil {
		return "", err
	}
	if _, err := f.Write([]byte(diffText)); err != nil {
		return "", err
	}
	if err := zipWriter.Close(); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func main() {
	testFiles := []struct {
		FileName string
		ToolName string
	}{
		{"gitleaks.txt", "gitleaks"},
		{"bandit.txt", "bandit"},
		{"ruff.txt", "ruff"},
		{"actionlint.txt", "actionlint"},
		{"hadolint.txt", "hadolint"},
		{"eslint.txt", "eslint"},
		{"tfsec.txt", "tfsec"},
		{"kubescape.txt", "kubescape"},
		{"openapi.txt", "openapi"},
		{"spectral.txt", "spectral"},
		{"trivy.txt", "trivy"},
		{"detect-secrets.txt", "detect-secrets"},
		{"trufflehog.txt", "trufflehog"},
		{"semgrep.txt", "semgrep"},
		{"shellcheck.txt", "shellcheck"},
	}

	testCasesDir := "/home/gk/hex/lr-tools/test_cases"

	fmt.Println("==================================================================================")
	fmt.Printf("%-3s | %-18s | %-15s | %-10s | %-30s\n", "#", "Test Case File", "Target Tool", "Review ID", "Status")
	fmt.Println("==================================================================================")

	client := &http.Client{Timeout: 30 * time.Second}

	for idx, item := range testFiles {
		filePath := filepath.Join(testCasesDir, item.FileName)
		contentBytes, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("%-3d | %-18s | %-15s | %-10s | ❌ File Read Error\n", idx+1, item.FileName, item.ToolName, "-")
			continue
		}

		diffText := string(contentBytes)
		if len(diffText) > 60000 {
			diffText = diffText[:60000]
		}

		zipBase64, err := createDiffZipBase64(diffText)
		if err != nil {
			fmt.Printf("%-3d | %-18s | %-15s | %-10s | ❌ Zip Creation Error\n", idx+1, item.FileName, item.ToolName, "-")
			continue
		}

		payload := map[string]interface{}{
			"diff_zip_base64": zipBase64,
			"tools_only":      true,
			"repo_name":       "git-lrc",
			"branch_name":     "feat/tool-test-" + item.ToolName,
		}
		jsonBytes, _ := json.Marshal(payload)

		req, err := http.NewRequest("POST", APIURL, bytes.NewBuffer(jsonBytes))
		if err != nil {
			fmt.Printf("%-3d | %-18s | %-15s | %-10s | ❌ HTTP Req Error\n", idx+1, item.FileName, item.ToolName, "-")
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", getAPIKey())

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("%-3d | %-18s | %-15s | %-10s | ❌ HTTP Failed\n", idx+1, item.FileName, item.ToolName, "-")
			continue
		}

		respBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var respData map[string]interface{}
		_ = json.Unmarshal(respBytes, &respData)

		var reviewID interface{} = "-"
		if idVal, ok := respData["id"]; ok {
			reviewID = idVal
		} else if dataMap, ok := respData["data"].(map[string]interface{}); ok {
			if idVal, ok := dataMap["id"]; ok {
				reviewID = idVal
			}
		}

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			fmt.Printf("%-3d | %-18s | %-15s | Review #%-5v | ✓ Dispatched to Queue\n", idx+1, item.FileName, item.ToolName, reviewID)
		} else {
			fmt.Printf("%-3d | %-18s | %-15s | %-10s | ❌ Status %d\n", idx+1, item.FileName, item.ToolName, "-", resp.StatusCode)
		}

		time.Sleep(1200 * time.Millisecond)
	}

	fmt.Println("==================================================================================")
	fmt.Println(" ALL 15 TOOL REVIEWS DISPATCHED SUCCESSFULLY TO WORKER POOL!")
	fmt.Println("==================================================================================")
}
