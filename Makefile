.PHONY: build build-prod run-review run-review-verbose test clean develop develop-reflex river-deps river-install river-migrate river-setup river-ui-install river-ui install-vl-convert db-flip version version-bump version-patch version-minor version-major version-bump-dirty version-patch-dirty version-minor-dirty version-major-dirty version-bump-dry version-patch-dry version-minor-dry version-major-dry build-versioned docker-build docker-build-push docker-build-dry docker-interactive docker-interactive-push docker-interactive-dry docker-build docker-build-push docker-build-versioned docker-build-push-versioned docker-build-dry docker-build-push-dry docker-multiarch docker-multiarch-push docker-multiarch-dry docker-interactive-multiarch docker-interactive-multiarch-push cplrops vendor-prompts-encrypt vendor-prompts-build vendor-prompts-rebuild vendor-docker-build vendor-docker-build-dry vendor-docker-build-push vendor-docker-multiarch-dry vendor-docker-multiarch-push run-debug run-fast logrun api-with-migrations build-with-ui security-sbom security-sbom-cyclonedx security-sbom-spdx security-sbom-validate release-notes-init release-notes-check release-preflight release-gh niceurl niceurl2 run-api run-worker prep-dbctx
.PHONY: upload-secrets download-secrets list-secrets-files legacy-secrets-clear generate-openapi prep-training-data check-training-data
.PHONY: razorpay-webhook-ensure razorpay-webhook-ensure-dry razorpay-verify-plans razorpay-verify-plans-low-pricing
.PHONY: raw-deploy raw-deploy-low-pricing raw-deploy-backend raw-deploy-backend-low-pricing build-staging-with-ui raw-deploy-staging stop-staging
.PHONY: dev dev-up dev-down dev-restart dev-status dev-attach

# ============================================================================
# Environment switching
# ============================================================================

.PHONY: switch-env-prod switch-env-prod-low-pricing switch-env-selfhosted-local switch-env-selfhosted-docker switch-env-cloud-local which-env

define SWITCH_ENV
	@if [ ! -f ".env.$(1)" ]; then \
		echo "ERROR: .env.$(1) not found"; \
		exit 1; \
	fi
	@if [ -f .env ]; then cp .env .env.bak && echo "Backed up .env -> .env.bak"; fi
	@cp .env.$(1) .env
	@echo "$(1)" > .current-env
	@echo "Switched to $(1) (.env.$(1) -> .env)"
endef

switch-env-prod:
	$(call SWITCH_ENV,prod)

switch-env-prod-low-pricing:
	$(call SWITCH_ENV,prod-low-pricing)

switch-env-selfhosted-local:
	$(call SWITCH_ENV,selfhosted.local)

switch-env-selfhosted-docker:
	$(call SWITCH_ENV,selfhosted)

switch-env-cloud-local:
	$(call SWITCH_ENV,cloud.local)

which-env:
	@if [ ! -f .current-env ]; then \
		echo "No .current-env found. Run 'make switch-env-<name>' first."; \
	else \
		echo "Current env: $$(cat .current-env)"; \
	fi

# ============================================================================
# Docker management (requires selfhosted.docker env)
# ============================================================================

define CHECK_DOCKER_ENV
	@if [ ! -f .current-env ] || [ "$$(cat .current-env)" != "selfhosted.docker" ]; then \
		echo "ERROR: Current env is not selfhosted.docker. Run: make switch-env-selfhosted-docker"; \
		exit 1; \
	fi
endef

.PHONY: docker-local-start docker-local-rebuild docker-local-stop

docker-local-start:
	$(CHECK_DOCKER_ENV)
	docker compose up -d

docker-local-rebuild:
	$(CHECK_DOCKER_ENV)
	docker compose up -d --build

docker-local-stop:
	docker compose down

# Go parameters
GOENV=env -u GOROOT
GOCMD=$(GOENV) go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
BINARY_NAME=livereview
REQUIRED_GO_VERSION=$(shell awk '/^go /{print $$2; exit}' go.mod)
REQUIRED_GO_TOOLCHAIN_VER=$(shell go version | awk '{print substr($$3,3)}')
REQUIRED_GO_SERIES=$(shell echo $(REQUIRED_GO_VERSION) | awk -F. '{print $$1"."$$2}')
GOVULNCHECK_VERSION=v1.1.4
GOVULNCHECK_CMD=GOTOOLCHAIN=go$(REQUIRED_GO_TOOLCHAIN_VER) $(GOCMD) run -a golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
GH_REPO=HexmosTech/LiveReview
GH=/usr/bin/gh
GHSM_SCRIPT=scripts/ghsm.py
LEGACY_ENV_VARS=DATABASE_URL JWT_SECRET LIVEREVIEW_BACKEND_PORT LIVEREVIEW_FRONTEND_PORT LIVEREVIEW_REVERSE_PROXY LIVEREVIEW_IS_CLOUD CLOUD_JWT_SECRET FW_PARSE_ADMIN_SECRET RAZORPAY_MODE LIVEREVIEW_PRICING_PROFILE RAZORPAY_WEBHOOK_SECRET RAZORPAY_TEST_KEY RAZORPAY_TEST_SECRET RAZORPAY_TEST_MONTHLY_PLAN_ID_USD RAZORPAY_TEST_YEARLY_PLAN_ID_USD RAZORPAY_TEST_MONTHLY_PLAN_ID_INR RAZORPAY_TEST_YEARLY_PLAN_ID_INR RAZORPAY_LIVE_KEY RAZORPAY_LIVE_SECRET RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID_USD RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID_USD RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID_INR RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID_INR RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID_USD RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID_USD RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID_INR RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID_INR DISCORD_SIGNUP_WEBHOOK_URL OVSX_PAT
DEPLOY_ACTUAL_ENV_FILE=.env.prod
DEPLOY_LOW_PRICING_ENV_FILE=.env.prod.low-pricing
DEPLOY_PLAN_CATALOG_FILE=config/plan_catalog.json
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

# Content hash of ui/docs/training_data/ as of the last `make prep-training-data`
# run. Kept up to date automatically by that target (scripts/prep_training_data.sh
# rewrites this line). `make check-training-data` compares it against the
# corpus's current hash and re-runs prep-training-data when they diverge.
TRAINING_DATAgs_HASH=0d62f21d8a1b7bdb5951511afde5e85cc8b5dd8854a354f42d1f54074e66a8c9

# Load environment variables from .env file
-include .env
export

build:
	rm $(BINARY_NAME) || true
	$(GOBUILD) -o $(BINARY_NAME)

build-prod:
	rm $(BINARY_NAME) || true
	$(GOBUILD) -tags production -o $(BINARY_NAME)
# Minimal CI build
build-ci:
	rm -f $(BINARY_NAME)
	SKIP_TYPED_GEN=1 go build -tags=ci -o livereview .

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

