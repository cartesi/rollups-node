// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use eth_block_history::BlockSubscriber;
use eth_state_fold::StateFoldEnvironment;
use eth_state_fold_types::ethers::providers::{
    Http, HttpRateLimitRetryPolicy, Middleware, Provider, RetryClient,
};
use rollups_events::DAppMetadata;
use snafu::ResultExt;
use std::sync::{Arc, Mutex};
use types::UserData;
use url::Url;

use crate::error::ProviderSnafu;
use crate::{
    config::EthInputReaderConfig,
    error::{BlockArchiveSnafu, BrokerSnafu, EthInputReaderError, ParseSnafu},
    machine::{BrokerStatus, Context},
    metrics::EthInputReaderMetrics,
};

/// Maximum events allowed to be in a single provider response. If response
/// event number reaches over this number, the request must be split into
/// sub-ranges retried on each of them separately.
///
/// Motivation for this configuration parameter mainly comes from Alchemy
/// as past a certain limit it responds with invalid data.
const MAXIMUM_EVENTS_PER_RESPONSE: usize = 10000;
const MAX_RETRIES: u32 = 10;
const INITIAL_BACKOFF: u64 = 1000;

pub type InputProvider = Provider<RetryClient<Http>>;

pub async fn create_environment(
    config: &EthInputReaderConfig,
    broker: &impl BrokerStatus,
    dapp_metadata: DAppMetadata,
    metrics: EthInputReaderMetrics,
) -> Result<
    (
        BlockSubscriber<Provider<RetryClient<Http>>>,
        StateFoldEnvironment<InputProvider, Mutex<UserData>>,
        Context,
    ),
    EthInputReaderError,
> {
    let http = Http::new(
        Url::parse(&config.bh_config.http_endpoint).context(ParseSnafu)?,
    );

    let retry_client = RetryClient::new(
        http,
        Box::new(HttpRateLimitRetryPolicy),
        MAX_RETRIES,
        INITIAL_BACKOFF,
    );

    let provider = Arc::new(Provider::new(retry_client));

    let chain_id = provider.get_chainid().await.context(ProviderSnafu)?;

    assert!(chain_id.as_u64() == config.chain_id, "The chain id provided in the configuration ({}) doesn't match the chain id of the provider ({})", config.chain_id, chain_id);

    let genesis_timestamp = provider
        .get_block(config.dapp_deployment.deploy_block_hash)
        .await
        .context(ProviderSnafu)?
        .ok_or(EthInputReaderError::MissingChainId)?
        .timestamp
        .as_u64();

    let subscriber = BlockSubscriber::start(
        Arc::clone(&provider),
        config.bh_config.ws_endpoint.to_owned(),
        config.bh_config.block_timeout,
        config.bh_config.max_depth,
    )
    .await
    .context(BlockArchiveSnafu)?;

    let env = StateFoldEnvironment::new(
        provider,
        Some(Arc::clone(&subscriber.block_archive)),
        config.sf_config.safety_margin,
        config.sf_config.genesis_block,
        config.sf_config.query_limit_error_codes.clone(),
        config.sf_config.concurrent_events_fetch,
        MAXIMUM_EVENTS_PER_RESPONSE,
        Mutex::new(UserData::default()),
    );

    let epoch_length = config.epoch_duration;
    let context = Context::new(
        genesis_timestamp,
        epoch_length,
        broker,
        dapp_metadata,
        metrics,
    )
    .await
    .context(BrokerSnafu)?;

    Ok((subscriber, env, context))
}
