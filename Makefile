.PHONY: build run-review run-review-verbose test clean develop develop-reflex river-deps river-install river-migrate river-setup river-ui-install river-ui db-flip version version-bump version-patch version-minor version-major version-bump-dirty version-patch-dirty version-minor-dirty version-major-dirty version-bump-dry version-patch-dry version-minor-dry version-major-dry build-versioned docker-build docker-build-push docker-build-dry docker-interactive docker-interactive-push docker-interactive-dry docker-build docker-build-push docker-build-versioned docker-build-push-versioned docker-build-dry docker-build-push-dry docker-multiarch docker-multiarch-push docker-multiarch-dry docker-interactive-multiarch docker-interactive-multiarch-push cplrops vendor-prompts-encrypt vendor-prompts-build vendor-prompts-rebuild vendor-docker-build vendor-docker-build-dry vendor-docker-build-push vendor-docker-multiarch-dry vendor-docker-multiarch-push run logrun api-with-migrations build-with-ui security-sbom security-sbom-cyclonedx security-sbom-spdx security-sbom-validate release-notes-init release-notes-check release-preflight release-gh niceurl niceurl2
.PHONY: upload-secrets download-secrets list-secrets-files legacy-secrets-clear
.PHONY: razorpay-webhook-ensure razorpay-webhook-ensure-dry
.PHONY: raw-deploy raw-deploy-low-pricing raw-deploy-backend raw-deploy-backend-low-pricing

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
BINARY_NAME=livereview
REQUIRED_GO_VERSION=$(shell awk '/^go /{print $$2; exit}' go.mod)
REQUIRED_GO_SERIES=$(shell echo $(REQUIRED_GO_VERSION) | awk -F. '{print $$1"."$$2}')
GOVULNCHECK_VERSION=v1.1.4
GOVULNCHECK_CMD=GOTOOLCHAIN=go$(REQUIRED_GO_VERSION) $(GOCMD) run -a golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
GH_REPO=HexmosTech/LiveReview
GH=/usr/bin/gh
GHSM_SCRIPT=scripts/ghsm.py
LEGACY_ENV_VARS=DATABASE_URL JWT_SECRET LIVEREVIEW_BACKEND_PORT LIVEREVIEW_FRONTEND_PORT LIVEREVIEW_REVERSE_PROXY LIVEREVIEW_IS_CLOUD CLOUD_JWT_SECRET FW_PARSE_ADMIN_SECRET RAZORPAY_MODE LIVEREVIEW_PRICING_PROFILE RAZORPAY_WEBHOOK_SECRET RAZORPAY_TEST_KEY RAZORPAY_TEST_SECRET RAZORPAY_TEST_MONTHLY_PLAN_ID RAZORPAY_TEST_YEARLY_PLAN_ID RAZORPAY_LIVE_KEY RAZORPAY_LIVE_SECRET RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID DISCORD_SIGNUP_WEBHOOK_URL OVSX_PAT
DEPLOY_ACTUAL_ENV_FILE=.env.prod
DEPLOY_LOW_PRICING_ENV_FILE=.env.prod.low-pricing
DEPLOY_HOST=master
DEPLOY_PATH=/root/public_lr
SYFT_CMD=syft
SBOM_DIR=security_issues/sbom
SBOM_VERSION?=$(shell git describe --tags --exact-match 2>/dev/null || git describe --tags --abbrev=0 2>/dev/null || echo dev)
SBOM_CDX=$(SBOM_DIR)/livereview-$(SBOM_VERSION)-cyclonedx.json
SBOM_SPDX=$(SBOM_DIR)/livereview-$(SBOM_VERSION)-spdx.json
SBOM_UI_CDX=$(SBOM_DIR)/livereview-ui-$(SBOM_VERSION)-cyclonedx.json
SBOM_UI_SPDX=$(SBOM_DIR)/livereview-ui-$(SBOM_VERSION)-spdx.json
RELEASE_NOTES_DIR=docs/releases
RELEASE_NOTES_TEMPLATE=$(RELEASE_NOTES_DIR)/_template.md
RELEASE_GH_SCRIPT=scripts/release_gh.py
OSV_SCANNER_CONFIG=osv-scanner.toml

# Load environment variables from .env file
include .env
export

build:
	rm $(BINARY_NAME) || true
	$(GOBUILD) -o $(BINARY_NAME)

# Vendor prompts: encrypt plaintext templates and generate embedded assets
# Usage examples:
#   make vendor-prompts-encrypt                       # default output dir
#   make vendor-prompts-encrypt ARGS="--build-id 20250101010101"
#   make vendor-prompts-encrypt ARGS="--key-hex <64-hex> --key-id mykey"
vendor-prompts-encrypt:
	$(GOCMD) run ./internal/prompts/vendor/cmd/prompts-encrypt --out internal/prompts/vendor/templates $(ARGS)

# Build binary with vendor prompts embedded (requires assets from vendor-prompts-encrypt)
vendor-prompts-build:
	$(GOBUILD) -tags vendor_prompts -o $(BINARY_NAME)_vendor ./livereview.go

# Convenience: regenerate assets and build vendor binary in one step
vendor-prompts-rebuild: vendor-prompts-encrypt vendor-prompts-build

# Version management targets
version:
	@python scripts/lrops.py version

version-bump:
	@python scripts/lrops.py bump $(ARGS)

version-patch:
	@python scripts/lrops.py bump --type patch $(ARGS)

version-minor:
	@python scripts/lrops.py bump --type minor $(ARGS)

version-major:
	@python scripts/lrops.py bump --type major $(ARGS)

# Version management targets that allow dirty working directory
version-bump-dirty:
	@python scripts/lrops.py bump --allow-dirty

