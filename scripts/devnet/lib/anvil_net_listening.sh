#!/usr/bin/env bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)
ANVIL_IP_ADDR=${ANVIL_IP_ADDR:-"0.0.0.0"}

curl -X \
    POST \
    -s \
    -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","id":"1","method":"net_listening","params":[]}' \
    "http://$ANVIL_IP_ADDR:8545"
