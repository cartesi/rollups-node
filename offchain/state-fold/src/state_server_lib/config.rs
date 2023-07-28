// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;
use snafu::{ResultExt, Snafu};

use crate::block_history::config::{BHConfig, BHEnvCLIConfig};
use crate::state_fold::config::{SFConfig, SFEnvCLIConfig};

#[derive(Debug, Clone, Parser)]
#[command(name = "sate_server_config")]
#[command(about = "Configuration for state-fold state-server")]
pub struct StateServerEnvCLIConfig {
    #[command(flatten)]
    pub state_fold: SFEnvCLIConfig,

    #[command(flatten)]
    pub block_history: BHEnvCLIConfig,

    /// Server address
    #[arg(long, env)]
    pub ss_server_address: Option<String>,
}

#[derive(Clone, Debug)]
pub struct StateServerConfig {
    pub state_fold: SFConfig,
    pub block_history: BHConfig,
    pub server_address: std::net::SocketAddr,
}

#[derive(Debug, Snafu)]
pub enum ConfigError {
    #[snafu(display("Error loading block-history configuration: {}", source))]
    AddressParseError { source: std::net::AddrParseError },
}
pub type Result<T> = std::result::Result<T, ConfigError>;

const DEFAULT_SERVER_ADDRESS: &str = "0.0.0.0:50051";

impl StateServerConfig {
    pub fn initialize_from_args() -> Result<Self> {
        let env_cli_config = StateServerEnvCLIConfig::parse();
        Self::initialize(env_cli_config)
    }

    pub fn initialize(env_cli_config: StateServerEnvCLIConfig) -> Result<Self> {
        let state_fold = SFConfig::initialize(env_cli_config.state_fold);
        let block_history = BHConfig::initialize(env_cli_config.block_history);

        let server_address: std::net::SocketAddr = env_cli_config
            .ss_server_address
            .unwrap_or(DEFAULT_SERVER_ADDRESS.to_string())
            .parse()
            .context(AddressParseSnafu)?;

        Ok(Self {
            state_fold,
            block_history,
            server_address,
        })
    }
}
