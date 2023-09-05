// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;
use snafu::{ResultExt, Snafu};
use std::time::Duration;

use crate::server_manager::ServerManagerCLIConfig;
pub use crate::server_manager::ServerManagerConfig;
pub use crate::snapshot::config::{FSManagerConfig, SnapshotConfig};
use crate::snapshot::config::{SnapshotCLIConfig, SnapshotConfigError};
use log::{LogConfig, LogEnvCliConfig};
pub use rollups_events::{
    BrokerCLIConfig, BrokerConfig, DAppMetadata, DAppMetadataCLIConfig,
};

#[derive(Debug, Clone)]
pub struct AdvanceRunnerConfig {
    pub server_manager_config: ServerManagerConfig,
    pub broker_config: BrokerConfig,
    pub dapp_metadata: DAppMetadata,
    pub snapshot_config: SnapshotConfig,
    pub log_config: LogConfig,
    pub backoff_max_elapsed_duration: Duration,
    pub healthcheck_port: u16,
}

impl AdvanceRunnerConfig {
    pub fn parse() -> Result<Self, ConfigError> {
        let cli_config = CLIConfig::parse();
        let broker_config = cli_config.broker_cli_config.into();
        let dapp_metadata = cli_config.dapp_metadata_cli_config.into();
        let server_manager_config =
            ServerManagerConfig::parse_from_cli(cli_config.sm_cli_config);
        let snapshot_config =
            SnapshotConfig::parse_from_cli(cli_config.snapshot_cli_config)
                .context(SnapshotConfigSnafu)?;
        let backoff_max_elapsed_duration =
            Duration::from_millis(cli_config.backoff_max_elapsed_duration);
        let healthcheck_port = cli_config.healthcheck_port;

        let log_config = LogConfig::initialize(cli_config.log_cli_config);

        Ok(Self {
            server_manager_config,
            broker_config,
            dapp_metadata,
            snapshot_config,
            log_config,
            backoff_max_elapsed_duration,
            healthcheck_port,
        })
    }
}

#[derive(Debug, Snafu)]
pub enum ConfigError {
    #[snafu(display("error in snapshot configuration"))]
    SnapshotConfigError { source: SnapshotConfigError },
}

#[derive(Parser, Debug)]
struct CLIConfig {
    #[command(flatten)]
    sm_cli_config: ServerManagerCLIConfig,

    #[command(flatten)]
    broker_cli_config: BrokerCLIConfig,

    #[command(flatten)]
    dapp_metadata_cli_config: DAppMetadataCLIConfig,

    #[command(flatten)]
    snapshot_cli_config: SnapshotCLIConfig,

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
}
