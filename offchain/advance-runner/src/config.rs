// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;
use std::time::Duration;

use crate::server_manager::ServerManagerCLIConfig;
pub use crate::server_manager::ServerManagerConfig;
use log::{LogConfig, LogEnvCliConfig};
pub use rollups_events::{
    BrokerCLIConfig, BrokerConfig, DAppMetadata, DAppMetadataCLIConfig,
};

#[derive(Debug, Clone)]
pub struct AdvanceRunnerConfig {
    pub server_manager_config: ServerManagerConfig,
    pub broker_config: BrokerConfig,
    pub dapp_metadata: DAppMetadata,
    pub log_config: LogConfig,
    pub backoff_max_elapsed_duration: Duration,
    pub healthcheck_port: u16,
    pub reader_mode: bool,
}

impl AdvanceRunnerConfig {
    pub fn parse() -> Self {
        let cli_config = CLIConfig::parse();
        let broker_config = cli_config.broker_cli_config.into();
        let dapp_metadata: DAppMetadata =
            cli_config.dapp_metadata_cli_config.into();
        let server_manager_config =
            ServerManagerConfig::parse_from_cli(cli_config.sm_cli_config);

        let log_config = LogConfig::initialize(cli_config.log_cli_config);

        let backoff_max_elapsed_duration =
            Duration::from_millis(cli_config.backoff_max_elapsed_duration);

        let healthcheck_port = cli_config.healthcheck_port;

        let reader_mode = cli_config.reader_mode;

        Self {
            server_manager_config,
            broker_config,
            dapp_metadata,
            log_config,
            backoff_max_elapsed_duration,
            healthcheck_port,
            reader_mode,
        }
    }
}

#[derive(Parser)]
#[command(name = "advance_runner_config")]
#[command(about = "Configuration for advance-runner")]
struct CLIConfig {
    #[command(flatten)]
    sm_cli_config: ServerManagerCLIConfig,

    #[command(flatten)]
    broker_cli_config: BrokerCLIConfig,

    #[command(flatten)]
    dapp_metadata_cli_config: DAppMetadataCLIConfig,

    #[command(flatten)]
    pub log_cli_config: LogEnvCliConfig,

    /// The max elapsed time for backoff in ms
    #[arg(long, env, default_value = "120000")]
    backoff_max_elapsed_duration: u64,

    /// Port of health check
    #[arg(
        long,
        env = "ADVANCE_RUNNER_HEALTHCHECK_PORT",
        default_value_t = 8080
    )]
    pub healthcheck_port: u16,

    #[arg(long, env)]
    reader_mode: bool,
}
