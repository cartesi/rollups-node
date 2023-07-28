// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;
use eth_tx_manager::{
    config::{Error as TxError, TxEnvCLIConfig, TxManagerConfig},
    Priority,
};
use http_server::HttpServerConfig;
use snafu::{ResultExt, Snafu};
use state_fold::state_client_lib::config::{
    Error as SCError, SCConfig, SCEnvCLIConfig,
};
use std::{fs::File, io::BufReader, path::PathBuf};

use rollups_events::{BrokerCLIConfig, BrokerConfig};
use types::deployment_files::{
    dapp_deployment::DappDeployment,
    rollups_deployment::{RollupsDeployment, RollupsDeploymentJson},
};

use crate::auth::{AuthConfig, AuthEnvCLIConfig, AuthError};

#[derive(Clone, Parser)]
#[command(name = "rd_config")]
#[command(about = "Configuration for rollups dispatcher")]
pub struct DispatcherEnvCLIConfig {
    #[command(flatten)]
    pub sc_config: SCEnvCLIConfig,

    #[command(flatten)]
    pub tx_config: TxEnvCLIConfig,

    #[command(flatten)]
    pub broker_config: BrokerCLIConfig,

    #[command(flatten)]
    pub auth_config: AuthEnvCLIConfig,

    /// Path to file with deployment json of dapp
    #[arg(long, env, default_value = "./dapp_deployment.json")]
    pub rd_dapp_deployment_file: PathBuf,

    /// Path to file with deployment json of rollups
    #[arg(long, env, default_value = "./rollups_deployment.json")]
    pub rd_rollups_deployment_file: PathBuf,

    /// Duration of rollups epoch in seconds, for which dispatcher will make claims.
    #[arg(long, env, default_value = "604800")]
    pub rd_epoch_duration: u64,
}

#[derive(Clone, Debug)]
pub struct DispatcherConfig {
    pub sc_config: SCConfig,
    pub tx_config: TxManagerConfig,
    pub broker_config: BrokerConfig,
    pub auth_config: AuthConfig,

    pub dapp_deployment: DappDeployment,
    pub rollups_deployment: RollupsDeployment,
    pub epoch_duration: u64,
    pub priority: Priority,
}

#[derive(Debug, Snafu)]
pub enum Error {
    #[snafu(display("StateClient configuration error: {}", source))]
    StateClientError { source: SCError },

    #[snafu(display("TxManager configuration error: {}", source))]
    TxManagerError { source: TxError },

    #[snafu(display("Json read file error ({})", path.display()))]
    JsonReadFileError {
        path: PathBuf,
        source: std::io::Error,
    },

    #[snafu(display("Auth configuration error: {}", source))]
    AuthError { source: AuthError },

    #[snafu(display("Json parse error ({})", path.display()))]
    JsonParseError {
        path: PathBuf,
        source: serde_json::Error,
    },

    #[snafu(display("Rollups json read file error"))]
    RollupsJsonReadFileError { source: std::io::Error },

    #[snafu(display("Rollups json parse error"))]
    RollupsJsonParseError { source: serde_json::Error },
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

        let tx_config =
            TxManagerConfig::initialize(dispatcher_config.tx_config)
                .context(TxManagerSnafu)?;

        let auth_config = AuthConfig::initialize(dispatcher_config.auth_config)
            .context(AuthSnafu)?;

        let path = dispatcher_config.rd_dapp_deployment_file;
        let dapp_deployment: DappDeployment = read_json(path)?;

        let path = dispatcher_config.rd_rollups_deployment_file;
        let rollups_deployment = read_json::<RollupsDeploymentJson>(path)
            .map(RollupsDeployment::from)?;

        let broker_config = BrokerConfig::from(dispatcher_config.broker_config);

        assert!(
            sc_config.default_confirmations < tx_config.default_confirmations,
            "`state-client confirmations` has to be less than `tx-manager confirmations,`"
        );

        let dispatcher_config = DispatcherConfig {
            sc_config,
            tx_config,
            broker_config,
            auth_config,

            dapp_deployment,
            rollups_deployment,
            epoch_duration: dispatcher_config.rd_epoch_duration,
            priority: Priority::Normal,
        };

        Ok(Config {
            dispatcher_config,
            http_server_config,
        })
    }
}

fn read_json<T>(path: PathBuf) -> Result<T, Error>
where
    T: serde::de::DeserializeOwned,
{
    let file =
        File::open(&path).context(JsonReadFileSnafu { path: path.clone() })?;
    let reader = BufReader::new(file);
    serde_json::from_reader(reader).context(JsonParseSnafu { path })
}
