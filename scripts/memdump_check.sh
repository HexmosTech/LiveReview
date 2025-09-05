#!/usr/bin/env bash
set -euo pipefail

echo "[memdump] Using SIGSEGV fallback to produce a core dump (no gdb/gcore)."
ulimit -c unlimited || true

# Build harness if missing
if [ ! -x ./render-smoke ]; then
  echo "[memdump] Building render-smoke with vendor_prompts..."
  go build -tags vendor_prompts -o render-smoke ./cmd/render-smoke
fi

LOOPS=${LOOPS:-500}
echo "[memdump] Starting render-smoke (LOOPS=$LOOPS) ..."
LOOPS=$LOOPS ./render-smoke &
pid=$!
echo $pid > .render_smoke.pid
sleep 1

echo "[memdump] Sending SIGSEGV to PID $pid to trigger core dump..."
kill -SEGV "$pid" || true
sleep 1

echo "[memdump] Looking for core file(s) in CWD..."
shopt -s nullglob
cores=(core core.*)
if [ ${#cores[@]} -eq 0 ]; then
  echo "[memdump] No core file found. Your OS may redirect cores elsewhere (e.g., apport, systemd-coredump)."
  exit 0
fi

echo "[memdump] Grepping dump for raw template markers ({{VAR:) ..."
for c in "${cores[@]}"; do
  echo "-- $c --"
  strings "$c" | grep -n "{{VAR:" || true
done

rm -f .render_smoke.pid || true
exit 0