# Email template preview
email-preview:
	$(GOCMD) run cmd/email-preview/main.go

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

run-debug: check-training-data
	pkill -9 livereview || true
	@DLV_BIN_DIR=$$($(GOCMD) env GOBIN); \
	if [ -z "$$DLV_BIN_DIR" ]; then DLV_BIN_DIR="$$($(GOCMD) env GOPATH)/bin"; fi; \
	command -v dlv >/dev/null 2>&1 || { \
		echo "Installing Delve with Go $(REQUIRED_GO_VERSION)..."; \
		GOTOOLCHAIN=go$(REQUIRED_GO_TOOLCHAIN_VER) $(GOCMD) install github.com/go-delve/delve/cmd/dlv@latest; \
	}; \
	if ! $(GOCMD) version -m "$$DLV_BIN_DIR/dlv" 2>/dev/null | grep -q "go$(REQUIRED_GO_SERIES)"; then \
		echo "Rebuilding Delve with Go $(REQUIRED_GO_VERSION) for DWARFv5+ compatibility..."; \
		GOTOOLCHAIN=go$(REQUIRED_GO_TOOLCHAIN_VER) $(GOCMD) install github.com/go-delve/delve/cmd/dlv@latest; \
	fi
	which air || $(GOCMD) install github.com/air-verse/air@latest
	DLV_BIN_DIR=$$($(GOCMD) env GOBIN); if [ -z "$$DLV_BIN_DIR" ]; then DLV_BIN_DIR="$$($(GOCMD) env GOPATH)/bin"; fi; PATH="$$DLV_BIN_DIR:$$PATH" air


# Fast dev-run: full compiler optimizations, no Delve. Use this instead of
# `run-debug` when you don't need to attach a debugger - dbctx's
# embedding/scoring code in particular is much slower under run-debug's
# `-gcflags='all=-N -l'`, which disables optimization/inlining across the
# whole binary, not just the code being debugged.
run-fast: check-training-data
	pkill -9 livereview-fast || true
	which air || $(GOCMD) install github.com/air-verse/air@latest
	air -c .air.fast.toml

# Disable Typed OpenAPI schema generation for CI
run-skip-typed:
	SKIP_TYPED_GEN=1 $(MAKE) run-debug

logrun:
	which air || $(GOCMD) install github.com/air-verse/air@latest
	bash -c 'set -o pipefail; air 2>&1 | tee "logrun-$$(date +%Y%m%d-%H%M%S).log"'

develop: check-training-data
	@DLV_BIN_DIR=$$($(GOCMD) env GOBIN); \
	if [ -z "$$DLV_BIN_DIR" ]; then DLV_BIN_DIR="$$($(GOCMD) env GOPATH)/bin"; fi; \
	command -v dlv >/dev/null 2>&1 || { \
		echo "Installing Delve with Go $(REQUIRED_GO_VERSION)..."; \
		GOTOOLCHAIN=go$(REQUIRED_GO_TOOLCHAIN_VER) $(GOCMD) install github.com/go-delve/delve/cmd/dlv@latest; \
	}; \
	if ! $(GOCMD) version -m "$$DLV_BIN_DIR/dlv" 2>/dev/null | grep -q "go$(REQUIRED_GO_SERIES)"; then \
		echo "Rebuilding Delve with Go $(REQUIRED_GO_VERSION) for DWARFv5+ compatibility..."; \
		GOTOOLCHAIN=go$(REQUIRED_GO_TOOLCHAIN_VER) $(GOCMD) install github.com/go-delve/delve/cmd/dlv@latest; \
	fi
	which air || $(GOCMD) install github.com/air-verse/air@latest
	DLV_BIN_DIR=$$($(GOCMD) env GOBIN); if [ -z "$$DLV_BIN_DIR" ]; then DLV_BIN_DIR="$$($(GOCMD) env GOPATH)/bin"; fi; PATH="$$DLV_BIN_DIR:$$PATH" air

develop-reflex: check-training-data
	which reflex || $(GOCMD) install github.com/cespare/reflex@latest
	reflex -r '\.go$$' -s -- sh -c '$(GOENV) go build -o $(BINARY_NAME) && ./$(BINARY_NAME) api'

api-with-migrations:
	dbmate up
	$(GOCMD) run livereview.go api

run-api: build
	./$(BINARY_NAME) api

run-worker: build
	./$(BINARY_NAME) worker

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

# Exclude ./scripts and ./cmd/onboarding-pdf because they contain standalone/ignored build constraint main programs.
SECURITY_GOVULN_PACKAGES := $(filter-out ./scripts ./cmd/onboarding-pdf,$(TEST_PACKAGES))

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
	$(GOCMD) get github.com/riverqueue/river
	$(GOCMD) get github.com/riverqueue/river/riverdriver/riverpgxv5

river-install:
	$(GOCMD) install github.com/riverqueue/river/cmd/river@latest

river-ui-install:
	$(GOCMD) install riverqueue.com/riverui/cmd/riverui@latest

# Install vl-convert binary (Vega-Lite → PNG renderer) for the local OS/arch.
# Downloads the pre-built release from GitHub and places it in /usr/local/bin.
# Requires glibc >= 2.38 on Linux (pre-built against Ubuntu 24.04).
# On older Linux (glibc < 2.38), use Docker or pip install vl-convert-python instead.
VL_CONVERT_VERSION ?= v1.9.0
install-vl-convert:
	@OS=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	ARCH=$$(uname -m); \
	case "$$OS" in \
		linux)  glibc_ver=$$(ldd --version 2>&1 | awk '/GLIBC/{print $$NF; exit}'); \
			$$(expr "$$glibc_ver" \< "2.38" >/dev/null 2>&1) && { \
				echo "Detected GLIBC $$glibc_ver — too old for the pre-built binary (needs >= 2.38)."; \
				echo "Use one of these alternatives:"; \
				echo "  • pip install vl-convert-python        (native Python wheel)"; \
				echo "  • cargo install vl-convert             (build from source, requires Rust)"; \
				exit 1; \
			}; \
			case "$$ARCH" in \
				x86_64|amd64) asset="vl-convert_linux-64.zip" ;; \
				aarch64|arm64) asset="vl-convert_linux-aarch64.zip" ;; \
				*) echo "Unsupported arch: $$ARCH"; exit 1 ;; \
			esac ;; \
		darwin) case "$$ARCH" in \
					x86_64) asset="vl-convert_osx-64.zip" ;; \
					arm64)  asset="vl-convert_osx-arm64.zip" ;; \
					*) echo "Unsupported arch: $$ARCH"; exit 1 ;; \
				esac ;; \
		mingw*|msys*|cygwin*) asset="vl-convert_win-64.zip" ;; \
		*) echo "Unsupported OS: $$OS"; exit 1 ;; \
	esac; \
	url="https://github.com/vega/vl-convert/releases/download/$(VL_CONVERT_VERSION)/$$asset"; \
	echo "Downloading $$asset..."; \
	curl -sL --fail "$$url" -o /tmp/vl-convert.zip && \
	unzip -o /tmp/vl-convert.zip -d /tmp/vl-convert-extracted && \
	sudo mkdir -p /usr/local/lib/vl-convert && \
	sudo cp /tmp/vl-convert-extracted/bin/vl-convert /usr/local/bin/ && \
	sudo cp /tmp/vl-convert-extracted/bin/LICENSE /tmp/vl-convert-extracted/bin/thirdparty_* /usr/local/lib/vl-convert/ 2>/dev/null; \
	rm -rf /tmp/vl-convert.zip /tmp/vl-convert-extracted && \
	echo "Installed: $$(/usr/local/bin/vl-convert --version 2>&1 || true)"

