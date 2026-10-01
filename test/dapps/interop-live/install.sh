cat >/usr/local/bin/interop-live-dapp <<'EOF'
#!/usr/bin/env bash

# Test application of "daveinterop live --scenario full". The payload decides:
#
#   - an advance whose payload starts with "reject" (hex 72656a656374) emits
#     one report and is rejected;
#   - any other advance emits one voucher (1 gwei to the sender, empty
#     payload), one notice and one report (both with the payload), and is
#     accepted;
#   - an inspect emits one report with its payload.
#
# So a test chooses the accept/reject mix of each epoch. A voucher is an ETH
# transfer: the application calls an account without code only with an empty
# payload (TargetHasNoCode), and pays the value from its own balance, which
# the test funds before it executes the vouchers.

MERKLE_FILE=/tmp/merkle.dat
MERKLE_KEEP=/tmp/merkle.keep
REJECT_PREFIX=72656a656374
# 1 gwei (10^9 wei) as a 32-byte big-endian word.
VOUCHER_VALUE=000000000000000000000000000000000000000000000000000000003b9aca00

emit() {
  printf '%s\n' "$2" | rollup "$1" >/dev/null
}

accept_request() {
  local status

  rm -f "$MERKLE_KEEP"

  if [[ -f "$MERKLE_FILE" ]]; then
    cp -f "$MERKLE_FILE" "$MERKLE_KEEP"
  fi

  rollup accept
  status=$?

  # The stock rollup helper resets /tmp/merkle.dat after it receives the next
  # advance request. The node compares against the cumulative output tree, so a
  # shell dApp that invokes the helper once per operation must restore it (see
  # test/dapps/erc20-withdrawal). A rejected input rolls the whole machine
  # back, this file included, so rejects need nothing.
  if [[ -f "$MERKLE_KEEP" ]]; then
    mv -f "$MERKLE_KEEP" "$MERKLE_FILE"
  fi
  return "$status"
}

request="$(accept_request)"
while true; do
  printf '%s\n' "$request" >/tmp/request.json
  request_type="$(jq -r .request_type /tmp/request.json)"
  payload="$(jq -r .data.payload /tmp/request.json | tr 'A-F' 'a-f')"

  if [[ "$request_type" != "advance_state" ]]; then
    emit report "{\"payload\":\"$payload\"}"
    request="$(accept_request)"
    continue
  fi

  if [[ "${payload#0x}" == "$REJECT_PREFIX"* ]]; then
    emit report "{\"payload\":\"$payload\"}"
    request="$(rollup reject)"
    continue
  fi

  sender="$(jq -r .data.msg_sender /tmp/request.json)"
  emit voucher "{\"destination\":\"$sender\",\"value\":\"0x$VOUCHER_VALUE\",\"payload\":\"0x\"}"
  emit notice "{\"payload\":\"$payload\"}"
  emit report "{\"payload\":\"$payload\"}"
  request="$(accept_request)"
done
EOF

chmod +x /usr/local/bin/interop-live-dapp
