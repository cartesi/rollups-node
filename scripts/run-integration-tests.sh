#!/bin/bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)
#
# Entrypoint for the integration-test container.
# The node process is started and managed by TestMain in the Go test suite,
# so this script only needs to set up PATH and run the tests.
#
# Usage: run-integration-tests.sh

set -eu

export PATH="/opt/go/bin:/build/cartesi/go/rollups-node:$PATH"

# Smoke-check: verify the required binaries are on PATH.
which cartesi-rollups-cli || { echo "ERROR: cartesi-rollups-cli not found on PATH"; exit 1; }
which cartesi-rollups-node || { echo "ERROR: cartesi-rollups-node not found on PATH"; exit 1; }
if ! command -v cartesi-rollups-machine-tool >/dev/null 2>&1; then
  make cartesi-rollups-machine-tool
fi
which cartesi-rollups-machine-tool || { echo "ERROR: cartesi-rollups-machine-tool not found on PATH"; exit 1; }

# Print the node log on exit so it appears in docker compose logs.
NODE_LOG="${CARTESI_TEST_NODE_LOG_FILE:-}"
if [ -n "$NODE_LOG" ]; then
  trap 'echo "=== NODE LOG ==="; cat "$NODE_LOG" 2>/dev/null || true' EXIT
fi

# Timeout must be less than the CI job timeout-minutes (60) to produce
# a useful go test panic instead of an abrupt CI kill.
if command -v gotestsum >/dev/null 2>&1; then
  gotestsum --format testdox -- -count=1 -v -timeout 55m \
    -ldflags "-r /opt/cartesi/lib" \
    -tags=endtoendtests ./test/integration/...
else
  go test -count=1 -v -timeout 55m \
    -ldflags "-r /opt/cartesi/lib" \
    -tags=endtoendtests ./test/integration/...
fi
