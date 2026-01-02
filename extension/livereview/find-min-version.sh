#!/usr/bin/env bash
set -e

TMP=$(mktemp -d)
cp package.json package-lock.json "$TMP"

cleanup() {
  rm -rf node_modules
  cp "$TMP/package.json" .
  cp "$TMP/package-lock.json" .
  rm -rf "$TMP"
}
trap cleanup EXIT

for v in 1.70.0 1.75.0 1.80.0 1.85.0 1.90.0 1.95.0 1.100.0; do
  echo "Testing @types/vscode@$v"
  npm install -D @types/vscode@$v --no-save --silent
  if npm run check-types > /dev/null 2>&1; then
    echo "✔ Minimum API >= $v"
    exit 0
  else
    echo "✖ Fails at $v"
  fi
done

echo "No compatible version found"
exit 1

