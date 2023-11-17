#!/usr/bin/env bash
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

################################################################################
# Download rollups contracts
_download() {
    local download_dir=$1
    shift
    local artifact_name=$1
    shift
    local artifact_version=$1

    local tgz_file="$artifact_name-$artifact_version.tgz"
    local url="https://registry.npmjs.org/@cartesi/$artifact_name/-/$tgz_file"
    wget \
        --quiet \
        $url \
        --directory-prefix $download_dir
    check_error $? "failed to download $url"

    local tar_dir="$download_dir/$artifact_name-$artifact_version"
    mkdir -p $tar_dir
    tar zxf \
        $download_dir/$tgz_file \
        --directory $tar_dir \
        > /dev/null
    echo $tar_dir
}

rollups_download() {
    local download_dir=$1
    shift
    local contracts_version=$1

    _download $download_dir "rollups" $contracts_version
}

solidity_util_download() {
    local download_dir=$1
    shift
    local rollups_contracts_dir=$1

    version=$(\
        cat ${rollups_contracts_dir}/package/package.json \
        | jq -r '.dependencies."@cartesi/util"' \
    )

    _download $download_dir "util" $version
}

################################################################################
# Prepare forge environment
forge_prepare() {
    local tmp_dir=$1
    shift
    local remappings_file="$1"

    cd "$tmp_dir"
    mkdir forge_prj
    cd forge_prj

    # download contracts
    local download_dir="$tmp_dir/downloads"
    mkdir -p "$download_dir"
    local rollups_tar_dir=$(
        rollups_download \
            "$download_dir" \
            "$ROLLUPS_CONTRACTS_VERSION"
    )
    check_error $? "failed to download contracts"
    log "downloaded rollups-contracts to $rollups_tar_dir"

    local solidity_util_tar_dir=$(
        solidity_util_download \
            "$download_dir" \
            "$rollups_tar_dir"
        )
    check_error $? "failed to download solidity-util"
    log "downloaded solidity-util to $solidity_util_tar_dir"

    forge init \
        --shallow \
        --no-git \
        --quiet \
        . &> /dev/null
    check_error $? "failed to init forge project"

    # install openzeppelin
    openzeppelin_version=v$(\
        cat ${rollups_tar_dir}/package/package.json \
        | jq -r '.dependencies."@openzeppelin/contracts"' \
    )
    forge install \
        --shallow \
        --no-git \
        --quiet \
        openzeppelin/openzeppelin-contracts@${openzeppelin_version} \
        &> /dev/null
    check_error $? "failed to install openzeppelin $openzeppelin_version"

    # copy contracts
    local rollups_contracts_dir="$rollups_tar_dir/package/contracts"
    cp -pr $rollups_contracts_dir/* src

    local solidity_util_contracts_dir="$solidity_util_tar_dir/package/contracts"
    local util_lib_dir="lib/@cartesi/util/contracts"
    mkdir -p $util_lib_dir
    cp -pr $solidity_util_contracts_dir/* $util_lib_dir

    # apply forge remappings
    cp "$remappings_file" "remappings.txt"
}

################################################################################
# Deploy a contract
# Assumes all non-standard arguments are passed
# output: returned address
# input: contract and dependencies
contract_deploy() {
    local -n address="$1"
    shift
    local contract="$1"
    shift
    local contract_name="$1"
    shift

    address=$(
        forge create \
            --json \
            --rpc-url $DEVNET_RPC_URL \
            --private-key $DEVNET_FOUNDRY_ACCOUNT_0_PRIVATE_KEY \
            "$contract:$contract_name" \
            $@ \
        | jq -r ".deployedTo"
    )
    check_error $? "failed to deploy $contract_name"
}

################################################################################
# Call arbitrary code on a contract
# Splits returned values into an array
contract_create() {
    local -n addrs="$1"
    shift
    local -n block="$1"
    shift

    # Generate values without issuing a transaction
    local values=$(
        cast call \
            --rpc-url $DEVNET_RPC_URL \
            $@
    )
    check_error $? "failed to retrieve returned values"
    # Split returned values
    IFS=$'\n' addrs=($values)

    # Send tansaction
    block=$(cast send \
        --json \
        --rpc-url $DEVNET_RPC_URL \
        --private-key $DEVNET_FOUNDRY_ACCOUNT_0_PRIVATE_KEY \
        $@ \
        | jq -r '.blockNumber'
    )
    check_error $? "failed to send transaction"
}
