// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;
use eth_block_history::config::{BHConfig, BHEnvCLIConfig};
use eth_state_fold::config::{SFConfig, SFEnvCLIConfig};
use http_server::HttpServerConfig;
use log::{LogConfig, LogEnvCliConfig};
use snafu::{ResultExt, Snafu};
use std::{fs::File, io::BufReader, path::PathBuf};
use url::{ParseError, Url};

use rollups_events::{BrokerCLIConfig, BrokerConfig};
use types::deployment_files::{
    dapp_deployment::DappDeployment,
    rollups_deployment::{RollupsDeployment, RollupsDeploymentJson},
};

#[derive(Clone, Parser)]
#[command(name = "rd_config")]
#[command(about = "Configuration for rollups eth-input-reader")]
pub struct EthInputReaderEnvCLIConfig {
    #[command(flatten)]
    pub broker_config: BrokerCLIConfig,

    #[command(flatten)]
    pub sf_config: SFEnvCLIConfig,

    #[command(flatten)]
    pub bh_config: BHEnvCLIConfig,

    #[command(flatten)]
    pub log_config: LogEnvCliConfig,

    /// Path to file with deployment json of dapp
    #[arg(long, env, default_value = "./dapp_deployment.json")]
    pub rd_dapp_deployment_file: PathBuf,

    /// Path to file with deployment json of rollups
    #[arg(long, env, default_value = "./rollups_deployment.json")]
    pub rd_rollups_deployment_file: PathBuf,

    /// Duration of rollups epoch in seconds, for which eth-input-reader will read
    #[arg(long, env, default_value = "604800")]
    pub rd_epoch_duration: u64,

    /// Chain ID
    #[arg(long, env)]
    pub chain_id: u64,

    /// Depth on the blockchain the reader will be listening to
    #[arg(long, env, default_value = "12")]
    pub subscription_depth: usize,
}

#[derive(Clone, Debug)]
pub struct EthInputReaderConfig {
    pub broker_config: BrokerConfig,
    pub sf_config: SFConfig,
    pub bh_config: BHConfig,
    pub log_config: LogConfig,

    pub dapp_deployment: DappDeployment,
    pub rollups_deployment: RollupsDeployment,
    pub epoch_duration: u64,
    pub chain_id: u64,
    pub subscription_depth: usize,
    pub http_endpoint: Url,
}

#[derive(Debug, Snafu)]
pub enum Error {
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

    #[snafu(display("parser error"))]
    ParseError { source: ParseError },

    #[snafu(display("Rollups json read file error"))]
    RollupsJsonReadFileError { source: std::io::Error },

    #[snafu(display("Rollups json parse error"))]
    RollupsJsonParseError { source: serde_json::Error },

    #[snafu(display("Configuration missing chain_id"))]
    MissingChainId,
}

#[derive(Debug)]
pub struct Config {
    pub eth_input_reader_config: EthInputReaderConfig,
    pub http_server_config: HttpServerConfig,
}

impl Config {
    pub fn initialize() -> Result<Self, Error> {
        let (http_server_config, eth_input_reader_config) =
            HttpServerConfig::parse::<EthInputReaderEnvCLIConfig>(
                "eth_input_reader",
            );

        let sf_config = SFConfig::initialize(eth_input_reader_config.sf_config);

        let bh_config = BHConfig::initialize(eth_input_reader_config.bh_config);

        let http_endpoint =
            Url::parse(&bh_config.http_endpoint).context(ParseSnafu)?;

        let log_config =
            LogConfig::initialize(eth_input_reader_config.log_config);

        let path = eth_input_reader_config.rd_dapp_deployment_file;
        let dapp_deployment: DappDeployment = read_json(path)?;

        let path = eth_input_reader_config.rd_rollups_deployment_file;
        let rollups_deployment = read_json::<RollupsDeploymentJson>(path)
            .map(RollupsDeployment::from)?;

        let broker_config =
            BrokerConfig::from(eth_input_reader_config.broker_config);

        let eth_input_reader_config = EthInputReaderConfig {
            broker_config,
            sf_config,
            bh_config,
            log_config,
            dapp_deployment,
            rollups_deployment,
            epoch_duration: eth_input_reader_config.rd_epoch_duration,
            chain_id: eth_input_reader_config.chain_id,
            subscription_depth: eth_input_reader_config.subscription_depth,
            http_endpoint,
        };

        Ok(Config {
            eth_input_reader_config,
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
