// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::{
    log::{LogConfig, LogEnvCliConfig},
    redacted::Redacted,
    rollups_events::{Address, BrokerCLIConfig, BrokerConfig},
};
use clap::{command, Parser};
use eth_tx_manager::{
    config::{
        Error as TxManagerConfigError, TxEnvCLIConfig as TxManagerCLIConfig,
        TxManagerConfig,
    },
    Priority,
};
use rusoto_core::{region::ParseRegionError, Region};
use snafu::{ResultExt, Snafu};
use std::{fs, str::FromStr};

#[derive(Debug, Snafu)]
#[snafu(visibility(pub(crate)))]
pub enum AuthorityClaimerConfigError {
    #[snafu(display("TxManager configuration error"))]
    TxManager { source: TxManagerConfigError },

    #[snafu(display("parse IConsensus address error"))]
    ParseIConsensusAddress { source: serde_json::Error },

    #[snafu(display("Missing auth configuration"))]
    AuthConfigMissing,

    #[snafu(display("Could not read mnemonic file at path `{}`", path,))]
    MnemonicFileError {
        path: String,
        source: std::io::Error,
    },

    #[snafu(display("Missing AWS region"))]
    MissingRegion,

    #[snafu(display("Invalid AWS region"))]
    InvalidRegion { source: ParseRegionError },
}

#[derive(Debug, Clone)]
pub struct Config {
    pub tx_manager_config: TxManagerConfig,
    pub tx_signing_config: TxSigningConfig,
    pub tx_manager_priority: Priority,
    pub broker_config: BrokerConfig,
    pub log_config: LogConfig,
    pub iconsensus_address: Address,
    pub genesis_block: u64,
    pub http_server_port: u16,
}

#[derive(Debug, Clone)]
pub enum TxSigningConfig {
    PrivateKey {
        private_key: Redacted<String>,
    },

    Mnemonic {
        mnemonic: Redacted<String>,
        account_index: Option<u32>,
    },

    Aws {
        key_id: String,
        region: Region,
    },
}

impl Config {
    pub fn new() -> Result<Self, AuthorityClaimerConfigError> {
        let cli_config = AuthorityClaimerCLI::parse();

        let tx_manager_config =
            TxManagerConfig::initialize(cli_config.tx_manager_config)
                .context(TxManagerSnafu)?;

        let tx_signing_config =
            TxSigningConfig::try_from(cli_config.tx_signing_config)?;

        let broker_config = BrokerConfig::from(cli_config.broker_config);

        let log_config = LogConfig::initialize(cli_config.log_config);

        let iconsensus_address =
            serde_json::from_str(&cli_config.iconsensus_address)
                .context(ParseIConsensusAddressSnafu)?;

        Ok(Config {
            tx_manager_config,
            tx_signing_config,
            tx_manager_priority: Priority::Normal,
            broker_config,
            log_config,
            iconsensus_address,
            genesis_block: cli_config.genesis_block,
            http_server_port: cli_config.http_server_port,
        })
    }
}

#[derive(Parser)]
#[command(name = "authority_claimer_config")]
#[command(about = "Configuration for authority-claimer")]
struct AuthorityClaimerCLI {
    #[command(flatten)]
    pub tx_manager_config: TxManagerCLIConfig,

    #[command(flatten)]
    pub tx_signing_config: TxSigningCLIConfig,

    #[command(flatten)]
    pub broker_config: BrokerCLIConfig,

    #[command(flatten)]
    pub log_config: LogEnvCliConfig,

    /// Address of the IConsensus contract
    #[arg(long, env)]
    pub iconsensus_address: String,

    /// Genesis block for reading blockchain events
    #[arg(long, env, default_value_t = 1)]
    pub genesis_block: u64,

    /// Port of the authority-claimer HTTP server
    #[arg(long, env, default_value_t = 8080)]
    pub http_server_port: u16,
}

#[derive(Debug, Parser)]
#[command(name = "tx_signing_config")]
struct TxSigningCLIConfig {
    /// Signer private key, overrides `tx_signing_private_key_file`, `tx_signing_mnemonic` , `tx_signing_mnemonic_file` and `tx_signing_aws_kms_*`
    #[arg(long, env)]
    tx_signing_private_key: Option<String>,

    /// Signer private key file, overrides `tx_signing_mnemonic` , `tx_signing_mnemonic_file` and `tx_signing_aws_kms_*`
    #[arg(long, env)]
    tx_signing_private_key_file: Option<String>,

    /// Signer mnemonic, overrides `tx_signing_mnemonic_file` and `tx_signing_aws_kms_*`
    #[arg(long, env)]
    tx_signing_mnemonic: Option<String>,

    /// Signer mnemonic file path, overrides `tx_signing_aws_kms_*`
    #[arg(long, env)]
    tx_signing_mnemonic_file: Option<String>,

    /// Mnemonic account index
    #[arg(long, env)]
    tx_signing_mnemonic_account_index: Option<u32>,

    /// AWS KMS signer key-id
    #[arg(long, env)]
    tx_signing_aws_kms_key_id: Option<String>,

    /// AWS KMS signer region
    #[arg(long, env)]
    tx_signing_aws_kms_region: Option<String>,
}

impl TryFrom<TxSigningCLIConfig> for TxSigningConfig {
    type Error = AuthorityClaimerConfigError;

    fn try_from(cli: TxSigningCLIConfig) -> Result<Self, Self::Error> {
        let account_index = cli.tx_signing_mnemonic_account_index;
        if let Some(private_key) = cli.tx_signing_private_key {
            Ok(TxSigningConfig::PrivateKey {
                private_key: Redacted::new(private_key),
            })
        } else if let Some(path) = cli.tx_signing_private_key_file {
            let private_key = fs::read_to_string(path.clone())
                .context(MnemonicFileSnafu { path })?
                .trim()
                .to_string();
            Ok(TxSigningConfig::PrivateKey {
                private_key: Redacted::new(private_key),
            })
        } else if let Some(mnemonic) = cli.tx_signing_mnemonic {
            Ok(TxSigningConfig::Mnemonic {
                mnemonic: Redacted::new(mnemonic),
                account_index,
            })
        } else if let Some(path) = cli.tx_signing_mnemonic_file {
            let mnemonic = fs::read_to_string(path.clone())
                .context(MnemonicFileSnafu { path })?
                .trim()
                .to_string();
            Ok(TxSigningConfig::Mnemonic {
                mnemonic: Redacted::new(mnemonic),
                account_index,
            })
        } else {
            match (cli.tx_signing_aws_kms_key_id, cli.tx_signing_aws_kms_region)
            {
                (None, _) => {
                    Err(AuthorityClaimerConfigError::AuthConfigMissing)
                }
                (Some(_), None) => {
                    Err(AuthorityClaimerConfigError::MissingRegion)
                }
                (Some(key_id), Some(region)) => {
                    let region = Region::from_str(&region)
                        .context(InvalidRegionSnafu)?;
                    Ok(TxSigningConfig::Aws { key_id, region })
                }
            }
        }
    }
}
