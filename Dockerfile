# Multi-stage Dockerfile for LiveReview
# Creates a lightweight container with UI + Backend

# Stage 1: Build React UI
FROM node:20-alpine AS ui-builder
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
FROM golang:1.26-alpine AS go-builder

# Platform arguments for multi-arch builds
ARG TARGETPLATFORM
ARG TARGETARCH
ARG TARGETOS

WORKDIR /app

# Install build dependencies
RUN echo "🔧 Installing Go build dependencies..." && \
    apk add --no-cache curl git ca-certificates

# Install dbmate for database migrations
RUN echo "📊 Installing dbmate for database migrations..." && \
    DBMATE_ARCH=$(case ${TARGETARCH} in \
        "amd64") echo "amd64" ;; \
        "arm64") echo "arm64" ;; \
        "arm") echo "arm" ;; \
        *) echo "amd64" ;; \
    esac) && \
    curl -fsSL -o /usr/local/bin/dbmate \
    https://github.com/amacneil/dbmate/releases/latest/download/dbmate-linux-${DBMATE_ARCH} && \
    chmod +x /usr/local/bin/dbmate && \
    echo "dbmate installed successfully"

# Install River CLI and UI tools
RUN echo "🌊 Installing River CLI and UI tools..." && \
    go install github.com/riverqueue/river/cmd/river@latest && \
    go install riverqueue.com/riverui/cmd/riverui@latest && \
    echo "River tools installed successfully"

# Copy Go module files and download dependencies
COPY go.mod go.sum ./
RUN echo "📦 Downloading Go dependencies..." && \
    go mod download && go mod verify && \
    echo "Go dependencies downloaded successfully"

# Copy source code
COPY . .

# Copy built UI assets from previous stage
COPY --from=ui-builder /app/ui/dist ./ui/dist

# Build arguments for version injection (will be set by lrops.py)
ARG VERSION=development
ARG BUILD_TIME=unknown
ARG GIT_COMMIT=unknown

# Build the Go binary with version info and embedded UI
RUN echo "🔨 Building Go binary with version: ${VERSION}" && \
    CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build \
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
FROM ubuntu:24.04
LABEL maintainer="LiveReview Team"
LABEL description="LiveReview - AI-powered code review tool"

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
    curl -sL --fail "https://github.com/vega/vl-convert/releases/download/v1.9.0/vl-convert_linux-64.zip" -o /tmp/vl-convert.zip && \
    unzip -o /tmp/vl-convert.zip -d /tmp/vl-convert-extracted && \
    cp /tmp/vl-convert-extracted/bin/vl-convert /usr/local/bin/vl-convert && \
    chmod +x /usr/local/bin/vl-convert && \
    rm -rf /tmp/vl-convert.zip /tmp/vl-convert-extracted && \
    echo "vl-convert installed: $(/usr/local/bin/vl-convert --version 2>&1 || true)"

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
    chmod +x /usr/local/bin/riverui

RUN echo "📋 Final image contents:" && \
    ls -la /app/ && \
    echo "📦 Installed binaries:" && \
    ls -la /usr/local/bin/ && \
    echo "📊 Database migrations:" && \
    ls -la /app/db/migrations/ && \
    echo "Migration count: $(ls /app/db/migrations/*.sql | wc -l)" && \
    echo "✅ LiveReview container build completed successfully!"

# Switch to non-root user
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
