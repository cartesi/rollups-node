#!/usr/bin/env bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

readonly DEVNET_ANVIL_IP="0.0.0.0"
readonly DEVNET_ANVIL_TIMEOUT=3

_is_anvil_up() {
    local -n ready=$1

    local result="$(
        curl -X \
            POST \
            -s \
            -H 'Content-Type: application/json' \
            -d '{"jsonrpc":"2.0","id":"1","method":"net_listening","params":[]}' \
            "http://$DEVNET_ANVIL_IP:8545"
    )"

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
        --host $DEVNET_ANVIL_IP \
        --dump-state "$anvil_state_file" \
        > /dev/null &
    local pid=$!

    # check if anvil is up
    sleep $DEVNET_ANVIL_TIMEOUT
    _is_anvil_up is_up
    if [[ "$is_up" != "true" ]]; then
        err "anvil has not started"
        return 2
    fi

    ret="$pid"
}

anvil_down() {
    local anvil_pid="$1"

    verbose "waiting $DEVNET_ANVIL_TIMEOUT seconds before killing anvil..."
    sleep $DEVNET_ANVIL_TIMEOUT
    kill "$anvil_pid"
    check_error $? "failed to kill anvil"
    verbose "killed anvil (pid=$anvil_pid)"
}
