#!/usr/bin/env bash
# Shared helpers for tmux dev windows
ROOT_DIR="$(cd "$(dirname "$0")/../../" && pwd)"

wait_port() {
  local port="$1"
  while ! (echo >/dev/tcp/127.0.0.1/"$port") 2>/dev/null; do
    sleep 1
  done
}
