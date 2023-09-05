// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use error::InspectError;
use snafu::ResultExt;

pub use config::InspectServerConfig;
pub use inspect::InspectClient;

pub mod config;
mod error;
pub mod grpc;
pub mod inspect;
pub mod server;

#[tracing::instrument(level = "trace", skip_all)]
pub async fn run(config: InspectServerConfig) -> Result<(), InspectError> {
    let health_handle = http_health_check::start(config.healthcheck_port);
    let inspect_client = InspectClient::new(&config);
    let inspect_server =
        server::create(&config, inspect_client).context(error::ServerSnafu)?;
    tokio::select! {
        ret = health_handle => {
            ret.context(error::HealthCheckSnafu)
        }
        ret = inspect_server => {
            ret.context(error::ServerSnafu)
        }
    }
}