version-patch-dirty:
	@python scripts/lrops.py bump --type patch --allow-dirty

version-minor-dirty:
	@python scripts/lrops.py bump --type minor --allow-dirty

version-major-dirty:
	@python scripts/lrops.py bump --type major --allow-dirty

# Dry-run version targets
version-bump-dry:
	@python scripts/lrops.py bump --dry-run --allow-dirty

version-patch-dry:
	@python scripts/lrops.py bump --type patch --dry-run --allow-dirty

version-minor-dry:
	@python scripts/lrops.py bump --type minor --dry-run --allow-dirty

version-major-dry:
	@python scripts/lrops.py bump --type major --dry-run --allow-dirty

build-versioned:
	@python scripts/lrops.py build

# DOCKER-BUILD: Comprehensive Docker image build with automated version management
# Implementation: scripts/lrops.py:cmd_build() -> build_docker_image() (lines 634-661)
# Process Flow:
#   1. Gets current Git version/commit from get_current_version_info() (lines 186-261)
#   2. Builds React UI: cd ui/ && npm install && npm run build (via Dockerfile stage 1)
#   3. Creates multi-stage Docker build with embedded UI assets
#   4. Injects version info via build args: VERSION, BUILD_TIME, GIT_COMMIT (Dockerfile lines 78-85)
#   5. Uses Dockerfile stages: ui-builder (Node.js) -> go-builder (Go+tools) -> alpine runtime
#   6. Embeds dbmate, River CLI/UI tools for database/queue management
#   7. Single-arch build by default (amd64), can be multi-arch with --multiarch
#   8. Interactive confirmation prompt before build execution
# Files: scripts/lrops.py (lines 634-826), Dockerfile (multi-stage), ui/package.json
docker-build:
	@python scripts/lrops.py build --docker $(ARGS)

# DOCKER-BUILD-PUSH: Same as docker-build but automatically pushes to registry
# Implementation: scripts/lrops.py:cmd_build() with push=True flag
# Process Flow:
#   1-7. Same as docker-build above
#   8. Additional push phase via _build_single_arch_image() (lines 827-866)
#   9. Pushes to registry: git.apps.hexmos.com:5050/hexmos/livereview by default
#   10. Tags both version-specific (e.g., v1.2.3) and 'latest' if make_latest=True
#   11. Uses docker push commands for each tag
# Registry: Configurable via --registry, defaults to GitLab Container Registry
# Tags: <registry>/<image>:<version> and optionally <registry>/<image>:latest
docker-build-push:
	@python scripts/lrops.py build --docker --push $(ARGS)

# Interactive Docker build with tag selection
docker-interactive:
	@python scripts/lrops.py docker

docker-interactive-push:
	@python scripts/lrops.py docker --push $(ARGS)

# Dry-run Docker targets
docker-build-dry:
	@python scripts/lrops.py build --docker --dry-run $(ARGS)

docker-interactive-dry:
	@python scripts/lrops.py docker --dry-run

# Legacy build-push for backward compatibility (now uses versioning)
build-push: docker-build-push

run:
	pkill -9 livereview || true
	@DLV_BIN_DIR=$$(go env GOBIN); \
	if [ -z "$$DLV_BIN_DIR" ]; then DLV_BIN_DIR="$$(go env GOPATH)/bin"; fi; \
	command -v dlv >/dev/null 2>&1 || { \
		echo "Installing Delve with Go $(REQUIRED_GO_VERSION)..."; \
		GOTOOLCHAIN=go$(REQUIRED_GO_VERSION) $(GOCMD) install github.com/go-delve/delve/cmd/dlv@latest; \
	}; \
	if ! go version -m "$$DLV_BIN_DIR/dlv" 2>/dev/null | grep -q "go$(REQUIRED_GO_SERIES)"; then \
		echo "Rebuilding Delve with Go $(REQUIRED_GO_VERSION) for DWARFv5+ compatibility..."; \
		GOTOOLCHAIN=go$(REQUIRED_GO_VERSION) $(GOCMD) install github.com/go-delve/delve/cmd/dlv@latest; \
	fi
	which air || go install github.com/air-verse/air@latest
	DLV_BIN_DIR=$$(go env GOBIN); if [ -z "$$DLV_BIN_DIR" ]; then DLV_BIN_DIR="$$(go env GOPATH)/bin"; fi; PATH="$$DLV_BIN_DIR:$$PATH" air

logrun:
	which air || go install github.com/air-verse/air@latest
	bash -c 'set -o pipefail; air 2>&1 | tee "logrun-$$(date +%Y%m%d-%H%M%S).log"'

develop:
	@DLV_BIN_DIR=$$(go env GOBIN); \
	if [ -z "$$DLV_BIN_DIR" ]; then DLV_BIN_DIR="$$(go env GOPATH)/bin"; fi; \
	command -v dlv >/dev/null 2>&1 || { \
		echo "Installing Delve with Go $(REQUIRED_GO_VERSION)..."; \
		GOTOOLCHAIN=go$(REQUIRED_GO_VERSION) $(GOCMD) install github.com/go-delve/delve/cmd/dlv@latest; \
	}; \
	if ! go version -m "$$DLV_BIN_DIR/dlv" 2>/dev/null | grep -q "go$(REQUIRED_GO_SERIES)"; then \
		echo "Rebuilding Delve with Go $(REQUIRED_GO_VERSION) for DWARFv5+ compatibility..."; \
		GOTOOLCHAIN=go$(REQUIRED_GO_VERSION) $(GOCMD) install github.com/go-delve/delve/cmd/dlv@latest; \
	fi
	which air || go install github.com/air-verse/air@latest
	DLV_BIN_DIR=$$(go env GOBIN); if [ -z "$$DLV_BIN_DIR" ]; then DLV_BIN_DIR="$$(go env GOPATH)/bin"; fi; PATH="$$DLV_BIN_DIR:$$PATH" air

