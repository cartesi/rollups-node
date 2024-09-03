#!/usr/bin/env bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)
export CARTESI_LOG_LEVEL="info"
export CARTESI_LOG_PRETTY_ENABLED="true"
export CARTESI_EPOCH_LENGTH="10"
export CARTESI_BLOCKCHAIN_ID="31337"
export CARTESI_BLOCKCHAIN_HTTP_ENDPOINT="http://localhost:8545"
export CARTESI_BLOCKCHAIN_WS_ENDPOINT="ws://localhost:8545"
export CARTESI_BLOCKCHAIN_FINALITY_OFFSET="1"
export CARTESI_BLOCKCHAIN_BLOCK_TIMEOUT="60"
export CARTESI_CONTRACTS_APPLICATION_ADDRESS="0x1b0FAD42f016a9EBa358c7491A67fa1fAE82912A"
export CARTESI_CONTRACTS_ICONSENSUS_ADDRESS="0x3fd5dc9dCf5Df3c7002C0628Eb9AD3bb5e2ce257"
export CARTESI_CONTRACTS_INPUT_BOX_ADDRESS="0x593E5BCf894D6829Dd26D0810DA7F064406aebB6"
export CARTESI_CONTRACTS_INPUT_BOX_DEPLOYMENT_BLOCK_NUMBER="10"
export CARTESI_SNAPSHOT_DIR="$PWD/machine-snapshot"
export CARTESI_AUTH_KIND="mnemonic"
export CARTESI_AUTH_MNEMONIC="test test test test test test test test test test test junk"
export CARTESI_POSTGRES_ENDPOINT="postgres://postgres:password@localhost:5432/postgres"
export CARTESI_HTTP_ADDRESS="0.0.0.0"
export CARTESI_HTTP_PORT="10000"
export CARTESI_POSTGRES_SSL_ENABLED="false"

rust_bin_path="$PWD/cmd/authority-claimer/target/debug"
# Check if the path is already in $PATH
if [[ ":$PATH:" != *":$rust_bin_path:"* ]]; then
    export PATH=$PATH:$rust_bin_path
fi