river-migrate:
	river migrate-up --database-url "$(DATABASE_URL)"

river-ui:
	@echo "Starting River UI with DATABASE_URL: $(DATABASE_URL)"
	DATABASE_URL="$(DATABASE_URL)" riverui

staging-river-ui:
	@if [ ! -f $(DEPLOY_STAGING_ENV_FILE) ]; then \
		echo "❌ ERROR: $(DEPLOY_STAGING_ENV_FILE) not found"; \
		exit 1; \
	fi
	@echo "Starting River UI with Staging DATABASE_URL..."
	@set -a && . ./$(DEPLOY_STAGING_ENV_FILE) && set +a && DATABASE_URL="$$DATABASE_URL" riverui

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

# Build dbctx index from local .env DATABASE_URL and import terminology
prep-dbctx: export PATH := $(HOME)/go/bin:$(PATH)
prep-dbctx:
	@if [ -z "$(DATABASE_URL)" ]; then \
		echo "❌ ERROR: DATABASE_URL not set. Check your .env file."; \
		exit 1; \
	fi
	-rm -f $(HOME)/livereview.dtx $(HOME)/livereview.dtx-wal $(HOME)/livereview.dtx-shm $(HOME)/livereview.dtx-journal
	@DBURL=$$(echo '$(DATABASE_URL)' | sed 's/?sslmode=.*//'); \
	echo "🔨 Building dbctx index from $$(echo "$$DBURL" | sed -E 's#(://[^:]+:)[^@]+(@)#\1***\2#')..."; \
	dbctx build "$$DBURL" --output $(HOME)/livereview.dtx
	@echo "📦 Importing terminology..."
	dbctx terminology import $(HOME)/livereview.dtx ./internal/mcpagent/terminology.json
	@echo "✅ dbctx index ready at $(HOME)/livereview.dtx"

# Pull Markdown docs from git-lrc, git-lrc wiki, LiveReview, and LiveReview
# wiki into ui/docs/training_data/ (RAG corpus for the chatbot). Leaves
# ui/docs/training_data/lr_routes/ (hand-written route docs) untouched.
prep-training-data:
	@bash scripts/prep_training_data.sh

# Re-fetches ui/docs/training_data/ whenever its content has drifted from
# TRAINING_DATA_HASH above (corpus missing, never fetched, or stale). Cheap
# no-op when nothing changed. Wired as a prerequisite of the dev-server
# targets below so the RAG corpus is never silently out of date.
check-training-data:
	@current="$$(bash scripts/training_data_hash.sh)"; \
	if [ "$$current" != "$(TRAINING_DATA_HASH)" ]; then \
		echo "[check-training-data] out of date (recorded: $(TRAINING_DATA_HASH), actual: $$current) - running prep-training-data..."; \
		$(MAKE) prep-training-data; \
	else \
		echo "[check-training-data] up to date ($$current)"; \
	fi

# Generate a token-compact schema dump of the prod DB (public schema) for LLM context.
.PHONY: compressed-schema
compressed-schema:
	@if [ ! -f .env.prod ]; then \
		echo "❌ ERROR: .env.prod not found"; \
		exit 1; \
	fi
	@mkdir -p db
	@set -a && . ./.env.prod && set +a && python3 scripts/llm-schema.py db/schema-compressed.txt
	@echo "✅ Wrote db/schema-compressed.txt"

# Export a full snapshot of the prod DB (schema + data) using .env.prod's
# DATABASE_URL, for restoring into a separate, non-prod Postgres instance to
# test against locally without pointing a dev server at prod itself. pg_dump
# is read-only against its source by construction - it only ever issues SELECT-
# shaped queries - so this cannot write to or modify the prod database.
.PHONY: prod-data-export
prod-data-export:
	@if [ ! -f .env.prod ]; then \
		echo "❌ ERROR: .env.prod not found"; \
		exit 1; \
	fi
	@command -v pg_dump >/dev/null 2>&1 || { echo "❌ ERROR: pg_dump not found - install postgresql-client"; exit 1; }
	@mkdir -p db/prod-exports
	@set -a && . ./.env.prod && set +a && \
	OUT="db/prod-exports/prod-$$(date +%Y%m%d-%H%M%S).dump" && \
	echo "Exporting prod data (read-only pg_dump) to $$OUT ..." && \
	pg_dump "$$DATABASE_URL" --format=custom --no-owner --no-privileges --verbose \
	  --exclude-table-data=review_events \
	  --exclude-table-data=upgrade_request_events \
	  --exclude-table-data=river_job \
	  --file="$$OUT" && \
	echo "✅ Wrote $$OUT" && \
	echo "Now run: make prod-data-import"

# Restores the most recent db/prod-exports/*.dump into your LOCAL Postgres
# server (via `sudo -u postgres`, i.e. the local Unix socket - it never
# connects to prod). Target database/user come from THIS repo's local .env
# DATABASE_URL. Interactive: asks for sudo's password if needed, asks before
# dropping an existing local database, and asks before creating a new local
# role (showing the username/password it's about to create). See
# scripts/prod-data-import.sh for the exact sequence.
.PHONY: prod-data-import
prod-data-import:
	@if [ ! -f .env ]; then \
		echo "❌ ERROR: .env not found"; \
		exit 1; \
	fi
	@command -v pg_restore >/dev/null 2>&1 || { echo "❌ ERROR: pg_restore not found - install postgresql-client"; exit 1; }
	@bash scripts/prod-data-import.sh

# Export then import in one go. Two separate $(MAKE) calls, not a
# multi-prerequisite target, so they run strictly in order even under `make
# -j` - import must never start before export has actually finished writing
# the dump file.
.PHONY: prod-data-sync
prod-data-sync:
	@$(MAKE) prod-data-export
	@$(MAKE) prod-data-import

# ============================================================================
# Enterprise-Selfhosted: local rapid dev (no Docker rebuild needed)
# ============================================================================

