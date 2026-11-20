#!/bin/sh
# Idempotently install/upgrade the codebase-memory-mcp binary (the
# knowledge-graph engine behind blast-radius scoring - see
# internal/blastradius/serverscore/serverscore.go, which resolves it via
# exec.LookPath at runtime). Downloads the pinned release from GitHub and
# places it on PATH. Safe to run on every deploy.
#
# Unlike install-vl-convert.sh, this checks the INSTALLED VERSION against
# the pinned one, not just whether a binary is present - an existing but
# outdated binary is a silent scoring failure (CLI argument/format
# mismatches between an old engine and a newer LiveReview server), the exact
# bug class fixed in git-lrc's self-update/review runtime for the same tool
# (see https://github.com/HexmosTech/git-lrc/issues/151). "A binary exists"
# is not "the right binary exists".
set -e

# Keep in sync with docker/docker-deps.env's CODEBASE_MEMORY_MCP_VERSION -
# this is the non-Docker (raw-deploy) install path for the same pinned tool.
CODEBASE_MEMORY_MCP_VERSION="${CODEBASE_MEMORY_MCP_VERSION:-v0.12.1}"
BIN_DIR="${CODEBASE_MEMORY_MCP_BIN_DIR:-/usr/local/bin}"
TARGET="${BIN_DIR}/codebase-memory-mcp"
WANT_VERSION=$(echo "${CODEBASE_MEMORY_MCP_VERSION}" | sed 's/^v//')

current_version() {
    "${TARGET}" --version 2>/dev/null | awk '{print $NF}'
}

if [ -x "${TARGET}" ]; then
    installed=$(current_version || true)
    if [ "${installed}" = "${WANT_VERSION}" ]; then
        echo "codebase-memory-mcp already at pinned version ${WANT_VERSION}"
        exit 0
    fi
    echo "codebase-memory-mcp present at '${installed:-unknown}', pinned is ${WANT_VERSION} - upgrading"
else
    echo "codebase-memory-mcp not found - installing ${CODEBASE_MEMORY_MCP_VERSION}"
fi

for dep in curl tar install; do
    if ! command -v "${dep}" >/dev/null 2>&1; then
        echo "Error: required command '${dep}' is not installed. Please install it and re-run." >&2
        exit 1
    fi
done

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "${OS}" in
    linux)
        case "${ARCH}" in
            x86_64|amd64) asset_arch="linux-amd64" ;;
            aarch64|arm64) asset_arch="linux-arm64" ;;
            *)
                echo "Unsupported arch: ${ARCH} (linux). Install manually from https://github.com/DeusData/codebase-memory-mcp/releases" >&2
                exit 1
                ;;
        esac
        ;;
    darwin)
        case "${ARCH}" in
            x86_64|amd64) asset_arch="darwin-amd64" ;;
            arm64) asset_arch="darwin-arm64" ;;
            *)
                echo "Unsupported arch: ${ARCH} (darwin)." >&2
                exit 1
                ;;
        esac
        ;;
    *)
        echo "Unsupported OS: ${OS}. Install codebase-memory-mcp manually." >&2
        exit 1
        ;;
esac

url="https://github.com/DeusData/codebase-memory-mcp/releases/download/${CODEBASE_MEMORY_MCP_VERSION}/codebase-memory-mcp-${asset_arch}.tar.gz"
tmpdir=$(mktemp -d)
trap 'rm -rf "${tmpdir}"' EXIT

SUDO=""
if [ "$(id -u)" -ne 0 ] && command -v sudo >/dev/null 2>&1; then
    SUDO="sudo"
fi

echo "Downloading ${url}..."
curl -sSL --fail -o "${tmpdir}/mcp.tar.gz" "${url}"
tar -xzf "${tmpdir}/mcp.tar.gz" -C "${tmpdir}"
${SUDO} mkdir -p "${BIN_DIR}"
${SUDO} install -m 0755 "${tmpdir}/codebase-memory-mcp" "${TARGET}"

echo "Installed: $("${TARGET}" --version 2>&1 || true)"
