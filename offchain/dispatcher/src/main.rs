// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// NOTE: doesn't support History upgradability.
// NOTE: doesn't support changing epoch_duration in the middle of things.
#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let config = dispatcher::config::Config::initialize()?;

    log::configure(&config.dispatcher_config.log_config);

    log::log_service_start(&config, "Dispatcher");

    dispatcher::run(config).await.map_err(|e| e.into())
}