# Restore prod dump into the enterprise-selfhosted database (.env.selfhosted.local)
.PHONY: prod-data-import-enterprise-selfhosted
prod-data-import-enterprise-selfhosted:
	@if [ ! -f .env.selfhosted.local ]; then \
		echo "❌ ERROR: .env.selfhosted.local not found"; \
		exit 1; \
	fi
	@command -v pg_restore >/dev/null 2>&1 || { echo "❌ ERROR: pg_restore not found - install postgresql-client"; exit 1; }
	@bash scripts/prod-data-import-enterprise-selfhosted.sh

# Apply selfhosted transformations (plan, billing, dev user) to enterprise-selfhosted DB
.PHONY: prod-data-transform-selfhosted
prod-data-transform-selfhosted:
	@if [ ! -f .env.selfhosted.local ]; then \
		echo "❌ ERROR: .env.selfhosted.local not found"; \
		exit 1; \
	fi
	@bash scripts/prod-data-transform-selfhosted.sh

# Full sync: export from prod → import into enterprise-selfhosted → apply transformations
.PHONY: prod-data-sync-enterprise-selfhosted
prod-data-sync-enterprise-selfhosted:
	@$(MAKE) prod-data-export
	@$(MAKE) prod-data-import-enterprise-selfhosted
	@$(MAKE) prod-data-transform-selfhosted

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

# SYNC-LROPS: push the working-copy lrops.sh to a test host and install it as the
# real /usr/local/bin/lrops.sh (not a side copy), so what you test is what ships.
# Also re-extracts the embedded helper scripts and reverse-proxy templates into an
# existing ~/livereview, since those are written at install time and would otherwise
# stay stale after a script update.
#   make sync-lrops                  # defaults to host 'nats03'
#   make sync-lrops LROPS_HOST=myhost
LROPS_HOST ?= nats03
.PHONY: sync-lrops
sync-lrops: ## Deploy lrops.sh to $(LROPS_HOST):/usr/local/bin/lrops.sh
	@echo "→ syncing lrops.sh to $(LROPS_HOST)"
	@rsync -az lrops.sh $(LROPS_HOST):/tmp/lrops-sync.sh
	@ssh $(LROPS_HOST) '\
		sudo install -m 755 -o root -g root /tmp/lrops-sync.sh /usr/local/bin/lrops.sh && rm -f /tmp/lrops-sync.sh; \
		if [ -d "$$HOME/livereview" ]; then \
			for t in setup-ssl.sh backup.sh restore.sh renew-ssl.sh; do \
				sed -n "/^# === DATA:$$t ===/,/^# === END:$$t ===/p" /usr/local/bin/lrops.sh \
					| grep -v "^# === " > /tmp/$$t 2>/dev/null && \
					[ -s /tmp/$$t ] && install -m 755 /tmp/$$t "$$HOME/livereview/scripts/$$t"; \
				rm -f /tmp/$$t; \
			done; \
			for t in nginx.conf.example caddy.conf.example apache.conf.example; do \
				sed -n "/^# === DATA:$$t ===/,/^# === END:$$t ===/p" /usr/local/bin/lrops.sh \
					| grep -v "^# === " > /tmp/$$t 2>/dev/null && \
					[ -s /tmp/$$t ] && install -m 644 /tmp/$$t "$$HOME/livereview/config/$$t"; \
				rm -f /tmp/$$t; \
			done; \
			echo "  re-extracted helper scripts and proxy templates into $$HOME/livereview"; \
		else \
			echo "  no ~/livereview on $(LROPS_HOST) - script installed, nothing to re-extract"; \
		fi'
	@printf "  local:  %s\n" "$$(md5sum lrops.sh | cut -d" " -f1)"
	@printf "  remote: %s\n" "$$(ssh $(LROPS_HOST) 'md5sum /usr/local/bin/lrops.sh' | cut -d' ' -f1)"
	@ssh $(LROPS_HOST) 'lrops.sh --version | head -1'

