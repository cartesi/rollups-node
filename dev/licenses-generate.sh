#!/usr/bin/env bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)
#
# Write THIRD_PARTY_LICENSES.md to stdout. Invoked by `make THIRD_PARTY_LICENSES.md`.
#
# go-licenses only sees Go modules, and only the ones reachable for the platform
# it happens to run under. Redirecting its report straight over the notices file
# therefore drops both the hand-written entries and any module that is only
# linked on another target. This script keeps that knowledge outside the
# generated file:
#
#   1. run the report once per released target and cgo mode, and merge the
#      results,
#   2. drop the entry go-licenses emits for this module itself,
#   3. apply the corrections listed in dev/licenses-overrides.tsv,
#   4. prepend the hand-written entries in dev/licenses-manual.md.

set -euo pipefail

MODULE=github.com/cartesi/rollups-node
TEMPLATE=dev/licenses.tpl
MANUAL=dev/licenses-manual.md
OVERRIDES=dev/licenses-overrides.tsv

# The platforms binaries are released for. The notices file describes what is
# distributed, and the .deb and the container image are both Linux.
TARGETS="linux/amd64 linux/arm64"

# The machine artifacts are built with cgo and the other artifacts without it,
# and the two builds of a dependency may reach different packages. go-ethereum,
# for one, links libsecp256k1 with cgo and a pure Go secp256k1 without it.
CGO_MODES="1 0"

# Other go-licenses versions classify some licences differently. CI installs
# this version too.
GO_LICENSES_VERSION=v1.6.0
build_info=$(go version -m "$(command -v go-licenses)" 2>/dev/null) || build_info=
if ! grep -Eq "^[[:space:]]+mod[[:space:]]+github.com/google/go-licenses[[:space:]]+$GO_LICENSES_VERSION([[:space:]]|$)" \
	<<<"$build_info"; then
	echo "licenses-generate: go-licenses $GO_LICENSES_VERSION is required" >&2
	exit 1
fi

# go-licenses recognises the standard library by its GOROOT, which is the GOROOT
# of the Go that built it unless GOROOT is set. Use the GOROOT of the go command,
# so that a go-licenses built by another Go installation works too.
GOROOT=$(go env GOROOT)
export GOROOT

# Collect the reports before processing them, so that a go-licenses failure
# stops the script here.
reports=$(
	for target in $TARGETS; do
		for cgo in $CGO_MODES; do
			GOOS="${target%/*}" GOARCH="${target#*/}" CGO_ENABLED=$cgo \
				go-licenses report --template "$TEMPLATE" ./... || exit 1
		done
	done
)

cat "$MANUAL"

# Flatten each entry to a single "name version license url" record.
printf '%s\n' "$reports" |
	awk -v self="$MODULE" '
		/^## / { name = substr($0, 4); next }
		/^\* Version: / { version = substr($0, 12); next }
		/^\* License: / {
			link = substr($0, 12)
			split_at = index(link, "](")
			license = substr(link, 2, split_at - 2)
			url = substr(link, split_at + 2, length(link) - split_at - 2)
			if (name != self)
				printf "%s\t%s\t%s\t%s\n", name, version, license, url
			next
		}
	' |
	# A module reached by more than one target is listed once: deduplicate on
	# the module name, which also restores the generator's own ordering.
	LC_ALL=C sort -t "$(printf '\t')" -k1,1 -u |
	# Render the records back as Markdown, applying the overrides.
	awk -F'\t' -v overrides="$OVERRIDES" '
		BEGIN {
			while ((getline line < overrides) > 0) {
				if (line ~ /^#/ || line ~ /^[ \t]*$/)
					continue
				split(line, field, "\t")
				override_license[field[1]] = field[2]
				override_url[field[1]] = field[3]
				override_note[field[1]] = field[4]
			}
		}
		{
			name = $1; version = $2; license = $3; url = $4; note = ""
			if (name in override_license) {
				license = override_license[name]
				url = override_url[name]
				note = override_note[name]
				gsub(/\{version\}/, version, url)
				gsub(/\{version\}/, version, note)
				applied[name] = 1
			}
			# go-licenses writes Unknown when it cannot classify a licence, or
			# when it cannot resolve the URL, as happens without network access.
			if (license == "Unknown" || url == "Unknown") {
				printf "licenses-generate: unknown licence or URL for %s\n", name > "/dev/stderr"
				failed = 1
			}
			printf "\n## %s\n\n", name
			printf "* Name: %s\n* Version: %s\n* License: [%s](%s)\n", name, version, license, url
			if (note != "")
				printf "* Note: %s\n", note
		}
		END {
			for (name in override_license) {
				if (!(name in applied)) {
					printf "licenses-generate: no entry matches the override for %s\n", name > "/dev/stderr"
					failed = 1
				}
			}
			exit failed
		}
	'
