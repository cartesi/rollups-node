// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

pub mod checker;
pub mod claimer;
pub mod config;
pub mod listener;
pub mod metrics;
pub mod sender;
pub mod signer;

use config::Config;
use listener::{BrokerListener, MultidappBrokerListener};
use snafu::Error;
use tracing::{info, trace};

use crate::{
    checker::DefaultDuplicateChecker,
    claimer::{Claimer, DefaultClaimer},
    listener::DefaultBrokerListener,
    metrics::AuthorityClaimerMetrics,
    sender::DefaultTransactionSender,
};

pub async fn run(config: Config) -> Result<(), Box<dyn Error>> {
    let metrics = AuthorityClaimerMetrics::new();
    let dapp_address = config.authority_claimer_config.dapp_address.clone();
    if let Some(dapp_address) = dapp_address {
        info!("Creating the default broker listener");
        let broker_listener = DefaultBrokerListener::new(
            config.authority_claimer_config.broker_config.clone(),
            config.authority_claimer_config.tx_manager_config.chain_id,
            dapp_address,
        )
        .await?;
        _run(metrics, config, broker_listener).await
    } else {
        info!("Creating the multidapp broker listener");
        let broker_listener = MultidappBrokerListener::new(
            config.authority_claimer_config.broker_config.clone(),
            config.authority_claimer_config.tx_manager_config.chain_id,
        )
        .await?;
        _run(metrics, config, broker_listener).await
    }
}

async fn _run<B: BrokerListener + Send + Sync + 'static>(
    metrics: AuthorityClaimerMetrics,
    config: Config,
    broker_listener: B,
) -> Result<(), Box<dyn Error>> {
    let http_server_handle =
        http_server::start(config.http_server_config, metrics.clone().into());
    let config = config.authority_claimer_config;

    let chain_id = config.tx_manager_config.chain_id;

    trace!("Creating the duplicate checker");
    let duplicate_checker = DefaultDuplicateChecker::new(
        config.tx_manager_config.provider_http_endpoint.clone(),
        config.contracts_config.history_address.clone(),
        config.tx_manager_config.default_confirmations,
        config.genesis_block,
    )
    .await?;

    trace!("Creating the transaction sender");
    let transaction_sender =
        DefaultTransactionSender::new(config.clone(), chain_id, metrics)
            .await?;

    // Creating the claimer.
    let claimer = DefaultClaimer::new(
        broker_listener,
        duplicate_checker,
        transaction_sender,
    );

    let claimer_handle = claimer.start();

    // Starting the HTTP server and the claimer loop.
    tokio::select! {
        ret = http_server_handle => { ret? }
        ret = claimer_handle     => { ret? }
    };

    unreachable!()
}