# PURGE-LROPS: wipe a LiveReview install off a test host so the next run starts from a
# genuine blank slate. Removes containers, images, the install dir (including root's, if a
# sudo run created one), and any reverse-proxy vhost LiveReview wrote.
# Deliberately KEPT: /usr/local/bin/lrops.sh (so you can reinstall) and any Let's Encrypt
# certificates (re-issuing burns the 5-per-week duplicate limit).
#   make purge-lr-nats03
.PHONY: purge-lr-nats03
purge-lr-nats03: ## Remove LiveReview containers/images/config from $(LROPS_HOST)
	@echo "→ purging LiveReview from $(LROPS_HOST)"
	@ssh $(LROPS_HOST) '\
		if [ -f "$$HOME/livereview/docker-compose.yml" ]; then \
			(cd "$$HOME/livereview" && docker compose down -v --remove-orphans >/dev/null 2>&1) || true; \
		fi; \
		docker rm -f livereview-app livereview-db >/dev/null 2>&1 || true; \
		imgs=$$(docker images --format "{{.Repository}}:{{.Tag}}" | grep -E "hexmostech/livereview|^postgres:15-alpine$$" || true); \
		[ -n "$$imgs" ] && docker rmi -f $$imgs >/dev/null 2>&1 || true; \
		sudo rm -rf "$$HOME/livereview" "$$HOME"/livereview.backup.* "$$HOME"/livereview.removed.* "$$HOME/livereview_env" "$$HOME/lrops.sh"; \
		sudo sh -c "rm -rf /root/livereview /root/livereview.backup.* /root/livereview.removed.*"; \
		sudo rm -f /etc/nginx/sites-enabled/livereview.conf /etc/nginx/sites-available/livereview.conf \
			/etc/nginx/sites-available/livereview /etc/nginx/conf.d/livereview* ; \
		sudo sh -c "rm -f /etc/nginx/sites-available/*lrbak* /etc/nginx/sites-available/livereview.conf.bak.*"; \
		sudo rm -f /etc/caddy/conf.d/livereview.caddy; \
		sudo rm -f /etc/apache2/sites-enabled/livereview.conf /etc/apache2/sites-available/livereview.conf; \
		if command -v nginx >/dev/null 2>&1 && sudo nginx -t >/dev/null 2>&1; then \
			sudo systemctl reload nginx >/dev/null 2>&1 || sudo systemctl start nginx >/dev/null 2>&1 || true; \
		fi'
	@echo "→ verifying"
	@ssh $(LROPS_HOST) '\
		echo "  containers: $$(docker ps -a --filter name=livereview --format "{{.Names}}" | tr "\n" " ")"; \
		echo "  images:     $$(docker images --format "{{.Repository}}:{{.Tag}}" | grep -cE "hexmostech/livereview|postgres:15-alpine") remaining"; \
		echo "  install:    $$( [ -d "$$HOME/livereview" ] && echo PRESENT || echo removed )"; \
		echo "  nginx:      $$(sudo ls /etc/nginx/sites-enabled/ 2>/dev/null | tr "\n" " ")"; \
		echo "  script:     $$(lrops.sh --version 2>/dev/null | head -1)"'
	@echo "kept on purpose: /usr/local/bin/lrops.sh and any Let's Encrypt certificates"

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
	@PIDS="$$(lsof -tiTCP:20000 -sTCP:LISTEN 2>/dev/null || true) $$(pgrep -f '^/usr/lib/autossh/autossh -M 20000 ' || true)"; \
	PIDS="$$(printf '%s\n' $$PIDS | tr ' ' '\n' | awk 'NF' | sort -u | tr '\n' ' ')"; \
	if [ -n "$$PIDS" ]; then \
		echo "Stopping existing local autossh/ssh for niceurl: $$PIDS"; \
		kill -9 $$PIDS || true; \
	fi
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
	@PIDS="$$(lsof -tiTCP:20001 -sTCP:LISTEN 2>/dev/null || true) $$(pgrep -f '^/usr/lib/autossh/autossh -M 20001 ' || true)"; \
	PIDS="$$(printf '%s\n' $$PIDS | tr ' ' '\n' | awk 'NF' | sort -u | tr '\n' ' ')"; \
	if [ -n "$$PIDS" ]; then \
		echo "Stopping existing local autossh/ssh for niceurl2: $$PIDS"; \
		kill -9 $$PIDS || true; \
	fi
	@ssh root@master "PID=\$$( netstat -tulpn | grep :6544 | awk '{print \$$7}' | cut -d/ -f1 | head -n 1); [ -n \"\$$PID\" ] && kill -9 \$$PID || true" || true
	@echo "Starting autossh reverse tunnel on remote port 6544 -> localhost:8081"
	@AUTOSSH_GATETIME=0 AUTOSSH_POLL=60 AUTOSSH_FIRST_POLL=30 AUTOSSH_LOGLEVEL=6 autossh -M 20001 \
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
	@PIDS="$$(lsof -tiTCP:20002 -sTCP:LISTEN 2>/dev/null || true) $$(pgrep -f '^/usr/lib/autossh/autossh -M 20002 ' || true)"; \
	PIDS="$$(printf '%s\n' $$PIDS | tr ' ' '\n' | awk 'NF' | sort -u | tr '\n' ' ')"; \
	if [ -n "$$PIDS" ]; then \
		echo "Stopping existing local autossh/ssh for niceurl3: $$PIDS"; \
		kill -9 $$PIDS || true; \
	fi
	@ssh root@master "PID=\$$( netstat -tulpn | grep :6545 | awk '{print \$$7}' | cut -d/ -f1 | head -n 1); [ -n \"\$$PID\" ] && kill -9 \$$PID || true" || true
	@echo "Starting autossh reverse tunnel on remote port 6545 -> localhost:8081"
	@AUTOSSH_GATETIME=0 AUTOSSH_POLL=60 AUTOSSH_FIRST_POLL=30 AUTOSSH_LOGLEVEL=6 autossh -M 20002 \
		-o ServerAliveInterval=30 \
		-o ServerAliveCountMax=3 \
		-o TCPKeepAlive=yes \
		-o ExitOnForwardFailure=yes \
		-o ConnectTimeout=10 \
		-o ConnectionAttempts=3 \
		-R 6545:localhost:8081 root@master -N

niceurl4:
	@command -v autossh >/dev/null 2>&1 || { \
		echo "autossh is not installed. Install it with: sudo apt install autossh"; \
		exit 1; \
	}
	@PIDS="$$(lsof -tiTCP:20003 -sTCP:LISTEN 2>/dev/null || true) $$(pgrep -f '^/usr/lib/autossh/autossh -M 20003 ' || true)"; \
	PIDS="$$(printf '%s\n' $$PIDS | tr ' ' '\n' | awk 'NF' | sort -u | tr '\n' ' ')"; \
	if [ -n "$$PIDS" ]; then \
		echo "Stopping existing local autossh/ssh for niceurl4: $$PIDS"; \
		kill -9 $$PIDS || true; \
	fi
	@ssh root@master "PID=\$$( netstat -tulpn | grep :6546 | awk '{print \$$7}' | cut -d/ -f1 | head -n 1); [ -n \"\$$PID\" ] && kill -9 \$$PID || true" || true
	@echo "Starting autossh reverse tunnel on remote port 6546 -> localhost:8081"
	@AUTOSSH_GATETIME=0 AUTOSSH_POLL=60 AUTOSSH_FIRST_POLL=30 AUTOSSH_LOGLEVEL=6 autossh -M 20003 \
		-o ServerAliveInterval=30 \
		-o ServerAliveCountMax=3 \
		-o TCPKeepAlive=yes \
		-o ExitOnForwardFailure=yes \
		-o ConnectTimeout=10 \
		-o ConnectionAttempts=3 \
		-R 6546:localhost:8081 root@master -N

niceurl5:
	@command -v autossh >/dev/null 2>&1 || { \
		echo "autossh is not installed. Install it with: sudo apt install autossh"; \
		exit 1; \
	}
	@PIDS="$$(lsof -tiTCP:20004 -sTCP:LISTEN 2>/dev/null || true) $$(pgrep -f '^/usr/lib/autossh/autossh -M 20004 ' || true)"; \
	PIDS="$$(printf '%s\n' $$PIDS | tr ' ' '\n' | awk 'NF' | sort -u | tr '\n' ' ')"; \
	if [ -n "$$PIDS" ]; then \
		echo "Stopping existing local autossh/ssh for niceurl5: $$PIDS"; \
		kill -9 $$PIDS || true; \
	fi
	@ssh root@master "PID=\$$( netstat -tulpn | grep :6547 | awk '{print \$$7}' | cut -d/ -f1 | head -n 1); [ -n \"\$$PID\" ] && kill -9 \$$PID || true" || true
	@echo "Starting autossh reverse tunnel on remote port 6547 -> localhost:8081"
	@AUTOSSH_GATETIME=0 AUTOSSH_POLL=60 AUTOSSH_FIRST_POLL=30 AUTOSSH_LOGLEVEL=6 autossh -M 20004 \
		-o ServerAliveInterval=30 \
		-o ServerAliveCountMax=3 \
		-o TCPKeepAlive=yes \
		-o ExitOnForwardFailure=yes \
		-o ConnectTimeout=10 \
		-o ConnectionAttempts=3 \
		-R 6547:localhost:8081 root@master -N

build-with-ui:
	@echo "🔨 Building for PRODUCTION deployment (is_cloud=true)"
	@if [ ! -f .env.prod ]; then \
		echo "❌ ERROR: .env.prod not found! Cannot build for production."; \
		exit 1; \
	fi
	rm $(BINARY_NAME) || true
	cd ui/ && npm install && set -a && . ./.env.prod && set +a && LIVEREVIEW_BUILD_MODE=prod NODE_ENV=production npm run build:obfuscated && cd ..
	$(GOBUILD) -tags production -o $(BINARY_NAME) .
	@echo "✅ Production build complete. Binary ready for raw-deploy."

