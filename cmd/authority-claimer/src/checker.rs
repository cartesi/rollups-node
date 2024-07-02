// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::{
    contracts::iconsensus::IConsensus,
    rollups_events::{Address, Hash, RollupsClaim},
};
use async_trait::async_trait;
use ethers::{
    self,
    contract::ContractError,
    providers::{
        Http, HttpRateLimitRetryPolicy, Middleware, Provider, RetryClient,
    },
    types::{Address as EthersAddress, H160},
};
use snafu::{ensure, ResultExt, Snafu};
use std::{collections::HashSet, fmt::Debug, sync::Arc};
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
        iconsensus: &Address,
    ) -> Result<bool, Self::Error>;
}

// ------------------------------------------------------------------------------------------------
// DefaultDuplicateChecker
// ------------------------------------------------------------------------------------------------

#[derive(Debug, Clone, Hash, Eq, PartialEq)]
struct Claim {
    application: Address,
    last_block: u64,
    claim_hash: Hash,
}

#[derive(Debug)]
pub struct DefaultDuplicateChecker {
    provider: Arc<Provider<RetryClient<Http>>>,
    from: EthersAddress,
    claims: HashSet<Claim>,
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
    pub async fn new(
        http_endpoint: String,
        from: EthersAddress,
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
        let checker = Self {
            provider,
            from,
            claims: HashSet::new(),
            confirmations,
            next_block_to_read: genesis_block,
        };
        Ok(checker)
    }
}

#[async_trait]
impl DuplicateChecker for DefaultDuplicateChecker {
    type Error = DuplicateCheckerError;

    async fn is_duplicated_rollups_claim(
        &mut self,
        rollups_claim: &RollupsClaim,
        iconsensus: &Address,
    ) -> Result<bool, Self::Error> {
        self.update_claims(iconsensus).await?;
        let claim = Claim {
            application: rollups_claim.dapp_address.clone(),
            last_block: rollups_claim.last_block,
            claim_hash: rollups_claim.output_merkle_root_hash.clone(),
        };
        Ok(self.claims.contains(&claim))
    }
}

impl DefaultDuplicateChecker {
    async fn update_claims(
        &mut self,
        iconsensus: &Address,
    ) -> Result<(), DuplicateCheckerError> {
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

        let iconsensus = IConsensus::new(
            H160(iconsensus.inner().to_owned()),
            self.provider.clone(),
        );

        let claims = iconsensus
            .claim_submission_filter()
            .from_block(self.next_block_to_read)
            .to_block(latest)
            .topic1(self.from)
            .query()
            .await
            .context(ContractSnafu)?;

        trace!(
            "read new claims {:?} from block {} to {}",
            claims,
            self.next_block_to_read,
            latest
        );

        for claim_submission in claims.into_iter() {
            let claim = Claim {
                application: Address::new(claim_submission.app_contract.into()),
                last_block: claim_submission
                    .last_processed_block_number
                    .as_u64(),
                claim_hash: Hash::new(claim_submission.claim),
            };
            self.claims.insert(claim);
        }
        self.next_block_to_read = latest + 1;
        Ok(())
    }
}