develop-reflex:
	which reflex || go install github.com/cespare/reflex@latest
	reflex -r '\.go$$' -s -- sh -c 'go build -o $(BINARY_NAME) && ./$(BINARY_NAME) api'

api-with-migrations:
	dbmate up
	go run livereview.go api

run-review:
	./$(BINARY_NAME) review --dry-run https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/365

run-review-verbose:
	./$(BINARY_NAME) review --dry-run --verbose https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/365

test:
	$(GOTEST) -v ./...

# Discover Go package directories while avoiding restricted directories
TEST_PACKAGES := $(shell find . \
	-path './livereview_pgdata' -prune -o \
	-path './lrdata' -prune -o \
	-path './vendor' -prune -o \
	-path './debug' -prune -o \
	-path './tests' -prune -o \
	-type f -name '*.go' -print 2>/dev/null | \
	xargs -n1 dirname | sort -u | tr '\n' ' ')

# Exclude ./scripts because it contains multiple standalone main programs.
SECURITY_GOVULN_PACKAGES := $(filter-out ./scripts,$(TEST_PACKAGES))

.PHONY: testall
testall:
	$(GOTEST) -count=1 $(TEST_PACKAGES)

.PHONY: security-govulncheck security-govulncheck-json security-osv security-gitleaks security-semgrep security-dependabot security-gh-secret-scanning security-triage

# Run Go vulnerability analysis for reachable vulnerabilities.
security-govulncheck:
	@echo "Running govulncheck $(GOVULNCHECK_VERSION) with Go $(REQUIRED_GO_VERSION)..."
	@$(GOVULNCHECK_CMD) $(SECURITY_GOVULN_PACKAGES)

# Emit govulncheck report as JSON artifact under security_issues/.
security-govulncheck-json:
	mkdir -p security_issues
	$(GOVULNCHECK_CMD) -json $(SECURITY_GOVULN_PACKAGES) > security_issues/govulncheck-$(shell date +%d-%m-%Y).json

# Run OSV scanner against this repository.
security-osv:
	@command -v osv-scanner >/dev/null 2>&1 || { \
		echo "osv-scanner not found. Install from https://github.com/google/osv-scanner"; \
		exit 1; \
	}
	@mkdir -p security_issues
	@dated_report="security_issues/osv-scanner-$(shell date +%d-%m-%Y).json"; \
		latest_report="security_issues/osv-scanner-latest.json"; \
		status=0; \
		osv-scanner scan source --recursive --format json --config $(OSV_SCANNER_CONFIG) --no-call-analysis=go \
			--experimental-exclude=debug \
			--experimental-exclude=scripts \
			--experimental-exclude=tests \
			--experimental-exclude=.livereview_pgdata \
			--experimental-exclude=.lrdata \
			--experimental-exclude=livereview_pgdata \
			--experimental-exclude=lrdata \
			. > "$$dated_report" || status=$$?; \
		if [ $$status -ne 0 ] && [ $$status -ne 1 ]; then \
			echo "osv-scanner failed with exit code $$status"; \
			exit $$status; \
		fi; \
		if [ ! -s "$$dated_report" ]; then \
			echo "osv-scanner did not produce a report"; \
			exit 1; \
		fi; \
		cp "$$dated_report" "$$latest_report"; \
		if [ $$status -eq 1 ]; then \
			echo "osv-scanner reported vulnerabilities (exit 1); report still generated."; \
		fi; \
		echo "Wrote $$dated_report"; \
		echo "Updated $$latest_report"

# Run gitleaks and emit a dated CSV artifact under security_issues/.
security-gitleaks:
	@command -v gitleaks >/dev/null 2>&1 || { \
		echo "gitleaks not found. Install from https://github.com/gitleaks/gitleaks"; \
		exit 1; \
	}
	@mkdir -p security_issues
	@gitleaks git . -f csv -r security_issues/gitleaks-$(shell date +%d-%m-%Y).csv
	@echo "Wrote security_issues/gitleaks-$(shell date +%d-%m-%Y).csv"

# Run Semgrep and emit a dated JSON artifact under security_issues/.
security-semgrep:
	@command -v semgrep >/dev/null 2>&1 || { \
		echo "semgrep not found. Install from https://semgrep.dev/docs/getting-started/quickstart"; \
		exit 1; \
	}
	@mkdir -p security_issues
	@dated_report="security_issues/semgrep-$(shell date +%d-%m-%Y).json"; \
		latest_report="security_issues/semgrep-latest.json"; \
		fail_on_findings=$${SEMGREP_FAIL_ON_FINDINGS:-1}; \
		status=0; \
		semgrep scan --config auto \
			--exclude ui/docs \
			--exclude ui/build \
			--exclude ui/dist \
			--json --output "$$dated_report" . || status=$$?; \
		if [ $$status -ne 0 ] && [ $$status -ne 1 ]; then \
			echo "semgrep failed with exit code $$status"; \
			exit $$status; \
		fi; \
		if [ ! -s "$$dated_report" ]; then \
			echo "semgrep did not produce a report"; \
			exit 1; \
		fi; \
		cp "$$dated_report" "$$latest_report"; \
		if [ $$status -eq 1 ]; then \
			echo "semgrep reported findings (exit 1); report still generated."; \
			if [ "$$fail_on_findings" = "1" ]; then \
				echo "Failing because SEMGREP_FAIL_ON_FINDINGS=$$fail_on_findings"; \
				exit 1; \
			fi; \
			echo "Continuing because SEMGREP_FAIL_ON_FINDINGS=$$fail_on_findings"; \
		fi; \
		echo "Wrote $$dated_report"; \
		echo "Updated $$latest_report"

