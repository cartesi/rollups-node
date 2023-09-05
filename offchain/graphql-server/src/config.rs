// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;
use log::{LogConfig, LogEnvCliConfig};
use rollups_data::{RepositoryCLIConfig, RepositoryConfig};

#[derive(Debug)]
pub struct GraphQLConfig {
    pub repository_config: RepositoryConfig,
    pub log_config: LogConfig,
    pub graphql_host: String,
    pub graphql_port: u16,
    pub healthcheck_port: u16,
}

#[derive(Parser)]
pub struct CLIConfig {
    #[command(flatten)]
    repository_config: RepositoryCLIConfig,

    #[command(flatten)]
    pub log_config: LogEnvCliConfig,

    #[arg(long, env, default_value = "127.0.0.1")]
    pub graphql_host: String,

    #[arg(long, env, default_value_t = 4000)]
    pub graphql_port: u16,

    /// Port of health check
    #[arg(long, env = "GRAPHQL_HEALTHCHECK_PORT", default_value_t = 8080)]
    pub healthcheck_port: u16,
}

impl From<CLIConfig> for GraphQLConfig {
    fn from(cli_config: CLIConfig) -> Self {
        Self {
            repository_config: cli_config.repository_config.into(),
            log_config: cli_config.log_config.into(),
            graphql_host: cli_config.graphql_host,
            graphql_port: cli_config.graphql_port,
            healthcheck_port: cli_config.healthcheck_port,
        }
    }
}
