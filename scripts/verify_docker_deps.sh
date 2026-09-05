#!/usr/bin/env bash
# Smoke-tests every pinned Docker dependency INSIDE a built LiveReview
# image: confirms each tool is present on PATH, is executable, and responds
# to a basic invocation (--version/--help) without crashing. This does NOT
# check version numbers against docker/docker-deps.env - that's
# scripts/check_docker_deps.py's job. This only answers "is it actually
# installed and runnable in the image we just built".
#
# Usage:
#   scripts/verify_docker_deps.sh [IMAGE]
#   make verify-docker-deps [IMAGE=some:tag]
#
# IMAGE defaults to $DOCKER_DEPS_TEST_IMAGE, then to livereview:localtest.
# Exits 0 if every tool responds successfully, 1 otherwise.

set -u

IMAGE="${1:-${DOCKER_DEPS_TEST_IMAGE:-livereview:localtest}}"

if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
    echo "Image '$IMAGE' not found locally."
    echo "Build one first, e.g.:"
    echo "  docker buildx build -f Dockerfile.crosscompile -t $IMAGE --load ."
    echo "(or point at an existing image: make verify-docker-deps IMAGE=your:tag)"
    exit 1
fi

echo "Verifying Docker-dependency binaries inside image: $IMAGE"
echo

# name|command run inside the container (via --entrypoint sh -c "...")
CHECKS=(
    "dbmate|dbmate --version"
    "river|river --help"
    "riverui|riverui --help"
    "vl-convert|vl-convert --version"
    "codebase-memory-mcp|codebase-memory-mcp --version"
    "dbctx|dbctx --help"
    "alaws|alaws --help"
)

fail=0
for entry in "${CHECKS[@]}"; do
    name="${entry%%|*}"
    cmd="${entry#*|}"
    output=$(docker run --rm --entrypoint sh "$IMAGE" -c "$cmd" 2>&1)
    status=$?
    if [ "$status" -eq 0 ]; then
        first_line=$(printf '%s\n' "$output" | head -1)
        printf '  ✓ %-22s %s\n' "$name" "$first_line"
    else
        fail=1
        printf '  ✗ %-22s FAILED (exit %s): %s\n' "$name" "$status" "$cmd"
        printf '%s\n' "$output" | sed 's/^/      /'
    fi
done

echo
if [ "$fail" -eq 0 ]; then
    echo "All Docker dependency binaries are present and respond correctly."
else
    echo "One or more Docker dependency binaries failed to run. See above."
fi
exit "$fail"
