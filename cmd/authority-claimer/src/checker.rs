// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::contracts::history::{Claim, History};
use crate::rollups_events::{Address, RollupsClaim};
use async_trait::async_trait;
use ethers::{
    self,
    contract::ContractError,
    providers::{
        Http, HttpRateLimitRetryPolicy, Middleware, Provider, RetryClient,
    },
    types::H160,
};
use snafu::{ensure, ResultExt, Snafu};
use std::sync::Arc;
use std::{collections::HashMap, fmt::Debug};
use tracing::trace;
use url::{ParseError, Url};

const MAX_RETRIES: u32 = 10;
const INITIAL_BACKOFF: u64 = 1000;

/// The `DuplicateChecker` checks if a given claim was already submitted to the blockchain.
#[async_trait]
pub trait DuplicateChecker: Debug {
    type Error: snafu::Error + 'static;

    async fn is_duplicated_rollups_claim(
        &mut self,
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
    claims: HashMap<Address, Vec<Claim>>,
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

    #[snafu(display(
        "Claim mismatch; blockchain expects [{}, ?], but got claim with [{}, {}]",
        expected_first_index,
        claim_first_index,
        claim_last_index
    ))]
    ClaimMismatch {
        expected_first_index: u128,
        claim_first_index: u128,
        claim_last_index: u128,
    },
}

impl DefaultDuplicateChecker {
    pub async fn new(
        http_endpoint: String,
        history_address: Address,
        confirmations: usize,
        genesis_block: u64,
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
        let mut checker = Self {
            provider,
            history,
            claims: HashMap::new(),
            confirmations,
            next_block_to_read: genesis_block,
        };
        checker.update_claims().await?; // to allow failure during instantiation
        Ok(checker)
    }
}

#[async_trait]
impl DuplicateChecker for DefaultDuplicateChecker {
    type Error = DuplicateCheckerError;

    async fn is_duplicated_rollups_claim(
        &mut self,
        rollups_claim: &RollupsClaim,
    ) -> Result<bool, Self::Error> {
        self.update_claims().await?;
        let expected_first_index = self
            .claims // HashMap => DappAddress to Vec<Claim>
            .get(&rollups_claim.dapp_address) // Gets a Option<Vec<Claim>>
            .and_then(|claims| claims.last()) // Back to only one Option
            .map(|claim| claim.last_index + 1) // Maps to a number
            .unwrap_or(0); // If None, unwrap to 0
        if rollups_claim.first_index == expected_first_index {
            // This claim is the one the blockchain expects, so it is not considered duplicate.
            Ok(false)
        } else if rollups_claim.last_index < expected_first_index {
            // This claim is already on the blockchain.
            Ok(true)
        } else {
            // This claim is not on blockchain, but it isn't the one blockchain expects.
            // If this happens, there is a bug on the dispatcher.
            Err(DuplicateCheckerError::ClaimMismatch {
                expected_first_index,
                claim_first_index: rollups_claim.first_index,
                claim_last_index: rollups_claim.last_index,
            })
        }
    }
}

impl DefaultDuplicateChecker {
    async fn update_claims(&mut self) -> Result<(), DuplicateCheckerError> {
        let depth = self.confirmations as u64;

        let latest = self
            .provider
            .get_block_number()
            .await
            .context(ProviderSnafu)?
            .as_u64();

        ensure!(depth <= latest, DepthTooHighSnafu { depth, latest });
        let latest = latest - depth;

        if latest < self.next_block_to_read {
            trace!(
                "nothing to read; next block is {}, but current block is {}",
                self.next_block_to_read,
                latest
            );
            return Ok(());
        }

        let new_claims: Vec<(Address, Claim)> = self
            .history
            .new_claim_to_history_filter()
            .from_block(self.next_block_to_read)
            .to_block(latest)
            .query()
            .await
            .context(ContractSnafu)?
            .into_iter()
            .map(|e| (Address::new(e.dapp.into()), e.claim))
            .collect();
        trace!(
            "read new claims {:?} from block {} to {}",
            new_claims,
            self.next_block_to_read,
            latest
        );
        self.append_claims(new_claims);

        self.next_block_to_read = latest + 1;

        Ok(())
    }

    // Appends new claims to the [Address => Vec<Claim>] hashmap cache.
    fn append_claims(&mut self, new_claims: Vec<(Address, Claim)>) {
        if new_claims.is_empty() {
            return;
        }
        for (dapp_address, new_claim) in new_claims {
            match self.claims.get_mut(&dapp_address) {
                Some(old_claims) => {
                    old_claims.push(new_claim);
                }
                None => {
                    self.claims.insert(dapp_address, vec![new_claim]);
                }
            }
        }
    }
}
