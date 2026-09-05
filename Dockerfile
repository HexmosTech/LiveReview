# Multi-stage Dockerfile for LiveReview
# Creates a lightweight container with UI + Backend

# Frozen Docker dependency versions - see docker/docker-deps.env (single source of truth).
# scripts/lrops.py injects these as --build-arg for every build.
ARG NODE_IMAGE_TAG=20.20.2-alpine
ARG GOLANG_IMAGE_TAG=1.26.8-bookworm
ARG DEBIAN_IMAGE_TAG=trixie-20260824-slim

# Stage 1: Build React UI
FROM node:${NODE_IMAGE_TAG} AS ui-builder
WORKDIR /app/ui

# Copy package files and install dependencies
COPY ui/package*.json ./
RUN echo "📦 Installing UI dependencies..." && \
    npm ci --progress=true

# Copy UI source and build production assets
COPY ui/ ./
# Copy .env.selfhosted to parent directory for webpack (self-hosted Docker builds)
COPY .env.selfhosted ../.env.selfhosted

# Build UI with explicit SELFHOSTED mode to ensure is_cloud=false
RUN echo "🔨 Building UI for SELF-HOSTED deployment (is_cloud=false)..." && \
    LIVEREVIEW_BUILD_MODE=selfhosted CI=true NODE_ENV=production npm run build:obfuscated && \
    echo "✅ Webpack build completed successfully"

# Verify build output
RUN echo "✅ Verifying UI build output..." && \
    ls -la dist/ && \
    echo "UI build completed successfully"

# Stage 2: Build Go binary with embedded UI
FROM golang:${GOLANG_IMAGE_TAG} AS go-builder

# Platform arguments for multi-arch builds
ARG TARGETPLATFORM
ARG TARGETARCH
ARG TARGETOS

# Frozen Docker dependency versions - see docker/docker-deps.env
ARG RIVER_VERSION=v0.32.0
ARG RIVERUI_VERSION=v0.19.0
ARG DBMATE_VERSION=v2.35.1

WORKDIR /app

