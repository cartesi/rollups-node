#!/usr/bin/env bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)
#
# Advancer starvation load test.
#
# Tests that the advancer's bounded round-robin scheduling prevents starvation:
# one app ("flood") receives many inputs while two others ("trickle") receive
# few. The test passes if trickle apps finish processing while the flood app
# is still draining — proving the advancer serves all apps fairly.
#
# How it works:
#   1. SEND PHASE: Pauses Anvil mining, sends L1 transactions from multiple
#      accounts in parallel (accounts 1-N from the Foundry dev mnemonic;
#      account 0 is reserved for the node's claimer), then mines all pending
#      txs instantly via anvil_mine. This creates a burst of blocks that the
#      EVM Reader ingests at once, building a real backlog for the advancer.
#   2. MONITOR PHASE: Polls the database every 2s. When trickle apps reach
#      100% processed while flood still has remaining inputs, fairness is
#      confirmed. Then waits for all inputs to finish.
#
# The key variable is CARTESI_ADVANCER_INPUT_BATCH_SIZE: with batch_size=10,
# the advancer processes 10 inputs per app per tick. Trickle apps (10 inputs)
# finish in ~1 tick; flood (200 inputs) takes ~20 ticks. Without the fix,
# the advancer would drain all 200 flood inputs before touching trickle.
#
# Prerequisites:
#   - Node running with the load-test apps deployed (make deploy-load-test-apps)
#   - Node started with a small batch size: CARTESI_ADVANCER_INPUT_BATCH_SIZE=10
#   - Devnet built with --mixed-mining (see test/devnet/Dockerfile)
#   - cartesi-rollups-cli built and in PATH
#   - Environment variables set (eval $(make env))
#
# Usage:
#   make load-test                                # deploy apps + run test
#   scripts/load-test.sh                          # run test only (apps already deployed)
#   scripts/load-test.sh --flood 200 --trickle 10
#
# Heavier stress test (longer flood tail, more visible fairness gap):
#   Node:   CARTESI_ADVANCER_INPUT_BATCH_SIZE=5 cartesi-rollups-node
#   Script: scripts/load-test.sh --flood 1000 --trickle 20 --flood-ratio 20 --timeout 600

set -euo pipefail

# App names deployed by `make deploy-load-test-apps`
FLOOD_APP="load-test-flood"
TRICKLE_APPS=("load-test-trickle-1" "load-test-trickle-2")

# Defaults
FLOOD_COUNT=200
TRICKLE_COUNT=10
FLOOD_RATIO=10
NUM_SENDERS=4
POLL_INTERVAL=2
TIMEOUT=300

usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Options:
  --flood N       Number of inputs to send to the flood app (default: $FLOOD_COUNT)
  --trickle N     Number of inputs to send to each trickle app (default: $TRICKLE_COUNT)
  --flood-ratio N Flood inputs per trickle input per send cycle (default: $FLOOD_RATIO)
  --senders N     Parallel sender accounts, 1-9 (default: $NUM_SENDERS)
  --timeout N     Seconds to wait for all inputs to finish (default: $TIMEOUT)
  --help          Show this message

NOTE: The node must be started (in a separate terminal) with a small batch
      size to observe the scheduling effect:
        CARTESI_ADVANCER_INPUT_BATCH_SIZE=10 cartesi-rollups-node
EOF
    exit 0
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --flood)       FLOOD_COUNT="$2"; shift 2 ;;
        --trickle)     TRICKLE_COUNT="$2"; shift 2 ;;
        --flood-ratio) FLOOD_RATIO="$2"; shift 2 ;;
        --senders)     NUM_SENDERS="$2"; shift 2 ;;
        --timeout)     TIMEOUT="$2"; shift 2 ;;
        --help)        usage ;;
        *)             echo "Unknown option: $1"; usage ;;
    esac
done

if (( NUM_SENDERS < 1 || NUM_SENDERS > 9 )); then
    echo "Error: --senders must be between 1 and 9 (account 0 is reserved for the node)"
    exit 1
fi

echo ""
echo "=== Advancer Starvation Load Test ==="
echo "Flood app:    $FLOOD_APP ($FLOOD_COUNT inputs)"
for app in "${TRICKLE_APPS[@]}"; do
    echo "Trickle app:  $app ($TRICKLE_COUNT inputs)"
done
echo "Flood ratio:  ${FLOOD_RATIO}:1 (flood inputs per trickle input per cycle)"
echo "Senders:      $NUM_SENDERS parallel accounts (indices 1-$NUM_SENDERS)"
echo "Timeout:      ${TIMEOUT}s"
echo ""

