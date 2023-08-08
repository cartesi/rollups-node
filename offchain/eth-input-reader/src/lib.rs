// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

pub mod config;
pub mod eth_input_reader;
pub mod machine;

mod error;
mod metrics;
mod setup;

use config::Config;
use error::EthInputReaderError;
use metrics::EthInputReaderMetrics;
use snafu::ResultExt;

#[tracing::instrument(level = "trace", skip_all)]
pub async fn run(config: Config) -> Result<(), EthInputReaderError> {
    let metrics = EthInputReaderMetrics::default();
    let input_reader_handle = eth_input_reader::start(
        config.eth_input_reader_config,
        metrics.clone(),
    );
    let http_server_handle =
        http_server::start(config.http_server_config, metrics.into());
    tokio::select! {
        ret = http_server_handle => {
            ret.context(error::HttpServerSnafu)
        }
        ret = input_reader_handle => {
            ret
        }
    }
}
