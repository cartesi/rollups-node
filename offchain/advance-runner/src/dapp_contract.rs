// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use contracts::cartesi_dapp::CartesiDApp;
use ethers::{
    prelude::ContractError,
    providers::{Http, HttpRateLimitRetryPolicy, Provider, RetryClient},
};
use rollups_events::{Address, Hash};
use snafu::{ResultExt, Snafu};
use std::sync::Arc;
use url::Url;

const MAX_RETRIES: u32 = 10;
const INITIAL_BACKOFF: u64 = 1000;

#[derive(Debug, Snafu)]
#[snafu(display("failed to obtain hash from dapp contract"))]
pub struct DappContractError {
    source: ContractError<Provider<RetryClient<Http>>>,
}

pub async fn get_template_hash(
    dapp_address: &Address,
    provider_http_endpoint: Url,
) -> Result<Hash, DappContractError> {
    let provider = Provider::new(RetryClient::new(
        Http::new(provider_http_endpoint),
        Box::new(HttpRateLimitRetryPolicy),
        MAX_RETRIES,
        INITIAL_BACKOFF,
    ));

    let cartesi_dapp =
        CartesiDApp::new(dapp_address.inner(), Arc::new(provider));

    let template_hash = cartesi_dapp
        .get_template_hash()
        .call()
        .await
        .context(DappContractSnafu)?;

    Ok(Hash::new(template_hash))
}