# Pull Dependabot alerts via GitHub API and emit a dated JSON artifact under security_issues/.
security-dependabot:
	@command -v $(GH) >/dev/null 2>&1 || { \
		echo "gh not found. Install from https://cli.github.com/"; \
		exit 1; \
	}
	@mkdir -p security_issues
	@dated_report="security_issues/dependabot-live-review-$(shell date +%d-%m-%Y).json"; \
		$(GH) api \
			-H "Accept: application/vnd.github+json" \
			-H "X-GitHub-Api-Version: 2022-11-28" \
			/repos/$(GH_REPO)/dependabot/alerts \
			--paginate > "$$dated_report"; \
		echo "Wrote $$dated_report"

# Pull secret scanning alerts via GitHub API and emit a dated JSON artifact under security_issues/.
security-gh-secret-scanning:
	@command -v $(GH) >/dev/null 2>&1 || { \
		echo "gh not found. Install from https://cli.github.com/"; \
		exit 1; \
	}
	@mkdir -p security_issues
	@dated_report="security_issues/gh-secret-scanning-live-review-$(shell date +%d-%m-%Y).json"; \
		$(GH) api \
			-H "Accept: application/vnd.github+json" \
			-H "X-GitHub-Api-Version: 2022-11-28" \
			/repos/$(GH_REPO)/secret-scanning/alerts \
			--paginate > "$$dated_report"; \
		echo "Wrote $$dated_report"

# Regenerate machine-readable and markdown triage artifacts from the latest OSV report.
security-triage: security-osv
	@python3 scripts/extract_osv_report.py \
		--input security_issues/osv-scanner-latest.json \
		--csv security_issues/osv-triage-latest.csv \
		--md security_issues/osv-triage-latest.md
	@echo "Wrote security_issues/osv-triage-latest.csv"
	@echo "Wrote security_issues/osv-triage-latest.md"

.PHONY: license-test
license-test:
	$(GOTEST) -v ./internal/license -count=1

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

# River queue setup commands
river-deps:
	go get github.com/riverqueue/river
	go get github.com/riverqueue/river/riverdriver/riverpgxv5

river-install:
	go install github.com/riverqueue/river/cmd/river@latest

river-ui-install:
	go install riverqueue.com/riverui/cmd/riverui@latest

river-migrate:
	river migrate-up --database-url "$(DATABASE_URL)"

river-ui:
	@echo "Starting River UI with DATABASE_URL: $(DATABASE_URL)"
	DATABASE_URL="$(DATABASE_URL)" riverui

# 🚀 ONE COMMAND TO DO IT ALL - Install River dependencies, CLI tool, UI tool, and run migrations
river-setup: river-deps river-install river-ui-install river-migrate

# Database URL switcher - flips between localhost and livereview-db
db-flip:
	@echo "Current DATABASE_URL in .env:"
	@grep "DATABASE_URL=" .env
	@if grep -q "@localhost:" .env; then \
		echo "Switching from localhost to livereview-db..."; \
		sed -i 's/@localhost:/@livereview-db:/g' .env; \
	elif grep -q "@livereview-db:" .env; then \
		echo "Switching from livereview-db to localhost..."; \
		sed -i 's/@livereview-db:/@localhost:/g' .env; \
	else \
		echo "No recognized database host found in .env file"; \
		exit 1; \
	fi
	@echo "New DATABASE_URL in .env:"
	@grep "DATABASE_URL=" .env

# Multi-architecture Docker build targets
docker-multiarch:
	@python scripts/lrops.py build --docker --multiarch $(ARGS)

docker-multiarch-push:
	@python scripts/lrops.py build --docker --multiarch --push $(ARGS)
	@echo "ℹ️  Optional GitHub release publish: make release-gh"
	@echo "   Optional explicit override: make release-gh VERSION=$$(git describe --tags --abbrev=0 2>/dev/null || true)"

# Optionally publish a GitHub release using markdown notes (no binary assets).
# VERSION is optional and auto-inferred by scripts/release_gh.py.
release-gh:
	@python3 $(RELEASE_GH_SCRIPT) --repo $(GH_REPO) $(if $(VERSION),--version $(VERSION),)

docker-multiarch-dry:
	@python scripts/lrops.py build --docker --multiarch --dry-run $(ARGS)

# Vendor multi-arch dry run (Phase 9 validation)
vendor-docker-multiarch-dry:
	@python scripts/lrops.py build --docker --multiarch --dry-run --vendor-prompts $(ARGS)

# Vendor single-arch builds
vendor-docker-build-dry:
	@python scripts/lrops.py build --docker --dry-run --vendor-prompts $(ARGS)

vendor-docker-build:
	@python scripts/lrops.py build --docker --vendor-prompts $(ARGS)

vendor-docker-build-push:
	@python scripts/lrops.py build --docker --push --vendor-prompts $(ARGS)

# Vendor multi-arch push (with optional latest tagging via ARGS="--latest")
vendor-docker-multiarch-push:
	@python scripts/lrops.py build --docker --multiarch --push --vendor-prompts $(ARGS)

# Cross-compilation Docker build targets (faster ARM builds)
docker-multiarch-cross:
	@echo "🚀 Building multi-arch images using cross-compilation for faster ARM builds"
	@python scripts/lrops.py build --docker --multiarch $(ARGS)

