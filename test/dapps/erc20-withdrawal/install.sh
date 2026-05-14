cat >/usr/local/bin/erc20-withdrawal-dapp <<'EOF'
#!/usr/bin/env bash

ACCOUNT_DRIVE=/dev/pmem1
ACCOUNT_SIZE=32
MAX_ACCOUNTS=131072
ZERO_RECORD=0000000000000000000000000000000000000000000000000000000000000000
ERC20_TRANSFER_SELECTOR=a9059cbb
ZERO_VALUE=0000000000000000000000000000000000000000000000000000000000000000
MERKLE_FILE=/tmp/merkle.dat
MERKLE_KEEP=/tmp/merkle.keep

trusted_portal="$(printf '%s' "${TRUSTED_ERC20_PORTAL:-}" | tr 'A-F' 'a-f' | sed 's/^0x//')"
trusted_token="$(printf '%s' "${TRUSTED_ERC20_TOKEN:-}" | tr 'A-F' 'a-f' | sed 's/^0x//')"

report() {
  printf '{"payload":"0x%s"}\n' "$1" | rollup report >/dev/null
}

report_text() {
  printf '%s' "$1" | xxd -p -c 0 | {
    read -r payload
    report "$payload"
  }
}

reject_request() {
  report "$1"
  rollup reject
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
  # shell dApp that invokes the helper once per operation must restore it.
  if [[ -f "$MERKLE_KEEP" ]]; then
    mv -f "$MERKLE_KEEP" "$MERKLE_FILE"
  fi
  return "$status"
}

reverse_bytes() {
  printf '%s' "$1" | fold -w2 | tac | tr -d '\n'
}

uint64_be_to_dec() {
  printf '%d' "0x$1"
}

uint64_dec_to_le() {
  reverse_bytes "$(printf '%016x' "$1")"
}

hex_to_text() {
  printf '%s' "$1" | xxd -r -p
}

read_record() {
  dd if="$ACCOUNT_DRIVE" bs="$ACCOUNT_SIZE" skip="$1" count=1 2>/dev/null | xxd -p -c "$ACCOUNT_SIZE"
}

write_record() {
  printf '%s' "$2" | xxd -r -p | dd of="$ACCOUNT_DRIVE" bs="$ACCOUNT_SIZE" seek="$1" count=1 conv=notrunc 2>/dev/null
}

zero_record() {
  write_record "$1" "$ZERO_RECORD"
}

record_address() {
  printf '%s' "${1:16:40}"
}

record_balance() {
  uint64_be_to_dec "$(reverse_bytes "${1:0:16}")"
}

find_account_index() {
  local address="$1"
  local i record
  for ((i = 0; i < MAX_ACCOUNTS; i++)); do
    record="$(read_record "$i")"
    if [[ "$record" == "$ZERO_RECORD" ]]; then
      printf '%d' "$i"
      return 1
    fi
    if [[ "$(record_address "$record")" == "$address" ]]; then
      printf '%d' "$i"
      return 0
    fi
  done
  return 2
}

last_account_index() {
  local i record last=-1
  for ((i = 0; i < MAX_ACCOUNTS; i++)); do
    record="$(read_record "$i")"
    if [[ "$record" == "$ZERO_RECORD" ]]; then
      printf '%d' "$last"
      return 0
    fi
    last="$i"
  done
  printf '%d' "$last"
}

credit_account() {
  local address="$1"
  local amount="$2"
  local idx status record balance new_balance

  idx="$(find_account_index "$address")"
  status=$?
  if [[ "$status" -eq 2 ]]; then
    return 1
  fi

  if [[ "$status" -eq 0 ]]; then
    record="$(read_record "$idx")"
    balance="$(record_balance "$record")"
    new_balance=$((balance + amount))
  else
    new_balance="$amount"
  fi

  if (( new_balance <= 0 )); then
    return 1
  fi
  write_record "$idx" "$(uint64_dec_to_le "$new_balance")${address}00000000"
}

debit_account() {
  local address="$1"
  local amount="$2"
  local idx status record balance new_balance last last_record

  idx="$(find_account_index "$address")"
  status=$?
  if [[ "$status" -ne 0 ]]; then
    return 1
  fi

  record="$(read_record "$idx")"
  balance="$(record_balance "$record")"
  if (( amount <= 0 || balance < amount )); then
    return 1
  fi

  new_balance=$((balance - amount))
  if (( new_balance > 0 )); then
    write_record "$idx" "$(uint64_dec_to_le "$new_balance")${address}00000000"
    return 0
  fi

  last="$(last_account_index)"
  if (( last < 0 )); then
    return 1
  fi
  if [[ "$idx" != "$last" ]]; then
    last_record="$(read_record "$last")"
    write_record "$idx" "$last_record"
  fi
  zero_record "$last"
}

