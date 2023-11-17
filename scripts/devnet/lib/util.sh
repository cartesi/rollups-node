#!/usr/bin/env bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

################################################################################
# Utilitary functions
log() {
    echo "$1"
}

err() {
    log "ERROR: $1" >&2
}

verbose() {
    if [[ -n "$VERBOSE" ]]; then
        log "$1"
    fi
}

check_error() {
    exit_code=${1:-0}
    shift
    context=${1:-"main"}

    if [[ $exit_code -ne 0 ]]; then
        err "$context: exit_code=$exit_code"
        exit $exit_code
    fi
}
