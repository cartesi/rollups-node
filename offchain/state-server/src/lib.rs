// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use snafu::ResultExt;
use state_fold::state_fold::{Foldable, StateFoldEnvironment};
use state_fold::state_server_lib::{
    config,
    grpc_server::StateServer,
    utils::{start_server, wait_for_signal},
};
use state_fold_types::ethers::providers::{
    Http, HttpRateLimitRetryPolicy, Provider, RetryClient,
};
use std::sync::{Arc, Mutex};
use tokio::sync::oneshot;
use types::UserData;
use url::Url;

use crate::error::{
    BlockArchiveSnafu, ParserSnafu, StateServerError, TonicSnafu,
};

mod error;

const MAX_RETRIES: u32 = 10;
const INITIAL_BACKOFF: u64 = 1000;

#[tracing::instrument(level = "trace")]
pub async fn run_server<F: Foldable<UserData = Mutex<UserData>> + 'static>(
    config: config::StateServerConfig,
) -> Result<(), StateServerError>
where
    <F as Foldable>::InitialState: serde::de::DeserializeOwned,
    F: serde::ser::Serialize,
{
    tracing::trace!("Starting rollups state-server with config `{:?}`", config);

    let provider = create_provider(&config)?;
    let block_subscriber =
        create_block_subscriber(&config, Arc::clone(&provider)).await?;
    let env = create_env(
        &config,
        Arc::clone(&provider),
        Arc::clone(&block_subscriber.block_archive),
    )?;

    let server = StateServer::<_, _, F>::new(block_subscriber, env);

    let server_address = config.server_address;
    let (shutdown_tx, shutdown_rx) = oneshot::channel();

    tokio::spawn(async { wait_for_signal(shutdown_tx).await });

    start_server(server_address, server, shutdown_rx)
        .await
        .context(TonicSnafu)
}

type ServerProvider = Provider<RetryClient<Http>>;

fn create_provider(
    config: &config::StateServerConfig,
) -> Result<Arc<ServerProvider>, StateServerError> {
    let http = Http::new(
        Url::parse(&config.block_history.http_endpoint).context(ParserSnafu)?,
    );

    let retry_client = RetryClient::new(
        http,
        Box::new(HttpRateLimitRetryPolicy),
        MAX_RETRIES,
        INITIAL_BACKOFF,
    );

    let provider = Provider::new(retry_client);

    Ok(Arc::new(provider))
}

fn create_env(
    config: &config::StateServerConfig,
    provider: Arc<ServerProvider>,
    block_archive: Arc<state_fold::block_history::BlockArchive<ServerProvider>>,
) -> Result<
    Arc<StateFoldEnvironment<ServerProvider, Mutex<UserData>>>,
    StateServerError,
> {
    let env = StateFoldEnvironment::new(
        provider,
        Some(block_archive),
        config.state_fold.safety_margin,
        config.state_fold.genesis_block,
        config.state_fold.query_limit_error_codes.clone(),
        config.state_fold.concurrent_events_fetch,
        10000,
        Mutex::new(UserData::default()),
    );

    Ok(Arc::new(env))
}

async fn create_block_subscriber(
    config: &config::StateServerConfig,
    provider: Arc<ServerProvider>,
) -> Result<
    Arc<state_fold::block_history::BlockSubscriber<ServerProvider>>,
    StateServerError,
> {
    let block_subscriber = state_fold::block_history::BlockSubscriber::start(
        Arc::clone(&provider),
        config.block_history.ws_endpoint.to_owned(),
        config.block_history.block_timeout,
        config.block_history.max_depth,
    )
    .await
    .context(BlockArchiveSnafu)?;

    Ok(Arc::new(block_subscriber))
}
