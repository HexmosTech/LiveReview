package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

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

	// Exact 15 Beta Production Tools calculated from lr-tools/cmd/deployer/lambda-config/tools.json
	// Formula: (Memory_MB / 1024) * Timeout_s / 2.5
	allTools := []ToolSeed{
		{"openapi", "OpenAPI/YAML validation baseline", "arn:aws:lambda:us-east-1:042198857528:function:openapi-yaml-validator", 1.0, "API Schema"},
		{"actionlint", "GitHub Actions workflow linter", "arn:aws:lambda:us-east-1:042198857528:function:actionlint-linter", 1.0, "CI/CD & Workflow"},
		{"hadolint", "Dockerfile linter", "arn:aws:lambda:us-east-1:042198857528:function:hadolint-linter", 1.0, "API Schema"},
		{"ruff", "Fast Python linter", "arn:aws:lambda:us-east-1:042198857528:function:ruff-python-linter", 1.0, "Code Quality & Linting"},
		{"tfsec", "Terraform security scanner", "arn:aws:lambda:us-east-1:042198857528:function:tfsec-linter", 1.0, "Infrastructure & Config"},
		{"osv", "Google open-source vulnerability scanner", "arn:aws:lambda:us-east-1:042198857528:function:osv-scanner", 1.0, "Dependencies & Supply Chain"},
		{"spectral", "API style guide enforcement", "arn:aws:lambda:us-east-1:042198857528:function:spectral-linter", 3.0, "API Schema"},
		{"trivy", "Container + SCA scanner", "arn:aws:lambda:us-east-1:042198857528:function:trivy-linter", 4.0, "Dependencies & Supply Chain"},
		{"gitleaks", "Git history secret scanning", "arn:aws:lambda:us-east-1:042198857528:function:medium", 12.0, "Secrets & Security"},
		{"trufflehog", "Deep entropy-based secret detection", "arn:aws:lambda:us-east-1:042198857528:function:trufflehog-linter", 12.0, "Secrets & Security"},
		{"detect-secrets", "Baseline pattern-based detection", "arn:aws:lambda:us-east-1:042198857528:function:detect-secrets-linter", 12.0, "Secrets & Security"},
		{"eslint", "JavaScript / TypeScript linter", "arn:aws:lambda:us-east-1:042198857528:function:eslint-linter", 12.0, "Code Quality & Linting"},
		{"bandit", "Python security linter", "arn:aws:lambda:us-east-1:042198857528:function:bandit-linter", 12.0, "Code Quality & Linting"},
		{"semgrep", "SAST — cross-language, deep analysis", "arn:aws:lambda:us-east-1:042198857528:function:semgrep-linter", 63.0, "Deep / Comprehensive Scan"},
		{"golangci-lint", "Go multi-linter (heavy)", "arn:aws:lambda:us-east-1:042198857528:function:golangci-lint-linter", 72.0, "Code Quality & Linting"},
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
		rows, _ := res.RowsAffected()
		fmt.Printf("\n🧹 Cleaned %d non-production tools from database.\n", rows)
	}

	var count int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM public.available_tools`).Scan(&count)
	fmt.Printf("==========================================================================\n")
	fmt.Printf(" TOTAL ACTIVE PRODUCTION TOOLS IN DATABASE: %d\n", count)
	fmt.Printf("==========================================================================\n")
}
