// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

mod cli;
mod error;

pub use error::{AuthorityClaimerConfigError, TxSigningConfigError};

use cli::AuthorityClaimerCLI;
use eth_tx_manager::{config::TxManagerConfig, Priority};
use http_server::HttpServerConfig;
use log::LogConfig;
use rollups_events::BrokerConfig;
use rusoto_core::Region;
use types::blockchain_config::BlockchainConfig;

#[derive(Debug, Clone)]
pub struct Config {
    pub authority_claimer_config: AuthorityClaimerConfig,
    pub http_server_config: HttpServerConfig,
}

#[derive(Debug, Clone)]
pub struct AuthorityClaimerConfig {
    pub tx_manager_config: TxManagerConfig,
    pub tx_signing_config: TxSigningConfig,
    pub tx_manager_priority: Priority,
    pub broker_config: BrokerConfig,
    pub log_config: LogConfig,
    pub blockchain_config: BlockchainConfig,
}

#[derive(Debug, Clone)]
pub enum TxSigningConfig {
    Mnemonic {
        mnemonic: String,
        account_index: Option<u32>,
    },

    Aws {
        key_id: String,
        region: Region,
    },
}

impl Config {
    pub fn new() -> Result<Self, AuthorityClaimerConfigError> {
        let (http_server_config, authority_claimer_cli) =
            HttpServerConfig::parse::<AuthorityClaimerCLI>("authority_claimer");
        let authority_claimer_config = authority_claimer_cli.try_into()?;
        Ok(Self {
            authority_claimer_config,
            http_server_config,
        })
    }
}
