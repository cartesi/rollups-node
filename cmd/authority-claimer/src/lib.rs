// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

mod checker;
mod claimer;
mod config;
mod contracts;
mod http_server;
pub mod log;
mod metrics;
mod redacted;
mod repository;
mod rollups_events;
mod sender;
mod signer;

use checker::DefaultDuplicateChecker;
use claimer::{Claimer, DefaultClaimer};
pub use config::Config;
use ethers::signers::Signer;
use metrics::AuthorityClaimerMetrics;
use repository::DefaultRepository;
use sender::DefaultTransactionSender;
use signer::ConditionalSigner;
use snafu::Error;
use tracing::trace;

pub async fn run(config: Config) -> Result<(), Box<dyn Error>> {
    // Creating the metrics and health server.
    let metrics = AuthorityClaimerMetrics::new();
    let http_server_handle =
        http_server::start(config.http_server_port, metrics.clone().into());

    let chain_id = config.tx_manager_config.chain_id;

    // Creating repository.
    trace!("Creating the repository");
    let repository = DefaultRepository::new(config.postgres_endpoint)?;

    // Creating the conditional signer.
    let conditional_signer =
        ConditionalSigner::new(chain_id, &config.tx_signing_config).await?;
    let from = conditional_signer.address();

    // Creating the duplicate checker.
    trace!("Creating the duplicate checker");
    let duplicate_checker = DefaultDuplicateChecker::new(
        config.tx_manager_config.provider_http_endpoint.clone(),
        from,
        config.tx_manager_config.default_confirmations,
        config.genesis_block,
    )
    .await?;

    // Creating the transaction sender.
    trace!("Creating the transaction sender");
    let transaction_sender = DefaultTransactionSender::new(
        config.tx_manager_config,
        config.tx_manager_priority,
        conditional_signer,
        from,
        chain_id,
        metrics,
    )
    .await?;

    // Creating the claimer loop.
    let claimer =
        DefaultClaimer::new(repository, duplicate_checker, transaction_sender);
    let claimer_handle = claimer.start();

    // Starting the HTTP server and the claimer loop.
    tokio::select! {
        ret = http_server_handle => { ret? }
        ret = claimer_handle     => { ret? }
    };

    unreachable!()
}