# Define API source files for spec generation
API_SPEC_INPUTS := typed.yaml $(shell find internal/api pkg/models -name "*.go" | grep -v "internal/api/docs/spec.go")


# Typed configuration
TYPED_VERSION=latest
TYPED_BIN_DIR=$(shell go env GOBIN)
ifeq ($(TYPED_BIN_DIR),)
TYPED_BIN_DIR=$(shell go env GOPATH)/bin
endif

typed-install:
	@PATH="$(TYPED_BIN_DIR):$$PATH" command -v typed >/dev/null 2>&1 || { \
		echo "⚙️  'typed' not found."; \
		echo "   Installing the OpenAPI spec generation tool used to generate docs/openapi.yaml..."; \
		GOTOOLCHAIN=go$(REQUIRED_GO_VERSION) go install github.com/d1vbyz3r0/typed/cmd/typed@$(TYPED_VERSION) || exit 1; \
		echo "✅ typed installed successfully."; \
	}

	@PATH="$(TYPED_BIN_DIR):$$PATH" typed --help >/dev/null 2>&1 || { \
		echo "❌ Unable to access 'typed'."; \
		echo "   'typed' is required to generate the OpenAPI specification (docs/openapi.yaml)."; \
		echo "   Please install it manually using the official installation commands:"; \
		echo ""; \
		echo "   go install github.com/d1vbyz3r0/typed/cmd/typed@latest"; \
		echo "   go get github.com/d1vbyz3r0/typed@latest"; \
		echo ""; \
		exit 1; \
	}

docs/openapi.yaml internal/api/docs/spec.go: $(API_SPEC_INPUTS) typed-install
	@echo "🔄 Generating API specification..."
	@mkdir -p docs internal/api/docs
	@chmod 755 docs internal/api/docs
	@PATH="$(TYPED_BIN_DIR):$$PATH" typed -config typed.yaml > /tmp/lr_typed_build.log 2>&1 || (echo "❌ Typed generation failed. Logs:" && cat /tmp/lr_typed_build.log && exit 1)
	@$(GOCMD) run internal/api/docs/spec.go > /tmp/lr_spec_build.log 2>&1 || (echo "❌ OpenAPI spec generation failed. Logs:" && cat /tmp/lr_spec_build.log && exit 1)
	@python3 scripts/openapi/fix-openapi-spec.py docs/openapi.yaml


generate-openapi: docs/openapi.yaml

