// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

mod checker;
mod claimer;
mod config;
mod contracts;
mod http_server;
mod listener;
pub mod log;
mod metrics;
mod redacted;
mod rollups_events;
mod sender;
mod signer;
mod types;

#[cfg(test)]
mod test_fixtures;

use crate::{
    checker::DefaultDuplicateChecker,
    claimer::{Claimer, DefaultClaimer},
    listener::DefaultBrokerListener,
    metrics::AuthorityClaimerMetrics,
    sender::DefaultTransactionSender,
};
pub use config::Config;
use snafu::Error;
use tracing::trace;

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
