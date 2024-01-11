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
use snafu::Error;
use tracing::trace;

use crate::{
    checker::DefaultDuplicateChecker,
    claimer::{Claimer, DefaultClaimer},
    listener::DefaultBrokerListener,
    metrics::AuthorityClaimerMetrics,
    sender::DefaultTransactionSender,
};

pub async fn run(config: Config) -> Result<(), Box<dyn Error>> {
    // Creating the metrics and health server.
    let metrics = AuthorityClaimerMetrics::new();
    let http_server_handle =
        http_server::start(config.http_server_config, metrics.clone().into());

    let config = config.authority_claimer_config;
    let chain_id = config.tx_manager_config.chain_id;

    // Creating the broker listener.
    trace!("Creating the broker listener");
    let broker_listener =
        DefaultBrokerListener::new(config.broker_config.clone(), chain_id)
            .await?;

    // Creating the duplicate checker.
    trace!("Creating the duplicate checker");
    let duplicate_checker = DefaultDuplicateChecker::new(
        config.tx_manager_config.provider_http_endpoint.clone(),
        config.contracts_config.history_address.clone(),
        config.tx_manager_config.default_confirmations,
        config.genesis_block,
    )
    .await?;

    // Creating the transaction sender.
    trace!("Creating the transaction sender");
    let transaction_sender =
        DefaultTransactionSender::new(config.clone(), chain_id, metrics)
            .await?;

    // Creating the claimer loop.
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
