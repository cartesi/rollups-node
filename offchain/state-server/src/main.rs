// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use tracing_subscriber::filter::{EnvFilter, LevelFilter};
use types::foldables::authority::rollups::RollupsState;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let filter = EnvFilter::builder()
        .with_default_directive(LevelFilter::INFO.into())
        .from_env_lossy();
    tracing_subscriber::fmt().with_env_filter(filter).init();

    let config =
    state_fold::state_server_lib::config::StateServerConfig::initialize_from_args()?;

    tracing::info!(?config, "starting state server");

    state_server::run_server::<RollupsState>(config)
        .await
        .map_err(|e| e.into())
}
