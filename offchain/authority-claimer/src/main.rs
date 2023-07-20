// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use authority_claimer::config::Config;
use std::error::Error;
use tracing_subscriber::filter::{EnvFilter, LevelFilter};

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    // Settin up the logging environment.
    let env_filter = EnvFilter::builder()
        .with_default_directive(LevelFilter::INFO.into())
        .from_env_lossy();
    tracing_subscriber::fmt().with_env_filter(env_filter).init();

    // Getting the configuration.
    let config = Config::new().map_err(Box::new)?;

    authority_claimer::run(config).await
}