# Install build dependencies
RUN echo "🔧 Installing Go build dependencies..." && \
    apt-get update && \
    apt-get install -y --no-install-recommends curl git ca-certificates gcc && \
    rm -rf /var/lib/apt/lists/*

# Install dbmate for database migrations
RUN echo "📊 Installing dbmate for database migrations..." && \
    DBMATE_ARCH=$(case ${TARGETARCH} in \
        "amd64") echo "amd64" ;; \
        "arm64") echo "arm64" ;; \
        "arm") echo "arm" ;; \
        *) echo "amd64" ;; \
    esac) && \
    curl -fsSL -o /usr/local/bin/dbmate \
    https://github.com/amacneil/dbmate/releases/download/${DBMATE_VERSION}/dbmate-linux-${DBMATE_ARCH} && \
    chmod +x /usr/local/bin/dbmate && \
    echo "dbmate installed successfully"

# Install River CLI and UI tools
RUN echo "🌊 Installing River CLI and UI tools..." && \
    go install github.com/riverqueue/river/cmd/river@${RIVER_VERSION} && \
    go install riverqueue.com/riverui/cmd/riverui@${RIVERUI_VERSION} && \
    echo "River tools installed successfully"

# Copy Go module files and download dependencies
COPY go.mod go.sum ./
RUN echo "📦 Downloading Go dependencies..." && \
    go mod download && go mod verify && \
    echo "Go dependencies downloaded successfully"

# Fetch the chatbot's RAG corpus (internal/docindex/docs/{lrc_wiki,lr_wiki},
# go:embed target) to the exact commits pinned in scripts/docs_sources.env -
# deterministic regardless of what happened to be on the build host. This
# layer is cached against docs_sources.env alone (copied on its own, before
# the rest of the source tree), so unrelated source changes never force a
# refetch - only bumping a pinned commit does. See
# docs/docs-sources-pinning-plan.md.
COPY scripts/docs_sources.env scripts/sync_docs_sources.sh ./scripts/
RUN bash scripts/sync_docs_sources.sh

# Copy source code
COPY . .

# Re-run the sync: the 3 external sources above are already cached (no
# network), this pass only does the free local copy of this repo's own
# docs/ (which didn't exist yet in this stage until the COPY above).
RUN bash scripts/sync_docs_sources.sh

# Copy built UI assets from previous stage
COPY --from=ui-builder /app/ui/dist ./ui/dist

# Build arguments for version injection (will be set by lrops.py)
ARG VERSION=development
ARG BUILD_TIME=unknown
ARG GIT_COMMIT=unknown

# Build the Go binary with version info and embedded UI
RUN echo "🔨 Building Go binary with version: ${VERSION}" && \
    CGO_ENABLED=1 GOOS=linux GOARCH=$TARGETARCH go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT}" \
    -v -o livereview . && \
    echo "Go binary built successfully"

# Verify binary installations and migrations
RUN echo "✅ Verifying installed tools..." && \
    ls -la /usr/local/bin/dbmate && \
    ls -la /go/bin/river && \
    ls -la /go/bin/riverui && \
    ls -la livereview && ./livereview --version && \
    echo "📊 Verifying database migrations..." && \
    ls -la db/migrations/ && \
    echo "Migration count: $(ls db/migrations/*.sql | wc -l)" && \
    echo "All tools and migrations verified successfully"

# Stage 3: Create minimal runtime container
FROM debian:${DEBIAN_IMAGE_TAG}
LABEL maintainer="LiveReview Team"
LABEL description="LiveReview - AI-powered code review tool"

# Frozen Docker dependency versions - see docker/docker-deps.env
ARG VLCONVERT_VERSION=v1.9.0
ARG CODEBASE_MEMORY_MCP_VERSION=v0.10.8
ARG DBCTX_VERSION=v0.1.0
ARG ALAWS_VERSION=v0.1.0

# Install runtime dependencies
RUN echo "🔧 Installing runtime dependencies..." && \
    apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    postgresql-client \
    tzdata \
    unzip \
    && rm -rf /var/lib/apt/lists/* && \
    echo "Runtime dependencies installed successfully"

# Download pre-built vl-convert binary (glibc build, no Python needed)
RUN echo "📥 Downloading vl-convert binary..." && \
    curl -sL --fail "https://github.com/vega/vl-convert/releases/download/${VLCONVERT_VERSION}/vl-convert_linux-64.zip" -o /tmp/vl-convert.zip && \
    unzip -o /tmp/vl-convert.zip -d /tmp/vl-convert-extracted && \
    cp /tmp/vl-convert-extracted/bin/vl-convert /usr/local/bin/vl-convert && \
    chmod +x /usr/local/bin/vl-convert && \
    rm -rf /tmp/vl-convert.zip /tmp/vl-convert-extracted && \
    echo "vl-convert installed: $(/usr/local/bin/vl-convert --version 2>&1 || true)"

# Download codebase-memory-mcp binary
RUN echo "📥 Downloading codebase-memory-mcp binary..." && \
    curl -sL --fail "https://github.com/DeusData/codebase-memory-mcp/releases/download/${CODEBASE_MEMORY_MCP_VERSION}/codebase-memory-mcp-linux-amd64.tar.gz" \
        -o /tmp/mcp.tar.gz && \
    tar -xzf /tmp/mcp.tar.gz -C /usr/local/bin/ && \
    chmod +x /usr/local/bin/codebase-memory-mcp && \
    rm -f /tmp/mcp.tar.gz && \
    echo "codebase-memory-mcp installed: $(/usr/local/bin/codebase-memory-mcp --version 2>&1 || true)"

# Download dbctx binary
RUN echo "📥 Downloading dbctx binary..." && \
    curl -L --fail "https://github.com/shrsv/dbctx/releases/download/${DBCTX_VERSION}/dbctx_linux_amd64.tar.gz" \
        -o /tmp/dbctx.tar.gz && \
    tar -xzf /tmp/dbctx.tar.gz -C /tmp/ && \
    mv /tmp/dbctx_linux_amd64 /usr/local/bin/dbctx && \
    chmod +x /usr/local/bin/dbctx && \
    rm -f /tmp/dbctx.tar.gz && \
    echo "dbctx installed: $(/usr/local/bin/dbctx --help 2>&1 | head -1)"

# Download AgentLaws (alaws) binary
RUN echo "📥 Downloading AgentLaws binary..." && \
    curl -L --fail "https://github.com/shrsv/AgentLaws/releases/download/${ALAWS_VERSION}/alaws_linux_amd64.tar.gz" \
        -o /tmp/alaws.tar.gz && \
    tar -xzf /tmp/alaws.tar.gz -C /tmp/ && \
    mv /tmp/alaws_linux_amd64 /usr/local/bin/alaws && \
    chmod +x /usr/local/bin/alaws && \
    rm -f /tmp/alaws.tar.gz && \
    echo "AgentLaws installed: $(/usr/local/bin/alaws --help 2>&1 | head -1)"

# Create non-root user for security
RUN echo "👤 Creating non-root user..." && \
    groupadd -g 1001 -r livereview && \
    useradd -u 1001 -r -g livereview -d /app -s /sbin/nologin livereview && \
    echo "User 'livereview' created successfully"

# Create directories
RUN echo "📁 Creating application directories..." && \
    mkdir -p /app/db/migrations /app/data /app/logs && \
    chown -R livereview:livereview /app && \
    echo "Directories created and permissions set"

# Copy binaries and config from build stages
COPY --from=go-builder /usr/local/bin/dbmate /usr/local/bin/dbmate
COPY --from=go-builder /go/bin/river /usr/local/bin/river
COPY --from=go-builder /go/bin/riverui /usr/local/bin/riverui
COPY --from=go-builder /app/livereview /app/livereview
COPY --from=go-builder /app/livereview.toml /app/livereview.toml
COPY --from=go-builder /app/db/migrations/ /app/db/migrations/
COPY --from=go-builder /app/config/ /app/config/

# Copy the startup script
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh && \
    chmod +x /usr/local/bin/dbmate && \
    chmod +x /usr/local/bin/river && \
    chmod +x /usr/local/bin/riverui && \
    chmod +x /usr/local/bin/codebase-memory-mcp && \
    chmod +x /usr/local/bin/dbctx && \
    chmod +x /usr/local/bin/alaws

RUN echo "📋 Final image contents:" && \
    ls -la /app/ && \
    echo "📦 Installed binaries:" && \
    ls -la /usr/local/bin/ && \
    echo "📊 Database migrations:" && \
    ls -la /app/db/migrations/ && \
    echo "Migration count: $(ls /app/db/migrations/*.sql | wc -l)" && \
    echo "✅ LiveReview container build completed successfully!"

# nosemgrep: dockerfile.security.missing-user.missing-user
# Runs as root initially, then drops to livereview via entrypoint
USER livereview
WORKDIR /app

# Expose ports for backend API (8888), frontend (8081), and River UI (8080)
EXPOSE 8888 8081 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -fsS http://localhost:8888/health || exit 1

# Ensure Go programs flush logs promptly and without buffering
ENV GIN_MODE=release \
    GODEBUG=madvdontneed=1 \
    STDOUT_LINE_BUFFERED=true

# Default command - runs the startup script that handles the full initialization sequence
CMD ["/app/docker-entrypoint.sh"]