docker-multiarch-cross-push:
	@echo "🚀 Building and pushing multi-arch images using cross-compilation"
	@python scripts/lrops.py build --docker --multiarch --push $(ARGS)

# Interactive multi-architecture Docker build
docker-interactive-multiarch:
	@python scripts/lrops.py docker --multiarch

docker-interactive-multiarch-push:
	@python scripts/lrops.py docker --multiarch --push

cplrops:
	@cp lrops.sh ../gh/LiveReview/

.PHONY: vendor-memdump-check
vendor-memdump-check: ## Build vendor binary, run render smoke, gcore, and grep for raw template markers
	@echo "[memdump] Building render-smoke with vendor_prompts..."
	$(GOBUILD) -tags vendor_prompts -o render-smoke ./cmd/render-smoke
	@echo "[memdump] Starting render-smoke (short run)..."
	LOOPS=200 ./render-smoke & echo $$! > .render_smoke.pid
	sleep 1
	@echo "[memdump] Capturing core dump via gcore (requires gdb)..."
	-@pkill -0 `cat .render_smoke.pid` >/dev/null 2>&1 && gcore -o core_render_smoke `cat .render_smoke.pid` >/dev/null 2>&1 || true
	@echo "[memdump] Stopping render-smoke..."
	-@kill `cat .render_smoke.pid` >/dev/null 2>&1 || true
	rm -f .render_smoke.pid
	@echo "[memdump] Grepping dump for raw template markers ({{VAR:) ..."
	-@if ls core_render_smoke.* >/dev/null 2>&1; then \
		strings core_render_smoke.* | grep -n "{{VAR:" || true; \
	else \
		echo "No core via gcore; trying SIGSEGV fallback..."; \
		bash scripts/memdump_check.sh; \
	fi

niceurl:
	@command -v autossh >/dev/null 2>&1 || { \
		echo "autossh is not installed. Install it with: sudo apt install autossh"; \
		exit 1; \
	}
	@ssh root@master "PID=\$$( netstat -tulpn | grep :6543 | awk '{print \$$7}' | cut -d/ -f1 | head -n 1); [ -n \"\$$PID\" ] && kill -9 \$$PID || true" || true
	@echo "Starting autossh reverse tunnel on remote port 6543 -> localhost:8081"
	@AUTOSSH_GATETIME=0 AUTOSSH_POLL=60 AUTOSSH_FIRST_POLL=30 AUTOSSH_LOGLEVEL=6 autossh -M 20000 \
		-o ServerAliveInterval=30 \
		-o ServerAliveCountMax=3 \
		-o TCPKeepAlive=yes \
		-o ExitOnForwardFailure=yes \
		-o ConnectTimeout=10 \
		-o ConnectionAttempts=3 \
		-R 6543:localhost:8081 root@master -N

niceurl2:
	@command -v autossh >/dev/null 2>&1 || { \
		echo "autossh is not installed. Install it with: sudo apt install autossh"; \
		exit 1; \
	}
	@ssh root@master "PID=\$$( netstat -tulpn | grep :6544 | awk '{print \$$7}' | cut -d/ -f1 | head -n 1); [ -n \"\$$PID\" ] && kill -9 \$$PID || true" || true
	@echo "Starting autossh reverse tunnel on remote port 6544 -> localhost:8081"
	@AUTOSSH_GATETIME=0 AUTOSSH_POLL=60 AUTOSSH_FIRST_POLteL=30 AUTOSSH_LOGLEVEL=6 autossh -M 20001 \
		-o ServerAliveInterval=30 \
		-o ServerAliveCountMax=3 \
		-o TCPKeepAlive=yes \
		-o ExitOnForwardFailure=yes \
		-o ConnectTimeout=10 \
		-o ConnectionAttempts=3 \
		-R 6544:localhost:8081 root@master -N

niceurl3:
	@command -v autossh >/dev/null 2>&1 || { \
		echo "autossh is not installed. Install it with: sudo apt install autossh"; \
		exit 1; \
	}
	@ssh root@master-do "PID=\$$( netstat -tulpn | grep :6545 | awk '{print \$$7}' | cut -d/ -f1 | head -n 1); [ -n \"\$$PID\" ] && kill -9 \$$PID || true" || true
	@echo "Starting autossh reverse tunnel on remote port 6545 -> localhost:8081"
	@AUTOSSH_GATETIME=0 AUTOSSH_POLL=60 AUTOSSH_FIRST_POLteL=30 AUTOSSH_LOGLEVEL=6 autossh -M 20002 \
		-o ServerAliveInterval=30 \
		-o ServerAliveCountMax=3 \
		-o TCPKeepAlive=yes \
		-o ExitOnForwardFailure=yes \
		-o ConnectTimeout=10 \
		-o ConnectionAttempts=3 \
		-R 6545:localhost:8081 root@master-do -N

build-with-ui:
	@echo "🔨 Building for PRODUCTION deployment (is_cloud=true)"
	@if [ ! -f .env.prod ]; then \
		echo "❌ ERROR: .env.prod not found! Cannot build for production."; \
		exit 1; \
	fi
	rm $(BINARY_NAME) || true
	cd ui/ && npm install && set -a && . ./.env.prod && set +a && LIVEREVIEW_BUILD_MODE=prod NODE_ENV=production npm run build:obfuscated && cd ..
	go build livereview.go
	@echo "✅ Production build complete. Binary ready for raw-deploy."

