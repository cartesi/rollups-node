// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;

use indexer::{CLIConfig, IndexerConfig};
use log;
use tracing::info;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let config: IndexerConfig = CLIConfig::parse().into();

    log::configure(&config.log_config);

    info!(?config, "Starting Indexer");
    indexer::run(config).await.map_err(|e| e.into())
}
