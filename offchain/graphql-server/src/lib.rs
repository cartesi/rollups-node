// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use snafu::ResultExt;

pub use config::{CLIConfig, GraphQLConfig};
pub use error::GraphQLServerError;
pub use http::start_service;
pub use schema::Context;

pub mod config;
mod error;
pub mod http;
pub mod schema;

#[tracing::instrument(level = "trace", skip_all)]
pub async fn run(config: GraphQLConfig) -> Result<(), GraphQLServerError> {
    let repository = rollups_data::Repository::new(config.repository_config)
        .expect("failed to connect to database");
    let context = Context::new(repository);
    let service_handler =
        start_service(&config.graphql_host, config.graphql_port, context)
            .expect("failed to create server");

    let health_handle = http_health_check::start(config.healthcheck_port);

    tokio::select! {
        ret = health_handle => {
            ret.context(error::HealthCheckSnafu)
        }
        ret = service_handler => {
            ret.context(error::ServerSnafu)
        }
    }
}
