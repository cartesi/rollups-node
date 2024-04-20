export CARTESI_LOG_LEVEL="info"
export CARTESI_LOG_PRETTY="true"
export CARTESI_FEATURE_HOST_MODE="false"
export CARTESI_FEATURE_DISABLE_CLAIMER="false"
export CARTESI_FEATURE_DISABLE_MACHINE_HASH_CHECK="false"
export CARTESI_EPOCH_DURATION="120"
export CARTESI_BLOCKCHAIN_ID="31337"
export CARTESI_BLOCKCHAIN_HTTP_ENDPOINT="http://localhost:8545"
export CARTESI_BLOCKCHAIN_WS_ENDPOINT="ws://localhost:8545"
export CARTESI_BLOCKCHAIN_IS_LEGACY="false"
export CARTESI_BLOCKCHAIN_FINALITY_OFFSET="1"
export CARTESI_BLOCKCHAIN_BLOCK_TIMEOUT="60"
export CARTESI_CONTRACTS_APPLICATION_ADDRESS="0x522A944BD778831d08499b7f3757ba2f03A7F0e2"
export CARTESI_CONTRACTS_APPLICATION_DEPLOYMENT_BLOCK_NUMBER="19"
export CARTESI_CONTRACTS_HISTORY_ADDRESS="0x325272217ae6815b494bF38cED004c5Eb8a7CdA7"
export CARTESI_CONTRACTS_AUTHORITY_ADDRESS="0x58c93F83fb3304730C95aad2E360cdb88b782010"
export CARTESI_CONTRACTS_INPUT_BOX_ADDRESS="0x59b22D57D4f067708AB0c00552767405926dc768"
export CARTESI_CONTRACTS_INPUT_BOX_DEPLOYMENT_BLOCK_NUMBER="20"
export CARTESI_SNAPSHOT_DIR="$PWD/machine-snapshot"
export CARTESI_AUTH_KIND="mnemonic"
export CARTESI_AUTH_MNEMONIC="test test test test test test test test test test test junk"
export CARTESI_POSTGRES_ENDPOINT="postgres://postgres:password@localhost:5432/postgres"
export CARTESI_HTTP_ADDRESS="0.0.0.0"
export CARTESI_HTTP_PORT="10000"

rust_bin_path="$PWD/offchain/target/debug"
# Check if the path is already in $PATH
if [[ ":$PATH:" != *":$rust_bin_path:"* ]]; then
    export PATH=$PATH:$rust_bin_path
fi
