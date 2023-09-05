// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

pub mod checker;
pub mod claimer;
pub mod config;
pub mod listener;
pub mod metrics;
pub mod sender;

use config::Config;
use rollups_events::DAppMetadata;
use snafu::Error;
use tracing::trace;

use crate::{
    checker::DefaultDuplicateChecker,
    claimer::{AuthorityClaimer, DefaultAuthorityClaimer},
    listener::DefaultBrokerListener,
    metrics::AuthorityClaimerMetrics,
    sender::DefaultTransactionSender,
};

pub async fn run(config: Config) -> Result<(), Box<dyn Error>> {
    // Creating the metrics and health server.
    let metrics = AuthorityClaimerMetrics::new();
    let http_server_handle =
        http_server::start(config.http_server_config, metrics.clone().into());

    let dapp_address = config.authority_claimer_config.dapp_address;
    let dapp_metadata = DAppMetadata {
        chain_id: config.authority_claimer_config.tx_manager_config.chain_id,
        dapp_address,
    };

    // Creating the broker listener.
    trace!("Creating the broker listener");
    let broker_listener = DefaultBrokerListener::new(
        config.authority_claimer_config.broker_config.clone(),
        dapp_metadata.clone(),
        metrics.clone(),
    )
    .map_err(Box::new)?;

    // Creating the duplicate checker.
    trace!("Creating the duplicate checker");
    let duplicate_checker = DefaultDuplicateChecker::new().map_err(Box::new)?;

    // Creating the transaction sender.
    trace!("Creating the transaction sender");
    let transaction_sender =
        DefaultTransactionSender::new(dapp_metadata, metrics)
            .map_err(Box::new)?;

    // Creating the claimer loop.
    let authority_claimer = DefaultAuthorityClaimer::new();
    let claimer_handle = authority_claimer.start(
        broker_listener,
        duplicate_checker,
        transaction_sender,
    );

    // Starting the HTTP server and the claimer loop.
    tokio::select! {
        ret = http_server_handle => { ret.map_err(Box::new)? }
        ret = claimer_handle     => { ret.map_err(Box::new)? }
    };

    unreachable!()
}
