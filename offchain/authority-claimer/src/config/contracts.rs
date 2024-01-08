// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;
use rollups_events::Address;
use serde::de::DeserializeOwned;
use snafu::{ResultExt, Snafu};
use std::{fs::File, io::BufReader, path::PathBuf};
use types::blockchain_config::RollupsDeployment;

#[derive(Clone, Debug)]
pub struct ContractsConfig {
    pub history_address: Address,
    pub authority_address: Address,
}

#[derive(Debug, Parser)]
#[command(name = "blockchain_config")]
pub struct ContractsCLIConfig {
    /// History contract address
    #[arg(long, env)]
    pub history_address: Option<String>,

    /// Authority contract address
    #[arg(long, env)]
    pub authority_address: Option<String>,

    /// Path to file with deployment json of the rollups
    #[arg(long, env)]
    pub rollups_deployment_file: Option<PathBuf>,
}

impl TryFrom<ContractsCLIConfig> for ContractsConfig {
    type Error = ContractsConfigError;

    fn try_from(cli: ContractsCLIConfig) -> Result<Self, Self::Error> {
        // try to get the values from the environment values
        let mut history_address = cli
            .history_address
            .map(deserialize::<Address>)
            .transpose()?;
        let mut authority_address = cli
            .authority_address
            .map(deserialize::<Address>)
            .transpose()?;

        // read file and replace values if they are not set
        if let Some(file) = cli
            .rollups_deployment_file
            .map(read::<RollupsDeployment>)
            .transpose()?
        {
            history_address = history_address.or(file
                .contracts
                .history
                .map(|c| c.address)
                .flatten());
            authority_address = authority_address.or(file
                .contracts
                .authority
                .map(|c| c.address)
                .flatten());
        }

        Ok(ContractsConfig {
            history_address: history_address
                .ok_or(ContractsConfigError::MissingHistoryContractConfig)?,
            authority_address: authority_address
                .ok_or(ContractsConfigError::MissingAuthorityContractConfig)?,
        })
    }
}

#[derive(Debug, Snafu)]
pub enum ContractsConfigError {
    #[snafu(display("Json deserialize error"))]
    JsonDeserializeError { source: serde_json::Error },

    #[snafu(display("Json read error ({})", path.display()))]
    JsonReadError {
        path: PathBuf,
        source: serde_json::Error,
    },

    #[snafu(display("Read file error ({})", path.display()))]
    ReadFileError {
        path: PathBuf,
        source: std::io::Error,
    },

    #[snafu(display("Missing History contract configuration"))]
    MissingHistoryContractConfig,

    #[snafu(display("Missing Authority contract configuration"))]
    MissingAuthorityContractConfig,
}

// ------------------------------------------------------------------------------------------------
// Auxiliary
// ------------------------------------------------------------------------------------------------

fn read<T: DeserializeOwned>(path: PathBuf) -> Result<T, ContractsConfigError> {
    let file =
        File::open(&path).context(ReadFileSnafu { path: path.clone() })?;
    let reader = BufReader::new(file);
    serde_json::from_reader(reader).context(JsonReadSnafu { path })
}

fn deserialize<T: DeserializeOwned>(
    s: String,
) -> Result<T, ContractsConfigError> {
    serde_json::from_value(serde_json::Value::String(s))
        .context(JsonDeserializeSnafu)
}
