package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchBitbucketBotUser(t *testing.T) {
	versionInfo := &VersionInfo{Version: "test"}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(cwd, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("failed to change directory to repo root %s: %v", repoRoot, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	server, err := NewServer(8888, versionInfo)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	t.Cleanup(func() {
		if server.db != nil {
			server.db.Close()
		}
	})

	repoFullName := "contorted/fb_backends"

	botInfo, err := server.getFreshBitbucketBotUserInfo(repoFullName)
	if err != nil {
		t.Fatalf("failed to fetch Bitbucket bot user info: %v", err)
	}

	formatted, err := json.MarshalIndent(botInfo, "", "  ")
	if err != nil {
		t.Fatalf("failed to format bot info: %v", err)
	}

	fmt.Println("Bitbucket bot user info:\n" + string(formatted))
	t.Logf("Bitbucket bot user info: %s", string(formatted))

	if botInfo.Username == "" {
		t.Fatalf("bot username is empty")
	}
	if botInfo.DisplayName == "" {
		t.Fatalf("bot display name is empty")
	}
	if botInfo.AccountID == "" {
		t.Fatalf("bot account ID is empty")
	}
	if botInfo.UUID == "" {
		t.Fatalf("bot UUID is empty")
	}
}
