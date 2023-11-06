// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;
use eth_state_client_lib::config::{
    Error as SCError, SCConfig, SCEnvCLIConfig,
};
use http_server::HttpServerConfig;
use log::{LogConfig, LogEnvCliConfig};
use snafu::{ResultExt, Snafu};
use std::{fs::File, io::BufReader, path::PathBuf};

use rollups_events::{BrokerCLIConfig, BrokerConfig};
use types::deployment_files::{
    dapp_deployment::DappDeployment,
    rollups_deployment::{RollupsDeployment, RollupsDeploymentJson},
};

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

    /// Path to file with deployment json of dapp
    #[arg(long, env, default_value = "./dapp_deployment.json")]
    pub rd_dapp_deployment_file: PathBuf,

    /// Path to file with deployment json of rollups
    #[arg(long, env, default_value = "./rollups_deployment.json")]
    pub rd_rollups_deployment_file: PathBuf,

    /// Duration of rollups epoch in seconds, for which dispatcher will make claims.
    #[arg(long, env, default_value = "604800")]
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

    pub dapp_deployment: DappDeployment,
    pub rollups_deployment: RollupsDeployment,
    pub epoch_duration: u64,
    pub chain_id: u64,
}

#[derive(Debug, Snafu)]
pub enum Error {
    #[snafu(display("StateClient configuration error: {}", source))]
    StateClientError { source: SCError },

    #[snafu(display("Json read file error ({})", path.display()))]
    JsonReadFileError {
        path: PathBuf,
        source: std::io::Error,
    },

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

        let log_config = LogConfig::initialize(dispatcher_config.log_config);

        let path = dispatcher_config.rd_dapp_deployment_file;
        let dapp_deployment: DappDeployment = read_json(path)?;

        let path = dispatcher_config.rd_rollups_deployment_file;
        let rollups_deployment = read_json::<RollupsDeploymentJson>(path)
            .map(RollupsDeployment::from)?;

        let broker_config = BrokerConfig::from(dispatcher_config.broker_config);

        let dispatcher_config = DispatcherConfig {
            sc_config,
            broker_config,
            log_config,

            dapp_deployment,
            rollups_deployment,
            epoch_duration: dispatcher_config.rd_epoch_duration,
            chain_id: dispatcher_config.chain_id,
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
