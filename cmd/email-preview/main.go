package main

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/livereview/network/email"
)

type invitationData struct {
	email.InvitationParams
	CurrentYear int
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find project root (go.mod)")
		}
		dir = parent
	}
}

func main() {
	root, err := findProjectRoot()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Sample data for invitation email
	data := invitationData{
		InvitationParams: email.InvitationParams{
			AppName:             "LiveReview",
			InvitedToName:       "John Doe",
			InvitedToEmail:      "john@example.com",
			InvitedByName:       "Jane Smith",
			URL:                 "https://livereview.example.com",
			InstallCommandLinux:   `curl -fsSL https://hexmos.com/lrc-install.sh | LRC_API_KEY="test-key-123" LRC_API_URL="https://livereview.example.com" bash`,
			InstallCommandWindows: `$env:LRC_API_KEY="test-key-123"; $env:LRC_API_URL="https://livereview.example.com"; iwr -useb https://hexmos.com/lrc-install.ps1 | iex`,
		},
		CurrentYear: time.Now().Year(),
	}

	// Sample data for verification email
	verificationData := struct {
		Recipient   string
		CurrentYear int
	}{
		Recipient:   "admin@example.com",
		CurrentYear: time.Now().Year(),
	}

	// Read templates
	templateDir := filepath.Join(root, "network", "email", "templates")
	invitationHTML, err := os.ReadFile(filepath.Join(templateDir, "invitation.html"))
	if err != nil {
		fmt.Printf("Error reading invitation template: %v\n", err)
		os.Exit(1)
	}

	verificationHTML, err := os.ReadFile(filepath.Join(templateDir, "verification.html"))
	if err != nil {
		fmt.Printf("Error reading verification template: %v\n", err)
		os.Exit(1)
	}

	// Parse templates
	invitationTmpl, err := template.New("invitation").Parse(string(invitationHTML))
	if err != nil {
		fmt.Printf("Error parsing invitation template: %v\n", err)
		os.Exit(1)
	}

	verificationTmpl, err := template.New("verification").Parse(string(verificationHTML))
	if err != nil {
		fmt.Printf("Error parsing verification template: %v\n", err)
		os.Exit(1)
	}

	// Execute templates
	var invitationBuf, verificationBuf bytes.Buffer

	if err := invitationTmpl.Execute(&invitationBuf, data); err != nil {
		fmt.Printf("Error executing invitation template: %v\n", err)
		os.Exit(1)
	}

	if err := verificationTmpl.Execute(&verificationBuf, verificationData); err != nil {
		fmt.Printf("Error executing verification template: %v\n", err)
		os.Exit(1)
	}

	// Create preview HTML with both emails
	previewHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Email Preview</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background-color: #1a1a2e;
            margin: 0;
            padding: 20px;
            color: white;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
        }
        h1 {
            text-align: center;
            margin-bottom: 30px;
        }
        .email-section {
            margin-bottom: 40px;
        }
        .email-title {
            font-size: 18px;
            font-weight: 600;
            margin-bottom: 15px;
            padding: 10px;
            background-color: #16213e;
            border-radius: 8px;
        }
        .email-frame {
            background-color: white;
            border-radius: 8px;
            overflow: hidden;
        }
        iframe {
            width: 100%%;
            height: 1600px;
            border: none;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Email Template Preview</h1>
        
        <div class="email-section">
            <div class="email-title">Invitation Email</div>
            <div class="email-frame">
                <iframe srcdoc="%s"></iframe>
            </div>
        </div>

        <div class="email-section">
            <div class="email-title">SMTP Verification Email</div>
            <div class="email-frame">
                <iframe srcdoc="%s"></iframe>
            </div>
        </div>
    </div>
</body>
</html>`, escapeForHTML(invitationBuf.String()), escapeForHTML(verificationBuf.String()))

	// Write to file
	outputFile := "email-preview.html"
	if err := os.WriteFile(outputFile, []byte(previewHTML), 0644); err != nil {
		fmt.Printf("Error writing to file: %v\n", err)
		os.Exit(1)
	}

	// Get absolute path
	absPath, _ := os.Getwd()
	absPath = absPath + "/" + outputFile

	fmt.Printf("Email preview generated: %s\n", absPath)
	fmt.Printf("Open this file in your browser to preview the emails.\n")
}

func escapeForHTML(s string) string {
	// Replace problematic characters for srcdoc attribute
	// Order matters: replace & first to avoid double-encoding
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
