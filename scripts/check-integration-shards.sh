#!/bin/bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)
#
# Shard coverage guard: verifies that every top-level integration test
# belongs to exactly one shard and that every shard matches at least one
# test. Prevents false greens when a new integration test is added but not
# assigned to a shard.
#
# Usage: check-integration-shards.sh <shard>=<regex> [<shard>=<regex> ...]
#
# Test discovery uses `go test -list`, which builds the package and runs
# TestMain (TestMain skips node management when listing). The package has no
# CGo dependency on the Cartesi C library, so a plain Go toolchain suffices.

set -euo pipefail

cd "$(dirname "$0")/.."

if [ "$#" -lt 1 ]; then
    echo "usage: $0 <shard>=<regex> [<shard>=<regex> ...]" >&2
    exit 1
fi

SHARD_NAMES=()
SHARD_REGEXES=()
for arg in "$@"; do
    name="${arg%%=*}"
    regex="${arg#*=}"
    if [ -z "$name" ] || [ -z "$regex" ] || [ "$name" = "$arg" ]; then
        echo "ERROR: malformed shard spec '$arg' (expected <name>=<regex>)" >&2
        exit 1
    fi
    SHARD_NAMES+=("$name")
    SHARD_REGEXES+=("$regex")
done

# Discover top-level tests. Keep the build/list step separate from the grep so
# a build failure surfaces as a build failure — otherwise the grep swallows the
# empty output and the script misreports it as "no tests discovered".
if ! list_output=$(go test -list '^Test' -tags=endtoendtests ./test/integration/... 2>&1); then
    echo "ERROR: failed to list integration tests (build error?):" >&2
    echo "$list_output" >&2
    exit 1
fi

# Filter out the trailing "ok <package>" summary line of -list.
TESTS=$(printf '%s\n' "$list_output" | grep -E '^Test[A-Za-z0-9_]*$' || true)

if [ -z "$TESTS" ]; then
    echo "ERROR: no top-level integration tests discovered" >&2
    exit 1
fi

fail=0

while IFS= read -r t; do
    [ -n "$t" ] || continue
    count=0
    matched=""
    for i in "${!SHARD_NAMES[@]}"; do
        if printf '%s\n' "$t" | grep -Eq -- "${SHARD_REGEXES[$i]}"; then
            count=$((count + 1))
            matched="$matched ${SHARD_NAMES[$i]}"
        fi
    done
    if [ "$count" -eq 0 ]; then
        echo "ERROR: test $t matches no shard" >&2
        fail=1
    elif [ "$count" -gt 1 ]; then
        echo "ERROR: test $t matches multiple shards:$matched" >&2
        fail=1
    fi
done <<<"$TESTS"

for i in "${!SHARD_NAMES[@]}"; do
    if ! printf '%s\n' "$TESTS" | grep -Eq -- "${SHARD_REGEXES[$i]}"; then
        echo "ERROR: shard ${SHARD_NAMES[$i]} (${SHARD_REGEXES[$i]}) matches no tests" >&2
        fail=1
    fi
done

if [ "$fail" -ne 0 ]; then
    echo "FAIL: shard coverage check failed" >&2
    exit 1
fi

echo "OK: $(printf '%s\n' "$TESTS" | wc -l | tr -d ' ') tests covered by ${#SHARD_NAMES[@]} shards"
