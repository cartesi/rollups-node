// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use backoff::ExponentialBackoffBuilder;
use broker::BrokerFacade;
use config::AdvanceRunnerConfig;
use runner::Runner;
use server_manager::ServerManagerFacade;
use snafu::ResultExt;

pub use broker::BrokerFacadeError;
pub use error::AdvanceRunnerError;
pub use runner::RunnerError;

mod broker;
pub mod config;
mod error;
pub mod runner;
mod server_manager;

#[tracing::instrument(level = "trace", skip_all)]
pub async fn run(
    config: AdvanceRunnerConfig,
) -> Result<(), AdvanceRunnerError> {
    let health_handle = http_health_check::start(config.healthcheck_port);
    let advance_runner_handle = start_advance_runner(config);
    tokio::select! {
        ret = health_handle => {
            ret.context(error::HealthCheckSnafu)
        }
        ret = advance_runner_handle => {
            ret
        }
    }
}

#[tracing::instrument(level = "trace", skip_all)]
async fn start_advance_runner(
    config: AdvanceRunnerConfig,
) -> Result<(), AdvanceRunnerError> {
    let backoff = ExponentialBackoffBuilder::new()
        .with_max_elapsed_time(Some(config.backoff_max_elapsed_duration))
        .build();

    let server_manager = ServerManagerFacade::new(
        config.dapp_metadata.dapp_address.clone(),
        config.server_manager_config,
        backoff,
    )
    .await
    .context(error::ServerManagerSnafu)?;
    tracing::trace!("connected to the server-manager");

    let broker = BrokerFacade::new(
        config.broker_config,
        config.dapp_metadata,
        config.reader_mode,
    )
    .await
    .context(error::BrokerSnafu)?;
    tracing::trace!("connected the broker");

    Runner::start(server_manager, broker)
        .await
        .context(error::RunnerSnafu)
}
