// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use advance_runner::config::AdvanceRunnerConfig;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let config = AdvanceRunnerConfig::parse()?;

    log::configure(&config.log_config);

    log::log_service_start(&config, "Advance Runner");

    advance_runner::run(config).await.map_err(|e| e.into())
}
