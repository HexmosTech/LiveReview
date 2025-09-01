# LiveReview Operations (lrops) Setup Guide

This guide provides the necessary commands to configure your local machine and the remote build server for multi-architecture Docker builds.

The build script (`lrops.py`) is configured to use a Docker buildx builder named `multiplatform-builder`. You must create a builder with this name in both environments.

## 1. Build Server Setup (Remote Machine: `gitlab`)

Get binfmt:

```
docker run --privileged --rm tonistiigi/binfmt --install all
```

Run these commands on your remote build server via SSH (`ssh gitlab`). This creates a builder with the necessary container driver for multi-platform emulation.

```bash
# Create a new docker buildx builder with the docker-container driver
docker buildx create --name multiplatform-builder --driver docker-container --use

# Bootstrap the builder to ensure it's running and check for multi-platform support
# The output should list 'linux/arm64' among the supported platforms.
docker buildx inspect --bootstrap
```

## 2. Local Machine Setup

Run these commands on your local development machine. This ensures that the build script can run consistently in your local environment.

```bash
# (Optional) Remove the old 'gitlab' builder if it exists
docker buildx rm gitlab

# Create a new builder with the same name as the remote server
docker buildx create --name multiplatform-builder --driver docker-container --use

# Bootstrap the builder and verify multi-platform support
docker buildx inspect --bootstrap
```

### Running a Build

Once both environments are configured, you can run a multi-architecture build from your local machine:

```bash
make docker-multiarch
```
