#!/usr/bin/env bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

set -euo pipefail

# Run from the repository root, like the other Makefile lint checks.
golangci-lint run --enable-only forbidigo ./test/lint/testdata/approved

output=$(mktemp)
trap 'rm -f "$output"' EXIT
if golangci-lint run --enable-only forbidigo ./test/lint/testdata/forbidden >"$output" 2>&1; then
    echo "Expected endpoint lint rules to reject the forbidden fixture." >&2
    exit 1
fi

# Check every constructor's diagnostic and source location. A compilation or
# tooling failure, or a rule narrowed to DialContext, must not pass.
for identifier in 'endpoint.Raw' 'rpcAlias.Dial' 'rpcAlias.DialHTTP' 'rpcAlias.DialOptions' \
                  'rpcAlias.DialContext' 'ethAlias.Dial' 'ethAlias.DialContext' \
                  'factoryAlias.NewRepositoryFromConnectionString'; do
    if ! diagnostic=$(grep -F "use of \`$identifier\` forbidden" "$output") ||
       ! grep -Eq 'forbidden.go:[0-9]+:[0-9]+: use of' <<<"$diagnostic"; then
        cat "$output" >&2
        echo "Missing endpoint lint diagnostic for $identifier." >&2
        exit 1
    fi
done
