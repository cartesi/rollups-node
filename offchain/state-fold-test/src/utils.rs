// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use ethers::{
    abi::Abi,
    contract::{Contract, ContractFactory},
    core::utils::{Geth, GethInstance},
    providers::{Http, Middleware, Provider},
    types::Bytes,
};
use state_fold_types::ethers;
use state_fold_types::Block;

use std::convert::TryFrom;
use std::convert::TryInto;
use std::sync::Arc;

pub async fn new_geth() -> (GethInstance, Arc<Provider<Http>>) {
    let geth = Geth::new().spawn();
    let provider = Provider::<Http>::try_from(geth.endpoint()).unwrap();
    let deployer = provider.get_accounts().await.unwrap()[0];
    (geth, Arc::new(provider.with_sender(deployer)))
}

pub async fn deploy_contract<M: Middleware>(
    client: Arc<M>,
    bytecode: Bytes,
    abi: Abi,
) -> Contract<M> {
    let factory = ContractFactory::new(abi, bytecode, client);

    // This is what we wanted to write, but there's a bug in ethers preventing
    // it.
    /*
    factory
        .deploy("initial value".to_string())
        .unwrap()
        .send()
        .await
        .unwrap()
    */

    let mut deployer = factory.deploy("initial value".to_string()).unwrap();
    deployer.tx.set_gas(8000000);
    deployer.send().await.unwrap()
}

pub async fn get_current_block<M: Middleware>(provider: &M) -> Block {
    provider
        .get_block(provider.get_block_number().await.unwrap())
        .await
        .unwrap()
        .unwrap()
        .try_into()
        .unwrap()
}
