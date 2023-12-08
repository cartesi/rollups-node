#!/usr/bin/env bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)
anvil_lib_dir="$( cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEVNET_ANVIL_STATE_INTERVAL=5
DEVNET_ANVIL_TIMEOUT=$(expr $DEVNET_ANVIL_STATE_INTERVAL + 10)
readonly anvil_lib_dir DEVNET_ANVIL_STATE_INTERVAL DEVNET_ANVIL_TIMEOUT

ANVIL_IP_ADDR=${ANVIL_IP_ADDR:-"0.0.0.0"}

_is_anvil_up() {
    local -n ready=$1

    local result
    result=$("$anvil_lib_dir"/anvil_net_listening.sh)

    ready="false"
    if [[ -n "$result" ]]; then
        ready=$(echo "$result" | jq ".result")
    fi
}

anvil_up() {
    local -n ret="$1"
    shift
    local anvil_state_file="$1"

    # check if there's another instance of anvil up listening on the same port
    _is_anvil_up is_up
    if [[ "$is_up" == "true" ]]; then
        err "anvil is already up"
        return 1
    fi

    anvil \
        --host "$ANVIL_IP_ADDR" \
        --dump-state "$anvil_state_file" \
        --state-interval "$DEVNET_ANVIL_STATE_INTERVAL" \
        --silent &
    local pid=$!

    sleep "$DEVNET_ANVIL_TIMEOUT"
    # check if anvil is up
    _is_anvil_up is_up
    if [[ "$is_up" != "true" ]]; then
        err "anvil has not started"
        return 2
    fi

    ret="$pid"
}

anvil_down() {
    local anvil_pid="$1"

    kill "$anvil_pid"
    wait "$anvil_pid"
    check_error "$?" "failed to kill anvil"
    verbose "killed anvil (pid=$anvil_pid)"
}