valid_positive_i64_uint256() {
  local amount_hex="$1"
  [[ "${amount_hex:0:48}" == "000000000000000000000000000000000000000000000000" ]] || return 1
  [[ "${amount_hex:48:1}" =~ [0-7] ]] || return 1
}

handle_erc20_deposit() {
  local msg_sender="$1"
  local payload="$2"
  local token sender amount_hex amount

  [[ -n "$trusted_portal" && -n "$trusted_token" ]] || return 1
  [[ "$msg_sender" == "$trusted_portal" ]] || return 1
  [[ ${#payload} -ge 144 ]] || return 1

  token="${payload:0:40}"
  sender="${payload:40:40}"
  amount_hex="${payload:80:64}"
  [[ "$token" == "$trusted_token" ]] || return 1
  valid_positive_i64_uint256 "$amount_hex" || return 1

  amount="$(uint64_be_to_dec "${amount_hex:48:16}")"
  (( amount > 0 )) || return 1
  credit_account "$sender" "$amount"
}

emit_erc20_transfer_voucher() {
  local recipient="$1"
  local amount="$2"
  local amount_be

  [[ -n "$trusted_token" ]] || return 1
  amount_be="$(printf '%064x' "$amount")"
  printf '{"destination":"0x%s","value":"0x%s","payload":"0x%s000000000000000000000000%s%s"}\n' \
    "$trusted_token" "$ZERO_VALUE" "$ERC20_TRANSFER_SELECTOR" "$recipient" "$amount_be" |
    rollup voucher >/dev/null
}

handle_test_withdraw() {
  local recipient="$1"
  local payload="$2"
  local amount_be amount

  [[ -n "$recipient" ]] || return 1
  [[ ${#payload} -eq 18 ]] || return 1
  [[ "${payload:0:2}" == "01" ]] || return 1
  amount_be="${payload:2:16}"
  [[ "${amount_be:0:1}" =~ [0-7] ]] || return 1
  amount="$(uint64_be_to_dec "$amount_be")"
  (( amount > 0 )) || return 1

  debit_account "$recipient" "$amount" || return 1
  emit_erc20_transfer_voucher "$recipient" "$amount"
}

inspect_balance() {
  local query="$1"
  local normalized address idx status record balance

  normalized="$(printf '%s' "$query" | tr 'A-F' 'a-f')"
  if [[ "$normalized" =~ ^[[:space:]]*balance[[:space:]]+(0x)?([0-9a-f]{40})[[:space:]]*$ ]]; then
    address="${BASH_REMATCH[2]}"
  else
    report_text '{"error":"usage: balance 0x<address>"}'
    return 1
  fi

  idx="$(find_account_index "$address")"
  status=$?
  if [[ "$status" -eq 0 ]]; then
    record="$(read_record "$idx")"
    balance="$(record_balance "$record")"
    report_text "$(printf '{"type":"erc20_balance","address":"0x%s","found":true,"account_index":"0x%x","balance":"%s"}' \
      "$address" "$idx" "$balance")"
    return 0
  fi
  if [[ "$status" -eq 1 ]]; then
    report_text "$(printf '{"type":"erc20_balance","address":"0x%s","found":false,"balance":"0"}' "$address")"
    return 0
  fi

  report_text '{"error":"account drive full"}'
  return 1
}

request="$(accept_request)"
while true; do
  printf '%s\n' "$request" >/tmp/request.json
  request_type="$(jq -r .request_type /tmp/request.json)"

  if [[ "$request_type" == "inspect_state" ]]; then
    payload="$(jq -r .data.payload /tmp/request.json | sed 's/^0x//')"
    inspect_balance "$(hex_to_text "$payload")"
    request="$(accept_request)"
    continue
  fi

  if [[ "$request_type" != "advance_state" ]]; then
    request="$(accept_request)"
    continue
  fi

  msg_sender="$(jq -r .data.msg_sender /tmp/request.json | tr 'A-F' 'a-f' | sed 's/^0x//')"
  payload="$(jq -r .data.payload /tmp/request.json | tr 'A-F' 'a-f' | sed 's/^0x//')"

  if handle_erc20_deposit "$msg_sender" "$payload"; then
    report 6465706f736974206f6b
    request="$(accept_request)"
  elif handle_test_withdraw "$msg_sender" "$payload"; then
    report 7769746864726177206f6b
    request="$(accept_request)"
  else
    request="$(reject_request 62616420696e707574)"
  fi
done
EOF

chmod +x /usr/local/bin/erc20-withdrawal-dapp
