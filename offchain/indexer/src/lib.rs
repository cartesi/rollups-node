// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use snafu::ResultExt;

pub use config::{CLIConfig, IndexerConfig};
pub use error::IndexerError;

pub mod config;
mod conversions;
mod error;
mod indexer;

#[tracing::instrument(level = "trace", skip_all)]
pub async fn run(config: IndexerConfig) -> Result<(), IndexerError> {
    let health_handle = http_health_check::start(config.healthcheck_port);
    let indexer_handle = indexer::Indexer::start(config);
    tokio::select! {
        ret = health_handle => {
            ret.context(error::HealthCheckSnafu)
        }
        ret = indexer_handle => {
            ret
        }
    }
}
