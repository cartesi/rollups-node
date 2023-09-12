// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use authority_claimer::config::Config;
use std::error::Error;
use tracing::info;

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    // Getting the configuration.
    let config: Config = Config::new().map_err(Box::new)?;

    // Setting up the logging environment.
    log::configure(&config.authority_claimer_config.log_config);

    info!(?config, "Starting Authority Claimer");

    authority_claimer::run(config).await
}
