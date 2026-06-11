#!/bin/bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)
#
# Entrypoint for the integration-test container.
# The node process is started and managed by TestMain in the Go test suite,
# so this script only needs to set up PATH and run the tests.
#
# Usage: run-integration-tests.sh
#
# Environment:
#   TEST_PATTERN  Optional anchored regex passed to go test -run to select a
#                 shard of top-level tests. Empty means run the full suite.
#   SHARD_NAME    Optional shard label, used only for log readability.

set -euo pipefail

export PATH="/opt/go/bin:/build/cartesi/go/rollups-node:$PATH"

# Smoke-check: verify the required binaries are on PATH.
which cartesi-rollups-cli || { echo "ERROR: cartesi-rollups-cli not found on PATH"; exit 1; }
which cartesi-rollups-node || { echo "ERROR: cartesi-rollups-node not found on PATH"; exit 1; }
if ! command -v cartesi-rollups-machine-tool >/dev/null 2>&1; then
  make cartesi-rollups-machine-tool
fi
which cartesi-rollups-machine-tool || { echo "ERROR: cartesi-rollups-machine-tool not found on PATH"; exit 1; }

NODE_LOG="${CARTESI_TEST_NODE_LOG_FILE:-}"
REPORT="$(mktemp)"

cleanup() {
  # Print the node log on exit so it appears in docker compose logs.
  if [ -n "$NODE_LOG" ]; then
    echo "=== NODE LOG ==="
    cat "$NODE_LOG" 2>/dev/null || true
  fi
  rm -f "$REPORT"
}
trap cleanup EXIT

# A skipped top-level test is not a pass. In the compose/CI topology the node
# is always test-managed, so an entire top-level test skipping (e.g. TestRestart
# deciding the node looks externally managed) means the shard reported success
# without exercising what it exists to cover — a false green. Suite/subtest
# skips are allowed; only whole top-level Test* functions are checked.
report_skips() {
  if [ -n "$1" ]; then
    echo "ERROR: top-level test(s) skipped in shard '${SHARD_NAME:-full}' (a skip is not a pass):" >&2
    echo "$1" | sed 's/^/  - /' >&2
    return 1
  fi
  return 0
}

# Parse skipped top-level tests from a go test -json event stream. The
# "Test":"Test..." match deliberately excludes names containing '/', so
# subtest skips (e.g. TestEchoQuorum/Foo) are ignored.
toplevel_skips_json() {
  grep '"Action":"skip"' "$1" 2>/dev/null \
    | grep -oE '"Test":"Test[A-Za-z0-9_]*"' \
    | sed -E 's/.*"Test":"([^"]*)".*/\1/' \
    | sort -u || true
}

# Parse skipped top-level tests from captured `go test -v` output. Top-level
# SKIP lines start at column 0; subtest SKIP lines are indented.
toplevel_skips_verbose() {
  grep -E '^--- SKIP: Test' "$1" 2>/dev/null \
    | sed -E 's/^--- SKIP: (Test[A-Za-z0-9_]*).*/\1/' \
    | sort -u || true
}

# Shard selection: a non-empty TEST_PATTERN narrows the run to the matching
# top-level tests. Built as a bash array so the pattern is never re-expanded
# by the shell.
GO_TEST_ARGS=(-count=1 -v -timeout 55m -ldflags "-r /opt/cartesi/lib" -tags=endtoendtests)
if [ -n "${TEST_PATTERN:-}" ]; then
  echo "Running integration shard '${SHARD_NAME:-unnamed}' with -run '${TEST_PATTERN}'"
  GO_TEST_ARGS+=(-run "${TEST_PATTERN}")
fi
GO_TEST_ARGS+=(./test/integration/...)

# Timeout must be less than the CI job timeout-minutes (60) to produce
# a useful go test panic instead of an abrupt CI kill.
status=0
if command -v gotestsum >/dev/null 2>&1; then
  # --jsonfile captures the machine-readable event stream alongside the
  # human-readable --format output, so we can post-check for skipped tests.
  gotestsum --jsonfile "$REPORT" --format "${GOTESTSUM_FORMAT:-testdox}" \
    -- "${GO_TEST_ARGS[@]}" || status=$?
  report_skips "$(toplevel_skips_json "$REPORT")" || status=1
else
  go test "${GO_TEST_ARGS[@]}" | tee "$REPORT" || status=$?
  report_skips "$(toplevel_skips_verbose "$REPORT")" || status=1
fi

exit "$status"
