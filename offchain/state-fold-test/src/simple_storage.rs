// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use ethers::{contract::Contract, providers::Middleware};
use hex;
use state_fold_types::{contract, ethers};
use std::sync::Arc;

contract::include!("simple_storage");

pub async fn deploy_simple_storage<M: Middleware>(
    client: Arc<M>,
) -> Contract<M> {
    let bytecode =
        hex::decode(include_bytes!("./contracts/bin/SimpleStorage.bin"))
            .unwrap()
            .into();
    let abi = SIMPLESTORAGE_ABI.clone();

    crate::utils::deploy_contract(client, bytecode, abi).await
}
