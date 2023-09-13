// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::{command, Parser};
use eth_tx_manager::{
    config::{TxEnvCLIConfig as TxManagerCLIConfig, TxManagerConfig},
    Priority,
};
use log::{LogConfig, LogEnvCliConfig};
use rollups_events::{BrokerCLIConfig, BrokerConfig};
use rusoto_core::Region;
use snafu::ResultExt;
use std::{fs, path::PathBuf, str::FromStr};

use crate::auth::{AuthConfig, AuthEnvCLIConfig};
use crate::config::{
    error::{
        AuthSnafu, AuthorityClaimerConfigError, InvalidRegionSnafu,
        MnemonicFileSnafu, TxManagerSnafu, TxSigningConfigError,
        TxSigningSnafu,
    },
    json::{
        read_json_file, DappDeployment, RollupsDeployment,
        RollupsDeploymentJson,
    },
    AuthorityClaimerConfig, TxSigningConfig,
};

// ------------------------------------------------------------------------------------------------
// AuthorityClaimerCLI
// ------------------------------------------------------------------------------------------------

#[derive(Parser)]
#[command(name = "authority_claimer_config")]
#[command(about = "Configuration for authority-claimer")]
pub(crate) struct AuthorityClaimerCLI {
    #[command(flatten)]
    tx_manager_config: TxManagerCLIConfig,

    #[command(flatten)]
    tx_signing_config: TxSigningCLIConfig,

    #[command(flatten)]
    broker_config: BrokerCLIConfig,

    #[command(flatten)]
    pub log_config: LogEnvCliConfig,

    #[command(flatten)]
    pub auth_config: AuthEnvCLIConfig,

    /// Path to a file with the deployment json of the dapp
    #[arg(long, env, default_value = "./dapp_deployment.json")]
    dapp_deployment_file: PathBuf,

    /// Path to file with deployment json of rollups
    #[arg(long, env, default_value = "./rollups_deployment.json")]
    pub rollups_deployment_file: PathBuf,
}

impl TryFrom<AuthorityClaimerCLI> for AuthorityClaimerConfig {
    type Error = AuthorityClaimerConfigError;

    fn try_from(cli_config: AuthorityClaimerCLI) -> Result<Self, Self::Error> {
        let tx_manager_config =
            TxManagerConfig::initialize(cli_config.tx_manager_config)
                .context(TxManagerSnafu)?;

        let tx_signing_config =
            TxSigningConfig::try_from(cli_config.tx_signing_config)
                .context(TxSigningSnafu)?;

        let broker_config = BrokerConfig::from(cli_config.broker_config);

        let auth_config = AuthConfig::initialize(cli_config.auth_config)
            .context(AuthSnafu)?;

        let dapp_deployment =
            read_json_file::<DappDeployment>(cli_config.dapp_deployment_file)?;
        let dapp_address = dapp_deployment.dapp_address;
        let dapp_deploy_block_hash = dapp_deployment.dapp_deploy_block_hash;

        let log_config = LogConfig::initialize(cli_config.log_config);
        let rollups_deployment = read_json_file::<RollupsDeploymentJson>(
            cli_config.rollups_deployment_file,
        )
        .map(RollupsDeployment::from)?;

        let authority_address = rollups_deployment.authority_address;

        Ok(AuthorityClaimerConfig {
            tx_manager_config,
            tx_signing_config,
            tx_manager_priority: Priority::Normal,
            auth_config,
            broker_config,
            log_config,
            authority_address,
            dapp_address,
            dapp_deploy_block_hash,
        })
    }
}

// ------------------------------------------------------------------------------------------------
// TxSigningConfig
// ------------------------------------------------------------------------------------------------

#[derive(Debug, Parser)]
#[command(name = "tx_signing_config")]
pub(crate) struct TxSigningCLIConfig {
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
    type Error = TxSigningConfigError;

    fn try_from(cli: TxSigningCLIConfig) -> Result<Self, Self::Error> {
        let account_index = cli.tx_signing_mnemonic_account_index;
        if let Some(mnemonic) = cli.tx_signing_mnemonic {
            Ok(TxSigningConfig::Mnemonic {
                mnemonic,
                account_index,
            })
        } else if let Some(path) = cli.tx_signing_mnemonic_file {
            let mnemonic = fs::read_to_string(path.clone())
                .context(MnemonicFileSnafu { path })?
                .trim()
                .to_string();
            Ok(TxSigningConfig::Mnemonic {
                mnemonic,
                account_index,
            })
        } else {
            match (cli.tx_signing_aws_kms_key_id, cli.tx_signing_aws_kms_region)
            {
                (None, _) => Err(TxSigningConfigError::MissingConfiguration),
                (Some(_), None) => Err(TxSigningConfigError::MissingRegion),
                (Some(key_id), Some(region)) => {
                    let region = Region::from_str(&region)
                        .context(InvalidRegionSnafu)?;
                    Ok(TxSigningConfig::Aws { key_id, region })
                }
            }
        }
    }
}