raw-deploy: build-with-ui
	@echo "🚀 Deploying to production server..."
	@if [ ! -f ./livereview ]; then \
		echo "❌ ERROR: livereview binary not found! Run 'make build-with-ui' first."; \
		exit 1; \
	fi
	@if [ ! -f ./$(DEPLOY_PLAN_CATALOG_FILE) ]; then \
		echo "❌ ERROR: $(DEPLOY_PLAN_CATALOG_FILE) not found"; \
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
	MONTHLY_PLAN_ID_USD=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID_USD=/{print $$2}' $(DEPLOY_ACTUAL_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	YEARLY_PLAN_ID_USD=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID_USD=/{print $$2}' $(DEPLOY_ACTUAL_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	MONTHLY_PLAN_ID_INR=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID_INR=/{print $$2}' $(DEPLOY_ACTUAL_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	YEARLY_PLAN_ID_INR=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID_INR=/{print $$2}' $(DEPLOY_ACTUAL_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	if [ -z "$$MONTHLY_PLAN_ID_USD" ] || [ -z "$$YEARLY_PLAN_ID_USD" ] || [ -z "$$MONTHLY_PLAN_ID_INR" ] || [ -z "$$YEARLY_PLAN_ID_INR" ]; then \
		echo "❌ ERROR: raw-deploy requires RAZORPAY_LIVE_ACTUAL_*_PLAN_ID_{USD,INR} in $(DEPLOY_ACTUAL_ENV_FILE)"; \
		exit 1; \
	fi
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && mv ./livereview ./livereview.bak || true"
	rsync -avz ./livereview db-ready.sh ecosystem.config.js deps.sh install-vl-convert.sh $(DEPLOY_HOST):$(DEPLOY_PATH)/
	rsync -avz ./$(DEPLOY_ACTUAL_ENV_FILE) $(DEPLOY_HOST):$(DEPLOY_PATH)/.env
	ssh $(DEPLOY_HOST) "mkdir -p $(DEPLOY_PATH)/config"
	rsync -avz ./$(DEPLOY_PLAN_CATALOG_FILE) $(DEPLOY_HOST):$(DEPLOY_PATH)/$(DEPLOY_PLAN_CATALOG_FILE)
	rsync -avz ./db/ $(DEPLOY_HOST):$(DEPLOY_PATH)/db/
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && chmod a+x db-ready.sh install-vl-convert.sh && ./install-vl-convert.sh && ./db-ready.sh"
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && pm2 reload ecosystem.config.js --update-env"
	@echo "✅ Production deployment complete!"

DEPLOY_STAGING_ENV_FILE=.env.staging
DEPLOY_STAGING_PATH=/home/ubuntu/staging_lr
DEPLOY_STAGING_HOST=nats03-do

build-staging-with-ui:
	@echo "🔨 Building for STAGING deployment"
	@if [ ! -f .env.staging ]; then \
		echo "❌ ERROR: .env.staging not found! Cannot build for staging."; \
		exit 1; \
	fi
	rm $(BINARY_NAME) || true
	cd ui/ && npm install && set -a && . ../.env.staging && set +a && LIVEREVIEW_BUILD_MODE=staging NODE_ENV=production npm run build:obfuscated && cd ..
	$(GOBUILD) -o $(BINARY_NAME) .
	@echo "✅ Staging build complete. Binary ready for raw-deploy-staging."

raw-deploy-staging: build-staging-with-ui
	@echo "🚀 Deploying to staging server..."
	@if [ ! -f ./livereview ]; then \
		echo "❌ ERROR: livereview binary not found!"; \
		exit 1; \
	fi
	@if [ ! -f ./$(DEPLOY_PLAN_CATALOG_FILE) ]; then \
		echo "❌ ERROR: $(DEPLOY_PLAN_CATALOG_FILE) not found"; \
		exit 1; \
	fi
	@if [ ! -f $(DEPLOY_STAGING_ENV_FILE) ]; then \
		echo "❌ ERROR: $(DEPLOY_STAGING_ENV_FILE) not found"; \
		exit 1; \
	fi
	@echo "🔄 Running database migrations from local machine..."
	set -a && . ./$(DEPLOY_STAGING_ENV_FILE) && set +a && dbmate --url "$$DATABASE_URL" up && river migrate-up --database-url "$$DATABASE_URL"
	ssh $(DEPLOY_STAGING_HOST) "mkdir -p $(DEPLOY_STAGING_PATH) && cd $(DEPLOY_STAGING_PATH) && mv ./livereview ./livereview.bak || true"
	rsync -avz ./livereview deps.sh install-vl-convert.sh ecosystem.staging.config.js $(DEPLOY_STAGING_HOST):$(DEPLOY_STAGING_PATH)/
	rsync -avz ./$(DEPLOY_STAGING_ENV_FILE) $(DEPLOY_STAGING_HOST):$(DEPLOY_STAGING_PATH)/.env
	ssh $(DEPLOY_STAGING_HOST) "mkdir -p $(DEPLOY_STAGING_PATH)/config $(DEPLOY_STAGING_PATH)/internal/mockllm"
	rsync -avz ./$(DEPLOY_PLAN_CATALOG_FILE) $(DEPLOY_STAGING_HOST):$(DEPLOY_STAGING_PATH)/$(DEPLOY_PLAN_CATALOG_FILE)
	rsync -avz ./internal/mockllm/mockllm.toml $(DEPLOY_STAGING_HOST):$(DEPLOY_STAGING_PATH)/internal/mockllm/mockllm.toml
	ssh $(DEPLOY_STAGING_HOST) "cd $(DEPLOY_STAGING_PATH) && chmod a+x install-vl-convert.sh && ./install-vl-convert.sh"
	ssh $(DEPLOY_STAGING_HOST) "bash -ic 'cd $(DEPLOY_STAGING_PATH) && pm2 reload ecosystem.staging.config.js --update-env || pm2 start ecosystem.staging.config.js'"
	@echo "✅ Staging deployment complete!"

stop-staging:
	@echo "🛑 Stopping staging processes on server..."
	ssh $(DEPLOY_STAGING_HOST) "bash -ic 'cd $(DEPLOY_STAGING_PATH) && pm2 delete ecosystem.staging.config.js || true'"
	@echo "✅ Staging processes stopped and removed from PM2!"

raw-deploy-low-pricing: build-with-ui
	@echo "🚀 Deploying to production server with LOW pricing profile..."
	@if [ ! -f ./livereview ]; then \
		echo "❌ ERROR: livereview binary not found! Run 'make build-with-ui' first."; \
		exit 1; \
	fi
	@if [ ! -f ./$(DEPLOY_PLAN_CATALOG_FILE) ]; then \
		echo "❌ ERROR: $(DEPLOY_PLAN_CATALOG_FILE) not found"; \
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
	ACTUAL_MONTHLY_PLAN_ID_USD=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID_USD=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	ACTUAL_YEARLY_PLAN_ID_USD=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID_USD=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	ACTUAL_MONTHLY_PLAN_ID_INR=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID_INR=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	ACTUAL_YEARLY_PLAN_ID_INR=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID_INR=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	MONTHLY_PLAN_ID_USD=$$(awk -F= '/^RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID_USD=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	YEARLY_PLAN_ID_USD=$$(awk -F= '/^RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID_USD=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	MONTHLY_PLAN_ID_INR=$$(awk -F= '/^RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID_INR=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	YEARLY_PLAN_ID_INR=$$(awk -F= '/^RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID_INR=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	if [ -z "$$MONTHLY_PLAN_ID_USD" ] || [ -z "$$YEARLY_PLAN_ID_USD" ] || [ -z "$$MONTHLY_PLAN_ID_INR" ] || [ -z "$$YEARLY_PLAN_ID_INR" ]; then \
		echo "❌ ERROR: raw-deploy-low-pricing requires RAZORPAY_LIVE_LOW_PRICING_*_PLAN_ID_{USD,INR} in $(DEPLOY_LOW_PRICING_ENV_FILE)"; \
		exit 1; \
	fi; \
	if [ "$$MONTHLY_PLAN_ID_USD" = "$$ACTUAL_MONTHLY_PLAN_ID_USD" ] || [ "$$YEARLY_PLAN_ID_USD" = "$$ACTUAL_YEARLY_PLAN_ID_USD" ] || [ "$$MONTHLY_PLAN_ID_INR" = "$$ACTUAL_MONTHLY_PLAN_ID_INR" ] || [ "$$YEARLY_PLAN_ID_INR" = "$$ACTUAL_YEARLY_PLAN_ID_INR" ]; then \
		echo "❌ ERROR: raw-deploy-low-pricing requires low-pricing Razorpay plan IDs to differ from actual profile IDs for both USD and INR in $(DEPLOY_LOW_PRICING_ENV_FILE)"; \
		exit 1; \
	fi
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && mv ./livereview ./livereview.bak || true"
	rsync -avz ./livereview db-ready.sh ecosystem.config.js deps.sh install-vl-convert.sh $(DEPLOY_HOST):$(DEPLOY_PATH)/
	rsync -avz ./$(DEPLOY_LOW_PRICING_ENV_FILE) $(DEPLOY_HOST):$(DEPLOY_PATH)/.env
	ssh $(DEPLOY_HOST) "mkdir -p $(DEPLOY_PATH)/config"
	rsync -avz ./$(DEPLOY_PLAN_CATALOG_FILE) $(DEPLOY_HOST):$(DEPLOY_PATH)/$(DEPLOY_PLAN_CATALOG_FILE)
	rsync -avz ./db/ $(DEPLOY_HOST):$(DEPLOY_PATH)/db/
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && chmod a+x db-ready.sh install-vl-convert.sh && ./install-vl-convert.sh && ./db-ready.sh"
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && pm2 reload ecosystem.config.js --update-env"
	@echo "✅ Production deployment complete!"

raw-deploy-backend:
	@echo "🚀 Deploying to production server..."
	$(GOBUILD) -tags production livereview.go
	@if [ ! -f ./livereview ]; then \
		echo "❌ ERROR: livereview binary not found! Run 'make build-with-ui' first."; \
		exit 1; \
	fi
	@if [ ! -f ./$(DEPLOY_PLAN_CATALOG_FILE) ]; then \
		echo "❌ ERROR: $(DEPLOY_PLAN_CATALOG_FILE) not found"; \
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
	fi; \
	MONTHLY_PLAN_ID_USD=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID_USD=/{print $$2}' $(DEPLOY_ACTUAL_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	YEARLY_PLAN_ID_USD=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID_USD=/{print $$2}' $(DEPLOY_ACTUAL_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	MONTHLY_PLAN_ID_INR=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID_INR=/{print $$2}' $(DEPLOY_ACTUAL_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	YEARLY_PLAN_ID_INR=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID_INR=/{print $$2}' $(DEPLOY_ACTUAL_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	if [ -z "$$MONTHLY_PLAN_ID_USD" ] || [ -z "$$YEARLY_PLAN_ID_USD" ] || [ -z "$$MONTHLY_PLAN_ID_INR" ] || [ -z "$$YEARLY_PLAN_ID_INR" ]; then \
		echo "❌ ERROR: raw-deploy-backend requires RAZORPAY_LIVE_ACTUAL_*_PLAN_ID_{USD,INR} in $(DEPLOY_ACTUAL_ENV_FILE)"; \
		exit 1; \
	fi
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && mv ./livereview ./livereview.bak || true"
	rsync -avz ./livereview db-ready.sh ecosystem.config.js deps.sh install-vl-convert.sh $(DEPLOY_HOST):$(DEPLOY_PATH)/
	rsync -avz ./$(DEPLOY_ACTUAL_ENV_FILE) $(DEPLOY_HOST):$(DEPLOY_PATH)/.env
	ssh $(DEPLOY_HOST) "mkdir -p $(DEPLOY_PATH)/config"
	rsync -avz ./$(DEPLOY_PLAN_CATALOG_FILE) $(DEPLOY_HOST):$(DEPLOY_PATH)/$(DEPLOY_PLAN_CATALOG_FILE)
	rsync -avz ./db/ $(DEPLOY_HOST):$(DEPLOY_PATH)/db/
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && chmod a+x db-ready.sh install-vl-convert.sh && ./install-vl-convert.sh && ./db-ready.sh"
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && pm2 reload ecosystem.config.js --update-env"
	@echo "✅ Production deployment complete!"

raw-deploy-backend-low-pricing:
	@echo "🚀 Deploying backend with LOW pricing profile..."
	$(GOBUILD) -tags production livereview.go
	@if [ ! -f ./livereview ]; then \
		echo "❌ ERROR: livereview binary not found! Run 'make build-with-ui' first."; \
		exit 1; \
	fi
	@if [ ! -f ./$(DEPLOY_PLAN_CATALOG_FILE) ]; then \
		echo "❌ ERROR: $(DEPLOY_PLAN_CATALOG_FILE) not found"; \
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
	fi; \
	ACTUAL_MONTHLY_PLAN_ID_USD=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID_USD=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	ACTUAL_YEARLY_PLAN_ID_USD=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID_USD=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	ACTUAL_MONTHLY_PLAN_ID_INR=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID_INR=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	ACTUAL_YEARLY_PLAN_ID_INR=$$(awk -F= '/^RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID_INR=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	MONTHLY_PLAN_ID_USD=$$(awk -F= '/^RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID_USD=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	YEARLY_PLAN_ID_USD=$$(awk -F= '/^RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID_USD=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	MONTHLY_PLAN_ID_INR=$$(awk -F= '/^RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID_INR=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	YEARLY_PLAN_ID_INR=$$(awk -F= '/^RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID_INR=/{print $$2}' $(DEPLOY_LOW_PRICING_ENV_FILE) | tail -n 1 | tr -d "'\"[:space:]"); \
	if [ -z "$$MONTHLY_PLAN_ID_USD" ] || [ -z "$$YEARLY_PLAN_ID_USD" ] || [ -z "$$MONTHLY_PLAN_ID_INR" ] || [ -z "$$YEARLY_PLAN_ID_INR" ]; then \
		echo "❌ ERROR: raw-deploy-backend-low-pricing requires RAZORPAY_LIVE_LOW_PRICING_*_PLAN_ID_{USD,INR} in $(DEPLOY_LOW_PRICING_ENV_FILE)"; \
		exit 1; \
	fi; \
	if [ "$$MONTHLY_PLAN_ID_USD" = "$$ACTUAL_MONTHLY_PLAN_ID_USD" ] || [ "$$YEARLY_PLAN_ID_USD" = "$$ACTUAL_YEARLY_PLAN_ID_USD" ] || [ "$$MONTHLY_PLAN_ID_INR" = "$$ACTUAL_MONTHLY_PLAN_ID_INR" ] || [ "$$YEARLY_PLAN_ID_INR" = "$$ACTUAL_YEARLY_PLAN_ID_INR" ]; then \
		echo "❌ ERROR: raw-deploy-backend-low-pricing requires low-pricing Razorpay plan IDs to differ from actual profile IDs for both USD and INR in $(DEPLOY_LOW_PRICING_ENV_FILE)"; \
		exit 1; \
	fi
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && mv ./livereview ./livereview.bak || true"
	rsync -avz ./livereview db-ready.sh ecosystem.config.js deps.sh install-vl-convert.sh $(DEPLOY_HOST):$(DEPLOY_PATH)/
	rsync -avz ./$(DEPLOY_LOW_PRICING_ENV_FILE) $(DEPLOY_HOST):$(DEPLOY_PATH)/.env
	ssh $(DEPLOY_HOST) "mkdir -p $(DEPLOY_PATH)/config"
	rsync -avz ./$(DEPLOY_PLAN_CATALOG_FILE) $(DEPLOY_HOST):$(DEPLOY_PATH)/$(DEPLOY_PLAN_CATALOG_FILE)
	rsync -avz ./db/ $(DEPLOY_HOST):$(DEPLOY_PATH)/db/
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && chmod a+x db-ready.sh install-vl-convert.sh && ./install-vl-convert.sh && ./db-ready.sh"
	ssh $(DEPLOY_HOST) "cd $(DEPLOY_PATH) && pm2 reload ecosystem.config.js --update-env"
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

# Stream the production chat_debug.log (requires LIVI_DEBUG_LOG=true on the
# server - see internal/logging/chat_debug_logger.go) live, mirroring it into
# the local chat_debug_logs/ dir as it grows. Runs until Ctrl+C.
chat_debug:
	@echo "📥 Tailing chat_debug.log from $(DEPLOY_HOST) (Ctrl+C to stop)..."
	@mkdir -p chat_debug_logs
	ssh $(DEPLOY_HOST) "tail -n 200 -f $(DEPLOY_PATH)/chat_debug_logs/chat_debug.log" | tee chat_debug_logs/prod_chat_debug.log

run-selfhosted:
	which air || $(GOCMD) install github.com/air-verse/air@latest
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

razorpay-verify-plans:
	@bash ./scripts/verify-razorpay-plans.sh $(DEPLOY_ACTUAL_ENV_FILE)

razorpay-verify-plans-low-pricing:
	@bash ./scripts/verify-razorpay-plans.sh $(DEPLOY_LOW_PRICING_ENV_FILE)

# ============================================================================
# Tmux dev environment (port of VS Code "start all")
# ============================================================================

dev dev-up:
	@./dev up

dev-down:
	@./dev down

# Usage: make dev-restart SVC=api   (or ui, worker, niceurl)
dev-restart:
	@if [ -z "$(SVC)" ]; then \
		echo "Usage: make dev-restart SVC=<api|ui|worker|niceurl>"; \
		exit 1; \
	fi
	@./dev restart $(SVC)

dev-status:
	@./dev status

dev-attach:
	@./dev attach