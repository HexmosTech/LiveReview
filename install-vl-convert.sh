#!/bin/sh
# Idempotently install the vl-convert binary (Vega-Lite -> PNG renderer).
# Downloads the pre-built release from GitHub and places it in /usr/local/bin.
# Safe to run on every deploy; skips if the expected version is already present.
set -e

VL_CONVERT_VERSION="${VL_CONVERT_VERSION:-v1.9.0}"
BIN_DIR="${VL_CONVERT_BIN_DIR:-/usr/local/bin}"
TARGET="${BIN_DIR}/vl-convert"

already_installed=0
if [ -x "${TARGET}" ]; then
    if current=$("${TARGET}" --version 2>/dev/null); then
        echo "vl-convert already present: ${current}"
        already_installed=1
    fi
fi
if [ "${already_installed}" = "1" ]; then
    exit 0
fi

# Ensure required commands are present before attempting an install.
for dep in curl unzip install; do
    if ! command -v "${dep}" >/dev/null 2>&1; then
        echo "Error: required command '${dep}' is not installed. Please install it and re-run." >&2
        exit 1
    fi
done

LIB_DIR="${VL_CONVERT_LIB_DIR:-/usr/local/lib/vl-convert}"

echo "Installing vl-convert ${VL_CONVERT_VERSION} to ${TARGET}..."

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "${OS}" in
    linux)
        case "${ARCH}" in
            x86_64|amd64) asset="vl-convert_linux-64.zip" ;;
            aarch64|arm64) asset="vl-convert_linux-aarch64.zip" ;;
            *)
                echo "Unsupported arch: ${ARCH} (linux). Install manually (pip install vl-convert-python or cargo install vl-convert)." >&2
                exit 1
                ;;
        esac
        ;;
    darwin)
        case "${ARCH}" in
            x86_64) asset="vl-convert_osx-64.zip" ;;
            arm64) asset="vl-convert_osx-arm64.zip" ;;
            *)
                echo "Unsupported arch: ${ARCH} (darwin)." >&2
                exit 1
                ;;
        esac
        ;;
    *)
        echo "Unsupported OS: ${OS}. Install vl-convert manually." >&2
        exit 1
        ;;
esac

url="https://github.com/vega/vl-convert/releases/download/${VL_CONVERT_VERSION}/${asset}"
tmpdir=$(mktemp -d)
trap 'rm -rf "${tmpdir}"' EXIT

echo "Downloading ${url}..."
curl -sSL --fail -o "${tmpdir}/vl-convert.zip" "${url}"
unzip -o "${tmpdir}/vl-convert.zip" -d "${tmpdir}/extracted" >/dev/null
mkdir -p "${BIN_DIR}" "${LIB_DIR}"
install -m 0755 "${tmpdir}/extracted/bin/vl-convert" "${TARGET}"
cp "${tmpdir}/extracted/bin/LICENSE" "${tmpdir}/extracted/bin/thirdparty_"* "${LIB_DIR}/" 2>/dev/null || true

echo "Installed: $("${TARGET}" --version 2>&1 || true)"
echo "Licenses copied to: ${LIB_DIR}"
