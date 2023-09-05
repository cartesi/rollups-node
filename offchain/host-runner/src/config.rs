// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;
use log::{LogConfig, LogEnvCliConfig};

const DEFAULT_ADDRESS: &str = "0.0.0.0";
#[derive(Debug, Clone)]
pub struct Config {
    pub log_config: LogConfig,
    pub grpc_server_manager_address: String,
    pub grpc_server_manager_port: u16,
    pub http_inspect_address: String,
    pub http_inspect_port: u16,
    pub http_rollup_server_address: String,
    pub http_rollup_server_port: u16,
    pub finish_timeout: u64,
    pub healthcheck_port: u16,
}

#[derive(Parser, Clone, Debug)]
pub struct CLIConfig {
    /// Logs Config
    #[command(flatten)]
    pub log_config: LogEnvCliConfig,

    /// gRPC address of the Server Manager endpoint
    #[arg(long, env, default_value = DEFAULT_ADDRESS)]
    pub grpc_server_manager_address: String,

    /// gRPC port of the Server Manager endpoint
    #[arg(long, env, default_value = "5001")]
    pub grpc_server_manager_port: u16,

    /// HTTP address of the Inspect endpoint
    #[arg(long, env, default_value = DEFAULT_ADDRESS)]
    pub http_inspect_address: String,

    /// HTTP port of the Inspect endpoint
    #[arg(long, env, default_value = "5002")]
    pub http_inspect_port: u16,

    /// HTTP address of the Rollup Server endpoint
    #[arg(long, env, default_value = DEFAULT_ADDRESS)]
    pub http_rollup_server_address: String,

    /// HTTP port of the Rollup Server endpoint
    #[arg(long, env, default_value = "5004")]
    pub http_rollup_server_port: u16,

    /// Duration in ms for the finish request to timeout
    #[arg(long, env, default_value = "10000")]
    pub finish_timeout: u64,

    /// Port of health check
    #[arg(long, env = "HOST_RUNNER_HEALTHCHECK_PORT", default_value_t = 8080)]
    pub healthcheck_port: u16,
}

impl From<CLIConfig> for Config {
    fn from(cli_config: CLIConfig) -> Self {
        Self {
            log_config: cli_config.log_config.into(),
            grpc_server_manager_address: cli_config.grpc_server_manager_address,
            grpc_server_manager_port: cli_config.grpc_server_manager_port,
            http_inspect_address: cli_config.http_inspect_address,
            http_inspect_port: cli_config.http_inspect_port,
            http_rollup_server_address: cli_config.http_rollup_server_address,
            http_rollup_server_port: cli_config.http_rollup_server_port,
            finish_timeout: cli_config.finish_timeout,
            healthcheck_port: cli_config.healthcheck_port,
        }
    }
}
