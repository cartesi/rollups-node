#!/usr/bin/env bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)
set -o nounset
set -o pipefail
if [[ "${TRACE-0}" == "1" ]]; then set -o xtrace; fi

script_dir="$( cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly script_dir

. "$script_dir/lib/util.sh"
. "$script_dir/lib/anvil.sh"
. "$script_dir/lib/contracts.sh"

################################################################################
# Configuration
ROLLUPS_CONTRACTS_VERSION="1.1.0"
DEVNET_RPC_URL="http://localhost:8545"
DEVNET_FOUNDRY_ACCOUNT_0_ADDRESS="0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
DEVNET_FOUNDRY_ACCOUNT_0_PRIVATE_KEY="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
# Salt is the same as hardhat's create2Salt
# See https://github.com/wighawag/hardhat-deploy/blob/a611466906282969cee601e4f6cd53438fefa2b3/src/helpers.ts#L559
DEVNET_DEFAULT_SALT="0x0000000000000000000000000000000000000000000000000000000000000000"
readonly ROLLUPS_CONTRACTS_VERSION \
    DEVNET_RPC_URL \
    DEVNET_FOUNDRY_ACCOUNT_0_ADDRESS \
    DEVNET_FOUNDRY_ACCOUNT_0_PRIVATE_KEY \
    DEVNET_DEFAULT_SALT

# Defaults
devnet_anvil_state_file=$(realpath "./anvil_state.json")
devnet_deployment_file=$(realpath "./deployment.json")
template_hash_file=""
VERBOSE=""
forge_remappings_file="$script_dir/remappings.txt"
readonly forge_remappings_file

# Deployment info, which will be gathered during processing
declare -A deployment_info

################################################################################
# Utility functions
################################################################################
usage()
{
   echo "Generate devnet for testing the Cartesi Rollups Node"
   echo
   echo "Usage: $0 [options]"
   echo
   echo "OPTIONS:"
   echo
   echo "    -t template-hash-file"
   echo "        Mandatory Cartesi Machine template hash file"
   echo "    -a"
   echo "        Path for output anvil state file"
   echo "    -d"
   echo "        Path for deployment information file"
   echo "    -v"
   echo "        Verbose mode"
   echo "    -h"
   echo "        Show this message"
   echo

   exit 0
}

################################################################################
# Finish processing and clean up environment
finish() {
    verbose "finishing up"

    if [ -n "$work_dir" ]; then
        rm -rf "$work_dir"
        check_error $? "failed to remove workdir $work_dir"
    fi

    if [[ -n "$anvil_pid" ]]; then
        anvil_down "$anvil_pid"
    fi
}

################################################################################
# Print deployment report
print_report() {
    log "Deployment report:"
    log "rollups-contracts version: $ROLLUPS_CONTRACTS_VERSION"
    log "anvil state file location: $devnet_anvil_state_file"
    log "deployment file location : $devnet_deployment_file"
}

################################################################################
# Generate deployment file
generate_deployment_file() {
    local deployment_file="$1"

    echo "{" > "$deployment_file"
    for key in "${!deployment_info[@]}"; do
        echo "    \"${key}\": \"${deployment_info[${key}]}\"," >> "$deployment_file"
    done
    echo "}" >> "$deployment_file"
    verbose "Deployment information saved to $deployment_file"
}

################################################################################
# Deploy rollups libraries
deploy_libraries() {
    local contract="lib/@cartesi/util/contracts/CartesiMathV2.sol"
    local name="CartesiMathV2"
    local lib_address=""
    contract_deploy \
        lib_address \
        "$contract" \
        "$name"
    mathv2_lib="$contract:$name:$lib_address"
    verbose "deployed $mathv2_lib"

    contract="lib/@cartesi/util/contracts/MerkleV2.sol"
    name="MerkleV2"
    contract_deploy \
        lib_address \
        "$contract" \
        "$name" \
        --libraries \
            "$mathv2_lib"
    merklev2_lib="$contract:$name:$lib_address"
    verbose "deployed $merklev2_lib"

    contract="lib/@cartesi/util/contracts/Bitmask.sol"
    name="Bitmask"
    contract_deploy \
        lib_address \
        "$contract" \
        "$name"
    bitmask_lib="$contract:$name:$lib_address"
    verbose "deployed $bitmask_lib"
}

################################################################################
# Deploy portals
deploy_portals() {
    local name="InputBox"
    local contract="src/inputs/InputBox.sol"
    local portal_address=""
    contract_deploy \
        portal_address \
        "$contract" \
        "$name"
    deployment_info["CARTESI_CONTRACTS_INPUT_BOX_ADDRESS"]="$portal_address"
    verbose "deployed $contract:$name:$portal_address"

    declare -A portals
    portals["ERC1155BatchPortal"]="src/portals/ERC1155BatchPortal.sol"
    portals["ERC1155SinglePortal"]="src/portals/ERC1155SinglePortal.sol"
    portals["ERC20Portal"]="src/portals/ERC20Portal.sol"
    portals["ERC721Portal"]="src/portals/ERC721Portal.sol"
    portals["EtherPortal"]="src/portals/EtherPortal.sol"

    local inputbox_address="$portal_address"
    for key in "${!portals[@]}"; do
        name="${key}"
        contract="${portals[${key}]}"
        contract_deploy \
            portal_address \
            "$contract" \
            "$name" \
            --constructor-args \
                "$inputbox_address"
        verbose "deployed $contract:$name:$portal_address"
    done
}