raw-deploy: build-with-ui
	@echo "🚀 Deploying to production server..."
	@if [ ! -f ./livereview ]; then \
		echo "❌ ERROR: livereview binary not found! Run 'make build-with-ui' first."; \
		exit 1; \
	fi
	@if [ ! -f $(DEPLOY_ACTUAL_ENV_FILE) ]; then \
		echo "❌ ERROR: $(DEPLOY_ACTUAL_ENV_FILE) not found"; \
		exit 1; \
	fi
	@MODE_VALUE=$$(awk -F= '/^RAZORPAY_MODE=/{print $$2}' $(DEPLOY_ACTUAL_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	if [ "$$MODE_VALUE" != "live" ]; then \
		echo "❌ ERROR: raw-deploy requires RAZORPAY_MODE=live in $(DEPLOY_ACTUAL_ENV_FILE)"; \
		exit 1; \
	fi; \
	PROFILE_VALUE=$$(awk -F= '/^LIVEREVIEW_PRICING_PROFILE=/{print $$2}' $(DEPLOY_ACTUAL_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	if [ "$$PROFILE_VALUE" != "actual" ]; then \
		echo "❌ ERROR: raw-deploy requires LIVEREVIEW_PRICING_PROFILE=actual in $(DEPLOY_ACTUAL_ENV_FILE)"; \
		exit 1; \
	fi; \
	MONTHLY_PLAN_ID=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID=/{print $$2}' $(DEPLOY_ACTUAL_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	YEARLY_PLAN_ID=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID=/{print $$2}' $(DEPLOY_ACTUAL_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	if [ -z "$$MONTHLY_PLAN_ID" ] || [ -z "$$YEARLY_PLAN_ID" ]; then \
		echo "❌ ERROR: raw-deploy requires RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID and RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID"; \
		exit 1; \
	fi
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && mv ./livereview ./livereview.bak || true"
	rsync -avz ./livereview db-ready.sh ecosystem.config.js deps.sh $(DEPLOY_HOST):$(DEPLOY_PATH)/
	rsync -avz ./$(DEPLOY_ACTUAL_ENV_FILE) $(DEPLOY_HOST):$(DEPLOY_PATH)/.env
	rsync -avz ./db/ $(DEPLOY_HOST):$(DEPLOY_PATH)/db/
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && chmod a+x db-ready.sh && ./db-ready.sh"
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && pm2 reload ecosystem.config.js"
	@echo "✅ Production deployment complete!"

raw-deploy-low-pricing: build-with-ui
	@echo "🚀 Deploying to production server with LOW pricing profile..."
	@if [ ! -f ./livereview ]; then \
		echo "❌ ERROR: livereview binary not found! Run 'make build-with-ui' first."; \
		exit 1; \
	fi
	@if [ ! -f $(DEPLOY_LOW_PRICING_ENV_FILE) ]; then \
		echo "❌ ERROR: $(DEPLOY_LOW_PRICING_ENV_FILE) not found"; \
		exit 1; \
	fi
	@MODE_VALUE=$$(awk -F= '/^RAZORPAY_MODE=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	if [ "$$MODE_VALUE" != "live" ]; then \
		echo "❌ ERROR: raw-deploy-low-pricing requires RAZORPAY_MODE=live in $(DEPLOY_LOW_PRICING_ENV_FILE)"; \
		exit 1; \
	fi; \
	PROFILE_VALUE=$$(awk -F= '/^LIVEREVIEW_PRICING_PROFILE=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	if [ "$$PROFILE_VALUE" != "low_pricing_test" ]; then \
		echo "❌ ERROR: raw-deploy-low-pricing requires LIVEREVIEW_PRICING_PROFILE=low_pricing_test in $(DEPLOY_LOW_PRICING_ENV_FILE)"; \
		exit 1; \
	fi; \
	MONTHLY_PLAN_ID=$$(awk -F= '/^RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	YEARLY_PLAN_ID=$$(awk -F= '/^RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	if [ -z "$$MONTHLY_PLAN_ID" ] || [ -z "$$YEARLY_PLAN_ID" ]; then \
		echo "❌ ERROR: raw-deploy-low-pricing requires RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID and RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID"; \
		exit 1; \
	fi
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && mv ./livereview ./livereview.bak || true"
	rsync -avz ./livereview db-ready.sh ecosystem.config.js deps.sh $(DEPLOY_HOST):$(DEPLOY_PATH)/
	rsync -avz ./$(DEPLOY_LOW_PRICING_ENV_FILE) $(DEPLOY_HOST):$(DEPLOY_PATH)/.env
	rsync -avz ./db/ $(DEPLOY_HOST):$(DEPLOY_PATH)/db/
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && chmod a+x db-ready.sh && ./db-ready.sh"
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && pm2 reload ecosystem.config.js"
	@echo "✅ Production deployment complete!"

raw-deploy-backend:
	@echo "🚀 Deploying to production server..."
	go build livereview.go
	@if [ ! -f ./livereview ]; then \
		echo "❌ ERROR: livereview binary not found! Run 'make build-with-ui' first."; \
		exit 1; \
	fi
	@if [ ! -f $(DEPLOY_ACTUAL_ENV_FILE) ]; then \
		echo "❌ ERROR: $(DEPLOY_ACTUAL_ENV_FILE) not found"; \
		exit 1; \
	fi
	@MODE_VALUE=$$(awk -F= '/^RAZORPAY_MODE=/{print $$2}' $(DEPLOY_ACTUAL_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	if [ "$$MODE_VALUE" != "live" ]; then \
		echo "❌ ERROR: raw-deploy-backend requires RAZORPAY_MODE=live in $(DEPLOY_ACTUAL_ENV_FILE)"; \
		exit 1; \
	fi; \
	PROFILE_VALUE=$$(awk -F= '/^LIVEREVIEW_PRICING_PROFILE=/{print $$2}' $(DEPLOY_ACTUAL_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	if [ "$$PROFILE_VALUE" != "actual" ]; then \
		echo "❌ ERROR: raw-deploy-backend requires LIVEREVIEW_PRICING_PROFILE=actual in $(DEPLOY_ACTUAL_ENV_FILE)"; \
		exit 1; \
	fi
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && mv ./livereview ./livereview.bak || true"
	rsync -avz ./livereview db-ready.sh ecosystem.config.js deps.sh $(DEPLOY_HOST):$(DEPLOY_PATH)/
	rsync -avz ./$(DEPLOY_ACTUAL_ENV_FILE) $(DEPLOY_HOST):$(DEPLOY_PATH)/.env
	rsync -avz ./db/ $(DEPLOY_HOST):$(DEPLOY_PATH)/db/
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && chmod a+x db-ready.sh && ./db-ready.sh"
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && pm2 reload ecosystem.config.js"
	@echo "✅ Production deployment complete!"

raw-deploy-backend-low-pricing:
	@echo "🚀 Deploying backend with LOW pricing profile..."
	go build livereview.go
	@if [ ! -f ./livereview ]; then \
		echo "❌ ERROR: livereview binary not found! Run 'make build-with-ui' first."; \
		exit 1; \
	fi
	@if [ ! -f $(DEPLOY_LOW_PRICING_ENV_FILE) ]; then \
		echo "❌ ERROR: $(DEPLOY_LOW_PRICING_ENV_FILE) not found"; \
		exit 1; \
	fi
	@MODE_VALUE=$$(awk -F= '/^RAZORPAY_MODE=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	if [ "$$MODE_VALUE" != "live" ]; then \
		echo "❌ ERROR: raw-deploy-backend-low-pricing requires RAZORPAY_MODE=live in $(DEPLOY_LOW_PRICING_ENV_FILE)"; \
		exit 1; \
	fi; \
	PROFILE_VALUE=$$(awk -F= '/^LIVEREVIEW_PRICING_PROFILE=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	if [ "$$PROFILE_VALUE" != "low_pricing_test" ]; then \
		echo "❌ ERROR: raw-deploy-backend-low-pricing requires LIVEREVIEW_PRICING_PROFILE=low_pricing_test in $(DEPLOY_LOW_PRICING_ENV_FILE)"; \
		exit 1; \
	fi
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && mv ./livereview ./livereview.bak || true"
	rsync -avz ./livereview db-ready.sh ecosystem.config.js deps.sh $(DEPLOY_HOST):$(DEPLOY_PATH)/
	rsync -avz ./$(DEPLOY_LOW_PRICING_ENV_FILE) $(DEPLOY_HOST):$(DEPLOY_PATH)/.env
	rsync -avz ./db/ $(DEPLOY_HOST):$(DEPLOY_PATH)/db/
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && chmod a+x db-ready.sh && ./db-ready.sh"
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && pm2 reload ecosystem.config.js"
	@echo "✅ Production deployment complete!"

# Deploy nginx config to production server
deploy-nginx:
	@echo "🔧 Deploying nginx config to production server..."
	rsync -avz ./livereview.hexmos.com master:/etc/nginx/sites-available/livereview.hexmos.com
	ssh master "nginx -t && systemctl reload nginx"
	@echo "✅ Nginx config deployed and reloaded!"

# Fetch recent PM2 logs from the production host for quick inspection
# Usage: make pm2-logs [LINES=400]
pm2-logs:
	@LOG_LINES=$${LINES:-200}; \
	echo "📜 Fetching last $$LOG_LINES lines of PM2 logs from master..."; \
	ssh master "tail -n $$LOG_LINES ~/.pm2/logs/livereview-api-out.log ~/.pm2/logs/livereview-api-error.log ~/.pm2/logs/livereview-ui-out.log ~/.pm2/logs/livereview-ui-error.log"

run-selfhosted:
	which air || go install github.com/air-verse/air@latest
	air -- --env-file .env.selfhosted

# Upload tracked env files (.env, .env.prod, ui/.env.prod) to GitHub repo variables.
# Backward compatible target name; implementation moved to scripts/ghsm.py.
upload-secrets:
	@python3 $(GHSM_SCRIPT) --repo $(GH_REPO) upload

# Download tracked env files from GitHub repo variables and replace local files.
# scripts/ghsm.py always creates timestamped local backups before overwrite.
download-secrets:
	@python3 $(GHSM_SCRIPT) --repo $(GH_REPO) download

# Show which files are managed by GH secret manager.
list-secrets-files:
	@python3 $(GHSM_SCRIPT) --repo $(GH_REPO) list-files

# Legacy helper: clear old key/value repo variables used by previous upload-secrets flow.
legacy-secrets-clear:
	@echo "Removing legacy key/value repository variables from $(GH_REPO)..."
	@for var in $(LEGACY_ENV_VARS); do \
		$(GH) variable delete "$$var" --repo $(GH_REPO) >/dev/null 2>&1 || true; \
	done
	@echo "✅ Legacy variable cleanup complete."

# Generate SBOMs in both CycloneDX and SPDX formats for Go and UI dependencies.
security-sbom: security-sbom-cyclonedx security-sbom-spdx security-sbom-validate

security-sbom-cyclonedx:
	@command -v $(SYFT_CMD) >/dev/null 2>&1 || { \
		echo "❌ syft not found. Install from https://github.com/anchore/syft"; \
		exit 1; \
	}
	@mkdir -p $(SBOM_DIR)
	@$(SYFT_CMD) file:go.mod --source-name livereview --source-version $(SBOM_VERSION) -o cyclonedx-json=$(SBOM_CDX)
	@$(SYFT_CMD) file:ui/package-lock.json --source-name livereview-ui --source-version $(SBOM_VERSION) -o cyclonedx-json=$(SBOM_UI_CDX)
	@echo "ℹ️  SBOM version: $(SBOM_VERSION)"
	@echo "✅ Wrote $(SBOM_CDX)"
	@echo "✅ Wrote $(SBOM_UI_CDX)"

security-sbom-spdx:
	@command -v $(SYFT_CMD) >/dev/null 2>&1 || { \
		echo "❌ syft not found. Install from https://github.com/anchore/syft"; \
		exit 1; \
	}
	@mkdir -p $(SBOM_DIR)
	@$(SYFT_CMD) file:go.mod --source-name livereview --source-version $(SBOM_VERSION) -o spdx-json=$(SBOM_SPDX)
	@$(SYFT_CMD) file:ui/package-lock.json --source-name livereview-ui --source-version $(SBOM_VERSION) -o spdx-json=$(SBOM_UI_SPDX)
	@echo "ℹ️  SBOM version: $(SBOM_VERSION)"
	@echo "✅ Wrote $(SBOM_SPDX)"
	@echo "✅ Wrote $(SBOM_UI_SPDX)"

security-sbom-validate:
	@test -s $(SBOM_CDX)
	@test -s $(SBOM_SPDX)
	@test -s $(SBOM_UI_CDX)
	@test -s $(SBOM_UI_SPDX)
	@echo "✅ SBOM validation passed"

# Generate release notes file from template.
# Usage: make release-notes-init VERSION=v1.2.3
release-notes-init:
	@if [ -z "$(VERSION)" ]; then \
		echo "❌ VERSION is required. Example: make release-notes-init VERSION=v1.2.3"; \
		exit 1; \
	fi
	@echo "$(VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$$' || { \
		echo "❌ VERSION must match vX.Y.Z"; \
		exit 1; \
	}
	@test -f $(RELEASE_NOTES_TEMPLATE) || { \
		echo "❌ Missing template: $(RELEASE_NOTES_TEMPLATE)"; \
		exit 1; \
	}
	@mkdir -p $(RELEASE_NOTES_DIR)
	@target="$(RELEASE_NOTES_DIR)/$(VERSION).md"; \
	if [ -f "$$target" ]; then \
		echo "❌ Release notes already exist: $$target"; \
		exit 1; \
	fi; \
	sed -e "s/__VERSION__/$(VERSION)/g" -e "s/__DATE__/$(shell date -u +%Y-%m-%d)/g" "$(RELEASE_NOTES_TEMPLATE)" > "$$target"; \
	echo "✅ Created $$target"

# Validate release notes file exists and required headings are present.
# Usage: make release-notes-check VERSION=v1.2.3
release-notes-check:
	@if [ -z "$(VERSION)" ]; then \
		echo "❌ VERSION is required. Example: make release-notes-check VERSION=v1.2.3"; \
		exit 1; \
	fi
	@echo "$(VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$$' || { \
		echo "❌ VERSION must match vX.Y.Z"; \
		exit 1; \
	}
	@target="$(RELEASE_NOTES_DIR)/$(VERSION).md"; \
	test -f "$$target" || { echo "❌ Missing release notes: $$target"; exit 1; }; \
	test -s "$$target" || { echo "❌ Release notes file is empty: $$target"; exit 1; }; \
	grep -q '^## Summary' "$$target" || { echo "❌ Missing required section: ## Summary"; exit 1; }; \
	grep -q '^## Install and Update' "$$target" || { echo "❌ Missing required section: ## Install and Update"; exit 1; }; \
	grep -q '^## Changes' "$$target" || { echo "❌ Missing required section: ## Changes"; exit 1; }; \
	echo "✅ Release notes validated: $$target"

# Run all release checks before creating/publishing a GitHub release.
# Usage: make release-preflight VERSION=v1.2.3
release-preflight: release-notes-check
	@echo "✅ Release preflight passed for $(VERSION)"

check-status-doc:
	chmod +x scripts/check-status-doc-links.sh
	./scripts/check-status-doc-links.sh

# Ensure Razorpay webhook exists for this deployment URL.
# Usage:
#   make razorpay-webhook-ensure BASE_URL=https://manual-talent2.apps.hexmos.com MODE=test
#   make razorpay-webhook-ensure-dry BASE_URL=manual-talent2.apps.hexmos.com MODE=test
razorpay-webhook-ensure:
	@if [ -z "$(BASE_URL)" ]; then \
		echo "❌ BASE_URL is required. Example: make razorpay-webhook-ensure BASE_URL=https://manual-talent2.apps.hexmos.com MODE=test"; \
		exit 1; \
	fi
	@MODE_VALUE="$(MODE)"; \
	if [ -z "$$MODE_VALUE" ]; then MODE_VALUE="$${RAZORPAY_MODE:-live}"; fi; \
	python3 scripts/razorpay_webhook_ensure.py --base-url "$(BASE_URL)" --mode "$$MODE_VALUE" $(ARGS)

razorpay-webhook-ensure-dry:
	@if [ -z "$(BASE_URL)" ]; then \
		echo "❌ BASE_URL is required. Example: make razorpay-webhook-ensure-dry BASE_URL=https://manual-talent2.apps.hexmos.com MODE=test"; \
		exit 1; \
	fi
	@MODE_VALUE="$(MODE)"; \
	if [ -z "$$MODE_VALUE" ]; then MODE_VALUE="$${RAZORPAY_MODE:-live}"; fi; \
	python3 scripts/razorpay_webhook_ensure.py --base-url "$(BASE_URL)" --mode "$$MODE_VALUE" --dry-run $(ARGS)