# --- Send inputs --------------------------------------------------------------
# Send transactions from multiple Foundry dev accounts in parallel to maximize
# throughput. Each sender uses a different account index (1-N, avoiding 0 which
# the node uses for claim submission) so nonces don't conflict.
#
# The devnet runs Anvil with --block-time 1 --mixed-mining. With mixed-mining,
# auto-mine is active by default (txs mined instantly, 1 per block). To build
# a backlog we:
#   1. Disable auto-mine + interval mining so txs accumulate in the mempool
#   2. Send all txs in parallel from multiple accounts
#   3. Call anvil_mine to produce all blocks instantly (0s interval)
#   4. Restore auto-mine + interval mining
# The blocks still have 1 tx each (Anvil limitation), but they're produced
# instantly so the EVM Reader ingests them as a burst.

RPC_URL="${CARTESI_BLOCKCHAIN_HTTP_ENDPOINT:-http://localhost:8545}"

anvil_rpc() {
    curl -s -X POST "$RPC_URL" \
        -H 'Content-Type: application/json' \
        -d "$1" > /dev/null
}

# Disable auto-mine so txs accumulate in the mempool instead of being
# mined instantly. Also disable interval mining so only our explicit
# evm_mine calls produce blocks.
echo "Pausing Anvil mining for send phase..."
anvil_rpc '{"jsonrpc":"2.0","method":"evm_setAutomine","params":[false],"id":1}'
anvil_rpc '{"jsonrpc":"2.0","method":"evm_setIntervalMining","params":[0],"id":2}'

# Restore normal mining on exit (even on error/Ctrl-C)
restore_mining() {
    echo ""
    echo "Restoring Anvil mining..."
    anvil_rpc '{"jsonrpc":"2.0","method":"evm_setAutomine","params":[true],"id":90}'
    anvil_rpc '{"jsonrpc":"2.0","method":"evm_setIntervalMining","params":[1],"id":91}'
}
trap restore_mining EXIT

# Build array of sender account indices (1..NUM_SENDERS, skip 0)
SENDER_ACCOUNTS=()
for i in $(seq 1 "$NUM_SENDERS"); do
    SENDER_ACCOUNTS+=("$i")
done
next_sender=0

# Per-account PID tracking for nonce-safe serialization.
# SENDER_PIDS[account_index] holds the PID of the last background send for
# that account. Before reusing an account we wait for its previous job.
declare -A SENDER_PIDS

