#!/bin/bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)
#
# Verifies that every tracked Go source file carries the license header
# (.github/license-check/header.txt) as a contiguous, in-order block within
# its first lines. The block may be preceded by a "Code generated ... DO NOT
# EDIT." preamble. Generated code that lacks the header entirely is excluded.
#
# Usage: check-license-header.sh

set -euo pipefail

cd "$(dirname "$0")/.."

HEADER_FILE=".github/license-check/header.txt"
TOP_LINES=10

# has_header <file>: succeeds iff the exact header block appears, in order and
# contiguous, somewhere within the first TOP_LINES lines of <file>.
#
# The previous implementation counted how many of the first lines matched any
# header line and compared the count to the header length. That was order- and
# uniqueness-blind: a reversed header passed, and so did a file with the
# copyright line duplicated but the SPDX line missing. We compare positionally
# instead so the header must appear verbatim and in the right order.
has_header() {
    head -n "$TOP_LINES" "$1" | awk -v hdr="$HEADER_FILE" '
        BEGIN { n = 0; while ((getline line < hdr) > 0) h[++n] = line }
        { buf[NR] = $0 }
        END {
            for (i = 1; i + n - 1 <= NR; i++) {
                ok = 1
                for (j = 1; j <= n; j++)
                    if (buf[i + j - 1] != h[j]) { ok = 0; break }
                if (ok) exit 0
            }
            exit 1
        }'
}

fail=0
while IFS= read -r f; do
    if ! has_header "$f"; then
        echo "ERROR: missing or wrong license header: $f" >&2
        fail=1
    fi
done < <(git ls-files '*.go' \
    ':!internal/repository/postgres/db' \
    ':!pkg/contracts' \
    ':!pkg/inspectclient/generated.go')

if [ "$fail" -ne 0 ]; then
    echo "FAIL: license header check failed (expected header below)" >&2
    cat "$HEADER_FILE" >&2
    exit 1
fi

echo "OK: license headers present"
