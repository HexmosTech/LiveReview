# Multi-stage Dockerfile for LiveReview
# Creates a lightweight container with UI + Backend

# Stage 1: Build React UI
FROM node:18-alpine AS ui-builder
WORKDIR /app/ui

# Copy package files and install dependencies
COPY ui/package*.json ./
RUN echo "📦 Installing UI dependencies..." && \
    npm ci --progress=true

# Copy UI source and build production assets
COPY ui/ ./
RUN echo "🔨 Building UI production assets..." && \
    CI=true NODE_ENV=production npm run build && \
    echo "✅ Webpack build completed successfully"

# Verify build output
RUN echo "✅ Verifying UI build output..." && \
    ls -la dist/ && \
    echo "UI build completed successfully"

# Stage 2: Build Go binary with embedded UI
FROM golang:1.24-alpine AS go-builder
WORKDIR /app

# Install build dependencies
RUN echo "🔧 Installing Go build dependencies..." && \
    apk add --no-cache curl git ca-certificates

# Install dbmate for database migrations
RUN echo "📊 Installing dbmate for database migrations..." && \
    curl -fsSL -o /usr/local/bin/dbmate \
    https://github.com/amacneil/dbmate/releases/latest/download/dbmate-linux-amd64 && \
    chmod +x /usr/local/bin/dbmate && \
    echo "dbmate installed successfully"

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

# Build the Go binary with version info and embedded UI
RUN echo "🔨 Building Go binary with version: ${VERSION}" && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
    -v -o livereview . && \
    echo "Go binary built successfully"

# Verify binary was built
RUN echo "✅ Verifying Go binary..." && \
    ls -la livereview && ./livereview --version

# Stage 3: Create minimal runtime container
FROM alpine:3.18
LABEL maintainer="LiveReview Team"
LABEL description="LiveReview - AI-powered code review tool"

# Install runtime dependencies
RUN echo "🔧 Installing runtime dependencies..." && \
    apk add --no-cache \
    ca-certificates \
    curl \
    postgresql-client \
    tzdata \
    && rm -rf /var/cache/apk/* && \
    echo "Runtime dependencies installed successfully"

# Create non-root user for security
RUN echo "👤 Creating non-root user..." && \
    addgroup -g 1001 -S livereview && \
    adduser -u 1001 -S livereview -G livereview && \
    echo "User 'livereview' created successfully"

# Create directories
RUN echo "📁 Creating application directories..." && \
    mkdir -p /app/db/migrations /app/data /app/logs && \
    chown -R livereview:livereview /app && \
    echo "Directories created and permissions set"

# Copy binaries and config from build stages
COPY --from=go-builder /usr/local/bin/dbmate /usr/local/bin/dbmate
COPY --from=go-builder /app/livereview /app/livereview
COPY --from=go-builder /app/livereview.toml /app/livereview.toml
COPY --from=go-builder /app/db/migrations/ /app/db/migrations/

# Copy the startup script
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

RUN echo "📋 Final image contents:" && \
    ls -la /app/ && \
    echo "✅ LiveReview container build completed successfully!"

# Switch to non-root user
USER livereview
WORKDIR /app

# Expose ports for backend API (8888) and frontend (8081)
EXPOSE 8888 8081

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8888/api/health || exit 1

# Default command - runs the startup script that handles the full initialization sequence
CMD ["/app/docker-entrypoint.sh"]