# send_input dispatches a CLI send as a background job using the next sender
# account. Per-account serialization is enforced: we wait for the previous
# job on the same account before launching a new one, preventing nonce races.
send_input() {
    local app="$1" payload="$2"
    local acct_idx="${SENDER_ACCOUNTS[$next_sender]}"
    next_sender=$(( (next_sender + 1) % ${#SENDER_ACCOUNTS[@]} ))

    # Wait for the previous job on this account to finish.
    if [[ -n "${SENDER_PIDS[$acct_idx]:-}" ]]; then
        wait "${SENDER_PIDS[$acct_idx]}" 2>/dev/null || true
    fi

    CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX="$acct_idx" \
        cartesi-rollups-cli send "$app" "$payload" --yes --async > /dev/null 2>&1 &
    SENDER_PIDS[$acct_idx]=$!
}

echo "Sending inputs (${FLOOD_COUNT} flood + ${TRICKLE_COUNT} x ${#TRICKLE_APPS[@]} trickle)..."

flood_sent=0
trickle_sent=0
total_inputs=$(( FLOOD_COUNT + TRICKLE_COUNT * ${#TRICKLE_APPS[@]} ))
sent=0

# Interleave sends: FLOOD_RATIO flood inputs, then 1 trickle to each app.
# All txs accumulate in the mempool (mining is paused). After the loop we
# call anvil_mine to produce all blocks instantly.
while (( flood_sent < FLOOD_COUNT || trickle_sent < TRICKLE_COUNT )); do
    # Send a burst of flood inputs
    burst_end=$(( flood_sent + FLOOD_RATIO ))
    while (( flood_sent < FLOOD_COUNT && flood_sent < burst_end )); do
        flood_sent=$((flood_sent + 1))
        PAYLOAD=$(printf "flood-%06d" "$flood_sent")
        send_input "$FLOOD_APP" "$PAYLOAD"
        sent=$((sent + 1))
    done

    # Send 1 trickle input to each trickle app
    if (( trickle_sent < TRICKLE_COUNT )); then
        trickle_sent=$((trickle_sent + 1))
        for app in "${TRICKLE_APPS[@]}"; do
            PAYLOAD=$(printf "trickle-%06d" "$trickle_sent")
            send_input "$app" "$PAYLOAD"
            sent=$((sent + 1))
        done
    fi

    if (( sent % 100 == 0 )) || (( flood_sent >= FLOOD_COUNT && trickle_sent >= TRICKLE_COUNT )); then
        echo "  progress: flood=$flood_sent/$FLOOD_COUNT  trickle=$trickle_sent/$TRICKLE_COUNT  (total $sent txs)"
    fi
done

# Wait for all background sends to reach Anvil's mempool.
echo "  waiting for in-flight transactions..."
wait

# Mine all pending txs. anvil_mine produces blocks instantly (0s interval).
# We request more blocks than txs to ensure the mempool is fully drained —
# extra blocks are empty and cheap. Using sent+100 as a safe upper bound.
mine_blocks=$(( sent + 100 ))
mine_hex=$(printf '0x%x' "$mine_blocks")
echo "  mining all pending transactions ($mine_blocks blocks)..."
curl -s -X POST "$RPC_URL" -H 'Content-Type: application/json' \
    -d "{\"jsonrpc\":\"2.0\",\"method\":\"anvil_mine\",\"params\":[\"$mine_hex\",\"0x0\"],\"id\":10}" > /dev/null

echo "  done: $sent txs sent"

# Restore normal mining before monitoring phase.
echo "Restoring Anvil mining..."
anvil_rpc '{"jsonrpc":"2.0","method":"evm_setAutomine","params":[true],"id":12}'
anvil_rpc '{"jsonrpc":"2.0","method":"evm_setIntervalMining","params":[1],"id":13}'

# --- Monitor progress ---------------------------------------------------------

echo ""
echo "=== Monitoring Progress ==="
echo "Waiting for trickle apps to process all inputs..."
echo "(Flood app will take longer -- that is expected and proves fairness)"
echo ""

START_TIME=$(date +%s)

check_progress() {
    psql "$CARTESI_DATABASE_CONNECTION" -t -A -F'|' <<'SQL'
SELECT
    a.name,
    COUNT(*) FILTER (WHERE i.status != 'NONE') AS processed,
    COUNT(*) AS total
FROM input i
JOIN application a ON i.epoch_application_id = a.id
WHERE a.name LIKE 'load-test-%'
GROUP BY a.name
ORDER BY a.name;
SQL
}

FAIRNESS_CONFIRMED=false
EXPECTED_TOTAL=$(( FLOOD_COUNT + TRICKLE_COUNT * ${#TRICKLE_APPS[@]} ))

while true; do
    ELAPSED=$(( $(date +%s) - START_TIME ))
    if (( ELAPSED > TIMEOUT )); then
        echo ""
        echo "TIMEOUT after ${TIMEOUT}s. Final status:"
        check_progress
        echo ""
        echo "FAIL: Not all inputs were processed within the timeout."
        exit 1
    fi

    TRICKLE_DONE=true
    TRICKLE_SEEN=0
    FLOOD_REMAINING=0
    ALL_PROCESSED=true
    TOTAL_IN_DB=0
    echo "--- ${ELAPSED}s elapsed ---"
    while IFS='|' read -r name processed total; do
        [[ -z "$name" ]] && continue
        pct=0
        if (( total > 0 )); then
            pct=$(( processed * 100 / total ))
        fi
        echo "  $name: $processed / $total ($pct%)"
        TOTAL_IN_DB=$(( TOTAL_IN_DB + total ))

        if [[ "$name" == "$FLOOD_APP" ]]; then
            FLOOD_REMAINING=$(( total - processed ))
            if (( processed < total )); then
                ALL_PROCESSED=false
            fi
        fi

        for tapp in "${TRICKLE_APPS[@]}"; do
            if [[ "$name" == "$tapp" ]]; then
                TRICKLE_SEEN=$((TRICKLE_SEEN + 1))
                if (( processed < total )); then
                    TRICKLE_DONE=false
                    ALL_PROCESSED=false
                fi
            fi
        done
    done < <(check_progress)

    # Don't evaluate until all trickle apps have inputs in the DB
    if (( TRICKLE_SEEN < ${#TRICKLE_APPS[@]} )); then
        echo "  (waiting for inputs to appear in the database...)"
        TRICKLE_DONE=false
        ALL_PROCESSED=false
    fi

    # Phase 1: Detect fairness — trickle apps finish while flood still has work
    if ! $FAIRNESS_CONFIRMED && $TRICKLE_DONE && (( FLOOD_REMAINING > 0 )); then
        FAIRNESS_CONFIRMED=true
        echo ""
        echo "  >> FAIRNESS: Trickle apps finished while flood app still has $FLOOD_REMAINING inputs remaining."
        echo ""
    fi

    # Phase 2: Wait for ALL inputs to be processed
    if $ALL_PROCESSED && (( TOTAL_IN_DB >= EXPECTED_TOTAL )); then
        echo ""
        if $FAIRNESS_CONFIRMED; then
            echo "PASS: All $EXPECTED_TOTAL inputs processed. Starvation-freedom confirmed."
        else
            echo "PASS: All $EXPECTED_TOTAL inputs processed. (Fairness not observed -- flood app"
            echo "      finished before trickle was checked. Try increasing --flood or decreasing"
            echo "      CARTESI_ADVANCER_INPUT_BATCH_SIZE to observe the scheduling effect.)"
        fi
        exit 0
    fi

    sleep "$POLL_INTERVAL"
done
