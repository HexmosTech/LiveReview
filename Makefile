.PHONY: build run-review run-review-verbose test clean develop develop-reflex river-deps river-install river-migrate river-setup river-ui-install river-ui db-flip version version-bump version-patch version-minor version-major version-bump-dirty version-patch-dirty version-minor-dirty version-major-dirty version-bump-dry version-patch-dry version-minor-dry version-major-dry build-versioned docker-build docker-build-push docker-build-dry docker-interactive docker-interactive-push docker-interactive-dry docker-build docker-build-push docker-build-versioned docker-build-push-versioned docker-build-dry docker-build-push-dry docker-multiarch docker-multiarch-push docker-multiarch-dry docker-interactive-multiarch docker-interactive-multiarch-push cplrops vendor-prompts-encrypt vendor-prompts-build vendor-prompts-rebuild vendor-docker-build vendor-docker-build-dry vendor-docker-build-push vendor-docker-multiarch-dry vendor-docker-multiarch-push run logrun build-with-ui

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
BINARY_NAME=livereview

# Load environment variables from .env file
include .env
export

build:
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
	which air || go install github.com/air-verse/air@latest
	air

logrun:
	which air || go install github.com/air-verse/air@latest
	bash -c 'set -o pipefail; air 2>&1 | tee "logrun-$$(date +%Y%m%d-%H%M%S).log"'

develop:
	which air || go install github.com/air-verse/air@latest
	air

develop-reflex:
	which reflex || go install github.com/cespare/reflex@latest
	reflex -r '\.go$$' -s -- sh -c 'go build -o $(BINARY_NAME) && ./$(BINARY_NAME) api'

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
	-path './test' -prune -o \
	-path './tests' -prune -o \
	-type f -name '*.go' -print 2>/dev/null | \
	xargs -n1 dirname | sort -u | tr '\n' ' ')

.PHONY: testall
testall:
	$(GOTEST) -count=1 $(TEST_PACKAGES)

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
	ssh -R 6543:localhost:8081 root@master -N



build-with-ui:
	cd ui/ && npm install && npm run build:obfuscated && cd ..
	go build livereview.go

raw-deploy:
	rsync -avz ./livereview ./.env master:/root/public_lr/

	