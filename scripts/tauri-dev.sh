#!/bin/bash
# Start Tauri dev with a configurable port via TAURI_DEV_PORT env var.
# If the port is occupied, auto-increment until a free one is found.
# Usage: TAURI_DEV_PORT=1520 npm run tauri:dev
#        npm run tauri:dev                          (defaults to 1420)

set -euo pipefail

PORT="${TAURI_DEV_PORT:-1420}"
MAX_PORT=$((PORT + 100))

find_free_port() {
  local p=$1
  while [ $p -le $MAX_PORT ]; do
    if ! lsof -iTCP:"$p" -sTCP:LISTEN -P -n 2>/dev/null | grep -q ":$p "; then
      echo $p
      return 0
    fi
    p=$((p + 1))
  done
  return 1
}

FREE_PORT=$(find_free_port "$PORT")
if [ -z "$FREE_PORT" ]; then
  echo "Error: No free port in range $PORT-$MAX_PORT" >&2
  exit 1
fi

if [ "$FREE_PORT" != "$PORT" ]; then
  echo "Port $PORT is in use, using port $FREE_PORT"
fi

CONFIG='{"build":{"devUrl":"http://localhost:'"${FREE_PORT}"'","beforeDevCommand":"npm run dev -- --port '"${FREE_PORT}"'"}}'

npm run tauri dev -- --config "$CONFIG"
