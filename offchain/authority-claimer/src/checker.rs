// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use contracts::history::{Claim, History};
use ethers::{
    self,
    contract::ContractError,
    providers::{
        Http, HttpRateLimitRetryPolicy, Middleware, Provider, RetryClient,
    },
    types::{ValueOrArray, H160},
};
use rollups_events::{Address, RollupsClaim};
use snafu::{ensure, ResultExt, Snafu};
use std::fmt::Debug;
use std::sync::Arc;
use tracing::trace;
use url::{ParseError, Url};

const GENESIS_BLOCK: u64 = 1;
const MAX_RETRIES: u32 = 10;
const INITIAL_BACKOFF: u64 = 1000;

/// The `DuplicateChecker` checks if a given claim was already submitted to the blockchain.
#[async_trait]
pub trait DuplicateChecker: Debug {
    type Error: snafu::Error + 'static;

    async fn is_duplicated_rollups_claim(
        &mut self,
        dapp_address: Address,
        rollups_claim: &RollupsClaim,
    ) -> Result<bool, Self::Error>;
}

// ------------------------------------------------------------------------------------------------
// DefaultDuplicateChecker
// ------------------------------------------------------------------------------------------------

#[derive(Debug)]
pub struct DefaultDuplicateChecker {
    provider: Arc<Provider<RetryClient<Http>>>,
    history: History<Provider<RetryClient<Http>>>,
    claims: Vec<Claim>,
    confirmations: usize,
    next_block_to_read: u64,
}

#[derive(Debug, Snafu)]
pub enum DuplicateCheckerError {
    #[snafu(display("failed to call contract"))]
    ContractError {
        source: ContractError<ethers::providers::Provider<RetryClient<Http>>>,
    },

    #[snafu(display("failed to call provider"))]
    ProviderError {
        source: ethers::providers::ProviderError,
    },

    #[snafu(display("parser error"))]
    ParseError { source: ParseError },

    #[snafu(display(
        "Depth of `{}` higher than latest block `{}`",
        depth,
        latest
    ))]
    DepthTooHigh { depth: u64, latest: u64 },
}

impl DefaultDuplicateChecker {
    pub fn new(
        http_endpoint: String,
        history_address: Address,
        confirmations: usize,
    ) -> Result<Self, DuplicateCheckerError> {
        let http = Http::new(Url::parse(&http_endpoint).context(ParseSnafu)?);
        let retry_client = RetryClient::new(
            http,
            Box::new(HttpRateLimitRetryPolicy),
            MAX_RETRIES,
            INITIAL_BACKOFF,
        );
        let provider = Arc::new(Provider::new(retry_client));
        let history = History::new(
            H160(history_address.inner().to_owned()),
            provider.clone(),
        );
        Ok(Self {
            provider,
            history,
            claims: Vec::new(),
            confirmations,
            next_block_to_read: GENESIS_BLOCK,
        })
    }
}

#[async_trait]
impl DuplicateChecker for DefaultDuplicateChecker {
    type Error = DuplicateCheckerError;

    async fn is_duplicated_rollups_claim(
        &mut self,
        dapp_address: Address,
        rollups_claim: &RollupsClaim,
    ) -> Result<bool, Self::Error> {
        self.update_claims(dapp_address).await?;
        Ok(self.claims.iter().any(|read_claim| {
            &read_claim.epoch_hash == rollups_claim.epoch_hash.inner()
                && read_claim.first_index == rollups_claim.first_index
                && read_claim.last_index == rollups_claim.last_index
        }))
    }
}

impl DefaultDuplicateChecker {
    async fn update_claims(
        &mut self,
        dapp_address: Address,
    ) -> Result<(), DuplicateCheckerError> {
        let depth = self.confirmations as u64;

        let current_block = self
            .provider
            .get_block_number()
            .await
            .context(ProviderSnafu)?
            .as_u64();

        ensure!(
            depth <= current_block,
            DepthTooHighSnafu {
                depth: depth,
                latest: current_block
            }
        );

        let block_number = current_block - depth;

        let dapp_address = H160(dapp_address.inner().to_owned());
        let topic = ValueOrArray::Value(Some(dapp_address.into()));

        let mut claims: Vec<_> = self
            .history
            .new_claim_to_history_filter()
            .from_block(self.next_block_to_read)
            .to_block(block_number)
            .topic1(topic)
            .query()
            .await
            .context(ContractSnafu)?
            .into_iter()
            .map(|event| event.claim)
            .collect();

        if !claims.is_empty() {
            trace!("read new claims {:?}", claims);
            self.claims.append(&mut claims);
        }

        self.next_block_to_read = block_number + 1;

        Ok(())
    }
}
