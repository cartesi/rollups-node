// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;
use eth_state_client_lib::config::{
    Error as SCError, SCConfig, SCEnvCLIConfig,
};
use http_server::HttpServerConfig;
use log::{LogConfig, LogEnvCliConfig};
use snafu::{ResultExt, Snafu};
use types::blockchain_config::{
    BlockchainCLIConfig, BlockchainConfig, BlockchainConfigError,
};

use rollups_events::{BrokerCLIConfig, BrokerConfig};

#[derive(Parser)]
#[command(name = "rd_config")]
#[command(about = "Configuration for dispatcher")]
pub struct DispatcherEnvCLIConfig {
    #[command(flatten)]
    pub sc_config: SCEnvCLIConfig,

    #[command(flatten)]
    pub broker_config: BrokerCLIConfig,

    #[command(flatten)]
    pub log_config: LogEnvCliConfig,

    #[command(flatten)]
    pub blockchain_config: BlockchainCLIConfig,

    /// Duration of rollups epoch in blocks, for which dispatcher will make claims.
    #[arg(long, env, default_value = "7200")]
    pub rd_epoch_duration: u64,

    /// Chain ID
    #[arg(long, env)]
    pub chain_id: u64,
}

#[derive(Clone, Debug)]
pub struct DispatcherConfig {
    pub sc_config: SCConfig,
    pub broker_config: BrokerConfig,
    pub log_config: LogConfig,
    pub blockchain_config: BlockchainConfig,

    pub epoch_duration: u64,
    pub chain_id: u64,
}

#[derive(Debug, Snafu)]
pub enum Error {
    #[snafu(display("StateClient configuration error"))]
    StateClientError { source: SCError },

    #[snafu(display("Blockchain configuration error"))]
    BlockchainError { source: BlockchainConfigError },
}

#[derive(Debug)]
pub struct Config {
    pub dispatcher_config: DispatcherConfig,
    pub http_server_config: HttpServerConfig,
}

impl Config {
    pub fn initialize() -> Result<Self, Error> {
        let (http_server_config, dispatcher_config) =
            HttpServerConfig::parse::<DispatcherEnvCLIConfig>("dispatcher");

        let sc_config = SCConfig::initialize(dispatcher_config.sc_config)
            .context(StateClientSnafu)?;

        let log_config = LogConfig::initialize(dispatcher_config.log_config);

        let blockchain_config =
            BlockchainConfig::try_from(dispatcher_config.blockchain_config)
                .context(BlockchainSnafu)?;

        let broker_config = BrokerConfig::from(dispatcher_config.broker_config);

        let dispatcher_config = DispatcherConfig {
            sc_config,
            broker_config,
            log_config,
            blockchain_config,
            epoch_duration: dispatcher_config.rd_epoch_duration,
            chain_id: dispatcher_config.chain_id,
        };

        Ok(Config {
            dispatcher_config,
            http_server_config,
        })
    }
}
