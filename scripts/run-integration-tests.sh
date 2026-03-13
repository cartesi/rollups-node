#!/bin/bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)
#
# Entrypoint for the integration-test container.
# Waits for the node to become healthy, starts a background health monitor,
# then runs the Go integration test suite.
#
# Usage: run-integration-tests.sh <node-url>

set -eu

NODE_URL="${1:-http://node:10000}"

echo "Waiting for node to become healthy..."
attempts=0
until curl -sf "${NODE_URL}/readyz"; do
  attempts=$((attempts + 1))
  if [ "$attempts" -ge 60 ]; then
    echo "ERROR: Node failed to become healthy after 120 seconds"
    exit 1
  fi
  sleep 2
done
echo "Node is healthy. Running integration tests..."

# Monitor node health in background; kill the entire process group if
# the node crashes. Uses a failure counter (3 consecutive misses) to
# tolerate transient blips such as GC pauses.
(fail_count=0; while sleep 5; do
  if ! curl -sf "${NODE_URL}/readyz" >/dev/null 2>&1; then
    fail_count=$((fail_count + 1))
    echo "WARNING: Node health check failed ($fail_count/3)"
    if [ "$fail_count" -ge 3 ]; then
      echo "ERROR: Node unhealthy after 3 consecutive checks, aborting tests."
      kill 0 2>/dev/null
      exit 1
    fi
  else
    fail_count=0
  fi
done) &
HEALTH_PID=$!
trap "kill $HEALTH_PID 2>/dev/null" EXIT

export PATH="/opt/go/bin:/build/cartesi/go/rollups-node:$PATH"

# Smoke-check: verify the CLI binary is on PATH before running the suite.
which cartesi-rollups-cli || { echo "ERROR: cartesi-rollups-cli not found on PATH"; exit 1; }

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
