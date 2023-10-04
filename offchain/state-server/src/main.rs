// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
mod config;
use config::Config;
use types::foldables::authority::rollups::RollupsState;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let config: Config = Config::initialize_from_args()?;

    log::configure(&config.log_config);

    log::log_service_start(&config, "State Server");

    state_server::run_server::<RollupsState>(config.state_server_config)
        .await
        .map_err(|e| e.into())
}
