.PHONY: build run-review run-review-verbose test clean develop develop-reflex river-deps river-install river-migrate river-setup river-ui-install river-ui db-flip version version-bump version-patch version-minor version-major version-bump-dirty version-patch-dirty version-minor-dirty version-major-dirty version-bump-dry version-patch-dry version-minor-dry version-major-dry build-versioned docker-build docker-build-push docker-build-dry docker-interactive docker-interactive-push docker-interactive-dry docker-build docker-build-push docker-build-versioned docker-build-push-versioned docker-build-dry docker-build-push-dry docker-multiarch docker-multiarch-push docker-multiarch-dry docker-interactive-multiarch docker-interactive-multiarch-push cplrops

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
