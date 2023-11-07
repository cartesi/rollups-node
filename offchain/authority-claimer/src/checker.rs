// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use contracts::authority::Authority;
use ethers::{
    self,
    abi::AbiEncode,
    providers::{Http, HttpRateLimitRetryPolicy, Provider, RetryClient},
    types::{H160, U256},
};
use rollups_events::{Address, RollupsClaim};
use snafu::{ResultExt, Snafu};
use std::fmt::Debug;
use std::sync::Arc;
use tracing::info;
use url::{ParseError, Url};

const MAX_RETRIES: u32 = 10;
const INITIAL_BACKOFF: u64 = 1000;

/// The `DuplicateChecker` checks if a given claim was already submitted to the blockchain.
#[async_trait]
pub trait DuplicateChecker: Debug {
    type Error: snafu::Error + 'static;

    async fn is_duplicated_rollups_claim(
        &self,
        dapp_address: Address,
        rollups_claim: &RollupsClaim,
    ) -> Result<bool, Self::Error>;
}

// ------------------------------------------------------------------------------------------------
// DefaultDuplicateChecker
// ------------------------------------------------------------------------------------------------

#[derive(Debug)]
pub struct DefaultDuplicateChecker {
    authority: Authority<Provider<RetryClient<Http>>>,
}

#[derive(Debug, Snafu)]
pub enum DuplicateCheckerError {
    #[snafu(display("invalid provider URL"))]
    ContractError {
        source: ethers::contract::ContractError<
            ethers::providers::Provider<RetryClient<Http>>,
        >,
    },

    #[snafu(display("parser error"))]
    ParseError { source: ParseError },
}

impl DefaultDuplicateChecker {
    pub fn new(
        http_endpoint: String,
        authority_address: Address,
    ) -> Result<Self, DuplicateCheckerError> {
        let http = Http::new(Url::parse(&http_endpoint).context(ParseSnafu)?);

        let retry_client = RetryClient::new(
            http,
            Box::new(HttpRateLimitRetryPolicy),
            MAX_RETRIES,
            INITIAL_BACKOFF,
        );

        let provider = Arc::new(Provider::new(retry_client));

        let authority = Authority::new(
            H160(authority_address.inner().to_owned()),
            provider,
        );

        Ok(Self { authority })
    }
}

#[async_trait]
impl DuplicateChecker for DefaultDuplicateChecker {
    type Error = DuplicateCheckerError;

    async fn is_duplicated_rollups_claim(
        &self,
        dapp_address: Address,
        rollups_claim: &RollupsClaim,
    ) -> Result<bool, Self::Error> {
        let proof_context =
            U256([rollups_claim.epoch_index, 0, 0, 0]).encode().into();

        match self
            .authority
            .get_claim(H160(dapp_address.inner().to_owned()), proof_context)
            .block(ethers::types::BlockNumber::Latest)
            .call()
            .await
        {
            // If there's any response, the claim already exists
            Ok(_response) => Ok(true),
            Err(e) => {
                // If there's an InvalidClaimIndex error, we're asking for an index
                // bigger than the current one, which means it's a new claim
                if String::from(e.to_string()).contains("InvalidClaimIndex()") {
                    Ok(false)
                } else {
                    info!("{:?}", e);
                    Err(e).context(ContractSnafu)
                }
            }
        }
    }
}
