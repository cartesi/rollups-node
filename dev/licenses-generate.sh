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
#   1. run the report once per released target and merge the results,
#   2. drop the entry go-licenses emits for this module itself,
#   3. apply the corrections listed in dev/licenses-overrides.tsv,
#   4. prepend the hand-written entries in dev/licenses-manual.md.

set -euo pipefail

MODULE=github.com/cartesi/rollups-node
TEMPLATE=dev/licenses.tpl
MANUAL=dev/licenses-manual.md
OVERRIDES=dev/licenses-overrides.tsv

# The platforms binaries are released for. The notices file describes what is
# distributed, and the .deb and the container image are both Linux. CGO is on
# because the cgo build of a dependency may reach different packages.
TARGETS="linux/amd64 linux/arm64"

cat "$MANUAL"

for target in $TARGETS; do
	GOOS="${target%/*}" GOARCH="${target#*/}" CGO_ENABLED=1 \
		go-licenses report --template "$TEMPLATE" ./...
done |
	# Flatten each entry to a single "name version license url" record.
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
			name = $1; version = $2; license = $3; url = $4
			if (name in override_license) {
				license = override_license[name]
				url = override_url[name]
				gsub(/\{version\}/, version, url)
			}
			printf "\n## %s\n\n", name
			printf "* Name: %s\n* Version: %s\n* License: [%s](%s)\n", name, version, license, url
			if (name in override_note && override_note[name] != "")
				printf "* Note: %s\n", override_note[name]
		}
	'
