package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/livereview/internal/database"
	_ "github.com/lib/pq"
)

type ToolSeed struct {
	Name        string
	Description string
	LambdaARN   string
	Multiplier  float64
	UseCase     string
}

func main() {
	dbURL, err := database.LoadDatabaseURL()
	if err != nil {
		log.Fatalf("Failed to load DATABASE_URL: %v", err)
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	awsAccountID := os.Getenv("AWS_ACCOUNT_ID")
	if awsAccountID == "" {
		awsAccountID = os.Getenv("TOOLS_LAMBDA_ACCOUNT_ID")
	}
	arnFor := func(functionName string) string {
		if awsAccountID == "" {
			return ""
		}
		return fmt.Sprintf("arn:aws:lambda:us-east-1:%s:function:%s", awsAccountID, functionName)
	}

	// Exact 15 Beta Production Tools calculated from lr-tools/cmd/deployer/lambda-config/tools.json
	// Formula: (Memory_MB / 1024) * Timeout_s / 2.5
	allTools := []ToolSeed{
		{"openapi", "OpenAPI/YAML validation baseline", arnFor("openapi-yaml-validator"), 1.0, "API Schema"},
		{"actionlint", "GitHub Actions workflow linter", arnFor("actionlint-linter"), 1.0, "CI/CD & Workflow"},
		{"hadolint", "Dockerfile linter", arnFor("hadolint-linter"), 1.0, "API Schema"},
		{"ruff", "Fast Python linter", arnFor("ruff-python-linter"), 1.0, "Code Quality & Linting"},
		{"tfsec", "Terraform security scanner", arnFor("tfsec-linter"), 1.0, "Infrastructure & Config"},
		{"osv", "Google open-source vulnerability scanner", arnFor("osv-scanner"), 1.0, "Dependencies & Supply Chain"},
		{"spectral", "API style guide enforcement", arnFor("spectral-linter"), 3.0, "API Schema"},
		{"trivy", "Container + SCA scanner", arnFor("trivy-linter"), 4.0, "Dependencies & Supply Chain"},
		{"gitleaks", "Git history secret scanning", arnFor("gitleaks-linter"), 12.0, "Secrets & Security"},
		{"trufflehog", "Deep entropy-based secret detection", arnFor("trufflehog-linter"), 12.0, "Secrets & Security"},
		{"detect-secrets", "Baseline pattern-based detection", arnFor("detect-secrets-linter"), 12.0, "Secrets & Security"},
		{"eslint", "JavaScript / TypeScript linter", arnFor("eslint-linter"), 12.0, "Code Quality & Linting"},
		{"bandit", "Python security linter", arnFor("bandit-linter"), 12.0, "Code Quality & Linting"},
		{"semgrep", "SAST — cross-language, deep analysis", arnFor("semgrep-linter"), 63.0, "Deep / Comprehensive Scan"},
		{"golangci-lint", "Go multi-linter (heavy)", arnFor("golangci-lint-linter"), 72.0, "Code Quality & Linting"},
	}

	validNames := make([]string, 0, len(allTools))
	for _, t := range allTools {
		validNames = append(validNames, t.Name)
		_, err := db.ExecContext(ctx, `
			INSERT INTO public.available_tools (name, description, lambda_arn, multiplier, use_case)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (name) DO UPDATE SET
				description = EXCLUDED.description,
				lambda_arn = EXCLUDED.lambda_arn,
				multiplier = EXCLUDED.multiplier,
				use_case = EXCLUDED.use_case
		`, t.Name, t.Description, t.LambdaARN, t.Multiplier, t.UseCase)
		if err != nil {
			log.Printf("Failed to upsert tool %s: %v", t.Name, err)
		} else {
			fmt.Printf("✓ Upserted %-16s | Credits/PR: %2.0f | Category: %s\n", t.Name, t.Multiplier, t.UseCase)
		}
	}

	// Remove non-15 tools from available_tools and org_tools
	res, err := db.ExecContext(ctx, `
		DELETE FROM public.available_tools 
		WHERE name NOT IN ('openapi', 'actionlint', 'hadolint', 'ruff', 'tfsec', 'osv', 'spectral', 'trivy', 'gitleaks', 'trufflehog', 'detect-secrets', 'eslint', 'bandit', 'semgrep', 'golangci-lint')
	`)
	if err != nil {
		log.Printf("Failed to clean non-15 tools: %v", err)
	} else {
		rows, rowsErr := res.RowsAffected()
		if rowsErr != nil {
			log.Printf("Warning: failed to get rows affected: %v", rowsErr)
		} else {
			fmt.Printf("\n🧹 Cleaned %d non-production tools from database.\n", rows)
		}
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM public.available_tools`).Scan(&count); err != nil {
		log.Printf("Warning: failed to count active production tools: %v", err)
	} else {
		fmt.Printf("==========================================================================\n")
		fmt.Printf(" TOTAL ACTIVE PRODUCTION TOOLS IN DATABASE: %d\n", count)
		fmt.Printf("==========================================================================\n")
	}
}