################################################################################
# Deploy DAppRelay
deploy_relay() {
    local contract="src/relays/DAppAddressRelay.sol"
    local name="DAppAddressRelay"
    local relay_address=""
    contract_deploy \
        relay_address \
        "$contract" \
        "$name" \
        --constructor-args \
            "$DEVNET_FOUNDRY_ACCOUNT_0_ADDRESS"
    verbose "deployed $contract:$name:$relay_address"
}

################################################################################
# Create DApp factories
create_factories() {
    local -n auth_hist_factory_addr="$1"
    shift
    local -n dapp_factory_addr="$1"
    shift

    local contract="src/dapp/CartesiDAppFactory.sol"
    local name="CartesiDAppFactory"
    local factory_address=""
    contract_deploy \
        factory_address \
        "$contract" \
        "$name" \
            --libraries "$mathv2_lib" \
            --libraries "$merklev2_lib" \
            --libraries "$bitmask_lib"
    dapp_factory_addr="$factory_address"
    verbose "deployed $contract:$name:$factory_address"

    contract="src/consensus/authority/AuthorityFactory.sol"
    name="AuthorityFactory"
    contract_deploy \
        factory_address \
        "$contract" \
        "$name" \
        --constructor-args \
            "$DEVNET_FOUNDRY_ACCOUNT_0_ADDRESS"
    verbose "deployed $contract:$name:$factory_address"
    authority_factory_address="$factory_address"

    contract="src/history/HistoryFactory.sol"
    name="HistoryFactory"
    contract_deploy \
        factory_address \
        "$contract" \
        "$name" \
        --constructor-args \
            "$DEVNET_FOUNDRY_ACCOUNT_0_ADDRESS"
    verbose "deployed $contract:$name:$factory_address"
    history_factory_address="$factory_address"

    contract="src/consensus/authority/AuthorityHistoryPairFactory.sol"
    name="AuthorityHistoryPairFactory"
    contract_deploy \
        factory_address \
        "$contract" \
        "$name" \
        --constructor-args \
            "$authority_factory_address" \
            "$history_factory_address"
    auth_hist_factory_addr="$factory_address"
    verbose "deployed $contract:$name:$factory_address"
}

################################################################################
# create DApp contracts
create_dapp() {
    local -n ret="$1"
    shift
    local auth_hist_factory_addr="$1"
    shift
    local dapp_factory_addr="$1"
    shift

    local addresses
    contract_create \
        addresses \
        block_number \
        "$auth_hist_factory_addr" \
        "newAuthorityHistoryPair(address,bytes32)(address,address)" \
            "$DEVNET_FOUNDRY_ACCOUNT_0_ADDRESS" \
            "$DEVNET_DEFAULT_SALT"
    authority_address="${addresses[0]}"
    history_address="${addresses[1]}"
    deployment_info["CARTESI_CONTRACTS_AUTHORITY_ADDRESS"]="$authority_address"
    deployment_info["CARTESI_CONTRACTS_HISTORY_ADDRESS"]="$history_address"
    verbose "deployed authority_address=$authority_address"
    verbose "deployed history_address=$history_address"

    contract_create \
        addresses \
        block_number \
        "$dapp_factory_addr" \
        "newApplication(address,address,bytes32,bytes32)(address)" \
            "$authority_address" \
            "$DEVNET_FOUNDRY_ACCOUNT_0_ADDRESS" \
            "$template_hash" \
            "$DEVNET_DEFAULT_SALT"
    ret="${addresses[0]}"
    deployment_info["CARTESI_CONTRACTS_DAPP_ADDRESS"]="$ret"
    deployment_info["CARTESI_CONTRACTS_DAPP_DEPLOYMENT_BLOCK_NUMBER"]="$block_number"
    verbose "deployed dapp_address=$ret"
}

################################################################################
# Main workflow
################################################################################

# Process script options
while getopts ":a:d:t:hv" option; do
    case $option in
        a)
            devnet_anvil_state_file=$(realpath "$OPTARG")
            ;;
        d)
            devnet_deployment_file=$(realpath "$OPTARG")
            ;;
        t)
            template_hash_file="$OPTARG"
            ;;
        h)
            usage
            ;;
        v)
            VERBOSE=1
            ;;
        \?)
            err "$OPTARG is not a valid option"
            usage
            ;;
    esac
done

if [[ -z "$template_hash_file" ]]; then
    err "missing template-hash-file"
    usage
fi

template_hash=$(xxd -p "$template_hash_file")
check_error $? "failed to read template hash"
template_hash=$(echo "$template_hash" | tr -d "\n")
readonly devnet_anvil_state_file devnet_deployment_file template_hash

# From here on, any exit deserves a clean up
trap finish EXIT ERR

log "starting devnet creation"
work_dir=$(mktemp -d)
readonly work_dir
check_error $? "failed to create temp dir"
verbose "created work dir at $work_dir"

anvil_pid=""
anvil_up \
    anvil_pid \
    "$devnet_anvil_state_file"
check_error $? "failed to start anvil"
log "started anvil (pid=$anvil_pid)"

forge_prepare \
    "$work_dir" \
    "$forge_remappings_file"
log "prepared forge environment"

deploy_libraries

deploy_portals
deploy_relay
log "deployed contracts"

create_factories \
    auth_hist_factory_address \
    dapp_factory_address
log "created factories"

create_dapp \
    dapp_address \
    "$auth_hist_factory_address" \
    "$dapp_factory_address"
log "created CartesiDApp"

generate_deployment_file \
    "$devnet_deployment_file"

print_report
log "done creating devnet"
