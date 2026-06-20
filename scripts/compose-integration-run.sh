#!/bin/bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)
#
# Runs the integration-test Compose service inside an isolated Compose
# project and captures all logs (run output, node log, service logs) into a
# single log file. Cleanup is always project-scoped so concurrent shards do
# not interfere with each other.
#
# Usage: compose-integration-run.sh
#
# Environment:
#   COMPOSE_PROJECT   Compose project name (required)
#   INTEGRATION_LOGS  Log file to write (required; truncated at start)
#   TEST_PATTERN      Optional anchored regex selecting a shard of top-level
#                     tests (forwarded to the test container; empty = full suite)
#   SHARD_NAME        Optional shard label (log readability only)
#   NODE_TOPOLOGY     Node deployment topology (standalone | multiprocess);
#                     forwarded to the container, where TestMain starts and
#                     manages the matching node.

set -euo pipefail

COMPOSE_FILE="test/compose/compose.integration.yaml"
NODE_LOG_PATH="/var/lib/cartesi-rollups-node/logs/node.log"

: "${COMPOSE_PROJECT:?COMPOSE_PROJECT is required}"
: "${INTEGRATION_LOGS:?INTEGRATION_LOGS is required}"
export TEST_PATTERN="${TEST_PATTERN:-}"
export SHARD_NAME="${SHARD_NAME:-full}"
export NODE_TOPOLOGY="${NODE_TOPOLOGY:-standalone}"

compose() {
    docker compose -p "$COMPOSE_PROJECT" -f "$COMPOSE_FILE" "$@"
}

remove_integration_run_containers() {
    local ids
    docker rm -f "$COMPOSE_PROJECT-integration-test-run" >/dev/null 2>&1 || true
    ids=$(docker ps -aq \
        --filter "label=com.docker.compose.project=$COMPOSE_PROJECT" \
        --filter "label=com.docker.compose.service=integration-test" \
        2>/dev/null || true)
    if [ -n "$ids" ]; then
        docker rm -f $ids >/dev/null 2>&1 || true
    fi
}

cleanup() {
    # `docker compose run` creates a one-off integration-test container. On
    # Ctrl+C, the compose client can exit before that container is removed; if
    # it still holds the project network/volumes, `down -v` cannot clean them.
    # Remove it before starting the log-copy helper or tearing the project down.
    remove_integration_run_containers

    # The in-container trap already prints the node log into the run output;
    # this volume copy covers abnormal exits (e.g. an OOM-killed container).
    {
        echo
        echo "=== NODE LOG (from volume) ==="
    } >>"$INTEGRATION_LOGS"
    compose run --rm --no-deps --entrypoint cat integration-test \
        "$NODE_LOG_PATH" >>"$INTEGRATION_LOGS" 2>/dev/null || true
    remove_integration_run_containers
    {
        echo
        echo "=== COMPOSE SERVICE LOGS ==="
    } >>"$INTEGRATION_LOGS"
    compose logs --no-color >>"$INTEGRATION_LOGS" 2>&1 || true
    compose down -v --remove-orphans || true
}
trap cleanup EXIT

: >"$INTEGRATION_LOGS"
echo "Running integration tests (project=$COMPOSE_PROJECT shard=$SHARD_NAME topology=$NODE_TOPOLOGY logs=$INTEGRATION_LOGS)"

# Clear a stale one-off from a previously interrupted run of the same project.
remove_integration_run_containers

# pipefail keeps the test exit code authoritative despite the tee.
compose run --name "$COMPOSE_PROJECT-integration-test-run" --rm --remove-orphans integration-test \
    2>&1 | tee -a "$INTEGRATION_LOGS"
