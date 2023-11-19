#!/bin/sh
jq -r '.contracts | to_entries | .[] | "\(.value.address) \(.key)"' < /usr/share/cartesi/localhost.json
exec "$@"
