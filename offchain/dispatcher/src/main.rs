// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use log;
use tracing::info;

// NOTE: doesn't support History upgradability.
// NOTE: doesn't support changing epoch_duration in the middle of things.
#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let config = dispatcher::config::Config::initialize()?;

    log::configure(&config.dispatcher_config.log_config);

    info!(?config, "Starting Dispatcher");
    dispatcher::run(config).await.map_err(|e| e.into())
}
