#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
TEST_DATA_DIR=$(mktemp -d "${TMPDIR:-/tmp}/zensync-ui.XXXXXX")
trap 'rm -rf "$TEST_DATA_DIR"' EXIT
"${ZENSYNC_GO:-go}" build -o "$TEST_DATA_DIR/zensync" ./cmd/zensync
ZENSYNC_DATA_DIR="$TEST_DATA_DIR" ZENSYNC_LISTEN=127.0.0.1:18081 "$TEST_DATA_DIR/zensync" &
TEST_SERVER_PID=$!
trap 'kill "$TEST_SERVER_PID" 2>/dev/null || true; wait "$TEST_SERVER_PID" 2>/dev/null || true; rm -rf "$TEST_DATA_DIR"' EXIT INT TERM
wait "$TEST_SERVER_PID"
