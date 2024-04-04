// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::rollups_events::Address;
use clap::{command, Parser};
use serde::{de::DeserializeOwned, Deserialize};
use snafu::{ResultExt, Snafu};
use std::{fs::File, io::BufReader, path::PathBuf};

#[derive(Debug, Snafu)]
pub enum BlockchainConfigError {
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

    #[snafu(display("Missing configuration: ({})", name))]
    MissingConfig { name: String },
}

#[derive(Debug, Parser)]
#[command(name = "blockchain_config")]
pub struct BlockchainCLIConfig {
    /// DApp address
    #[arg(long, env)]
    pub dapp_address: Option<String>,

    /// Input box deployment block number
    #[arg(long, env)]
    pub input_box_deployment_block_number: Option<u64>,

    /// History contract address
    #[arg(long, env)]
    pub history_address: Option<String>,

    /// Authority contract address
    #[arg(long, env)]
    pub authority_address: Option<String>,

    /// Input Box contract address
    #[arg(long, env)]
    pub input_box_address: Option<String>,

    /// Path to a file with the deployment json of the dapp
    #[arg(long, env)]
    pub dapp_deployment_file: Option<PathBuf>,

    /// Path to file with deployment json of the rollups
    #[arg(long, env)]
    pub rollups_deployment_file: Option<PathBuf>,
}

#[derive(Clone, Debug)]
pub struct BlockchainConfig {
    pub dapp_address: Address,
    pub input_box_deployment_block_number: u64,
    pub history_address: Address,
    pub authority_address: Address,
    pub input_box_address: Address,
}

fn deserialize<T: DeserializeOwned>(
    s: String,
) -> Result<T, BlockchainConfigError> {
    serde_json::from_value(serde_json::Value::String(s))
        .context(JsonDeserializeSnafu)
}

macro_rules! check_missing {
    ($var_name: ident) => {
        match $var_name {
            Some(v) => v,
            None => {
                return Err(BlockchainConfigError::MissingConfig {
                    name: stringify!($var_name).to_string(),
                })
            }
        }
    };
}

impl TryFrom<BlockchainCLIConfig> for BlockchainConfig {
    type Error = BlockchainConfigError;

    fn try_from(cli: BlockchainCLIConfig) -> Result<Self, Self::Error> {
        // try to get the values from the environment values
        let mut dapp_address =
            cli.dapp_address.map(deserialize::<Address>).transpose()?;
        let input_box_deployment_block_number =
            cli.input_box_deployment_block_number;
        let mut history_address = cli
            .history_address
            .map(deserialize::<Address>)
            .transpose()?;
        let mut authority_address = cli
            .authority_address
            .map(deserialize::<Address>)
            .transpose()?;
        let mut input_box_address = cli
            .input_box_address
            .map(deserialize::<Address>)
            .transpose()?;

        // read files and replace values if they are not set
        if let Some(file) =
            cli.dapp_deployment_file.map(read::<Contract>).transpose()?
        {
            dapp_address = dapp_address.or(file.address);
        }
        if let Some(file) = cli
            .rollups_deployment_file
            .map(read::<RollupsDeployment>)
            .transpose()?
        {
            history_address = history_address
                .or(file.contracts.history.and_then(|c| c.address));
            authority_address = authority_address
                .or(file.contracts.authority.and_then(|c| c.address));
            input_box_address = input_box_address
                .or(file.contracts.input_box.and_then(|c| c.address));
        }

        Ok(BlockchainConfig {
            dapp_address: check_missing!(dapp_address),
            input_box_deployment_block_number: check_missing!(
                input_box_deployment_block_number
            ),
            history_address: check_missing!(history_address),
            authority_address: check_missing!(authority_address),
            input_box_address: check_missing!(input_box_address),
        })
    }
}

#[derive(Clone, Debug, Deserialize)]
pub struct Contract {
    #[serde(rename = "address")]
    pub address: Option<Address>,

    #[serde(rename = "blockNumber")]
    pub block_number: Option<u64>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct RollupsContracts {
    #[serde(rename = "History")]
    pub history: Option<Contract>,

    #[serde(rename = "Authority")]
    pub authority: Option<Contract>,

    #[serde(rename = "InputBox")]
    pub input_box: Option<Contract>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct RollupsDeployment {
    pub contracts: RollupsContracts,
}

fn read<T: DeserializeOwned>(
    path: PathBuf,
) -> Result<T, BlockchainConfigError> {
    let file =
        File::open(&path).context(ReadFileSnafu { path: path.clone() })?;
    let reader = BufReader::new(file);
    serde_json::from_reader(reader).context(JsonReadSnafu { path })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse() {
        let history_address = deserialize(
            "0xb6Eb78277C8a96Fb3f55BABef25eD0Bc5E5c95Fb".to_string(),
        )
        .unwrap();
        let authority_address = deserialize(
            "0xf3D8ce181a502B54512908a32780eaa9183Ef31a".to_string(),
        )
        .unwrap();
        let input_box_address = deserialize(
            "0x10dc33852b996A4C8A391d6Ed224FD89A3aD1ceE".to_string(),
        )
        .unwrap();

        let data = r#"{
            "contracts": {
                "History": {
                    "address": "0xb6Eb78277C8a96Fb3f55BABef25eD0Bc5E5c95Fb"
                },

                "Authority": {
                    "address": "0xf3D8ce181a502B54512908a32780eaa9183Ef31a"
                },

                "InputBox": {
                    "address": "0x10dc33852b996A4C8A391d6Ed224FD89A3aD1ceE"
                }
            }
        }"#;

        let deployment: RollupsDeployment = serde_json::from_str(data).unwrap();

        assert_eq!(
            deployment.contracts.history.unwrap().address,
            history_address
        );
        assert_eq!(
            deployment.contracts.authority.unwrap().address,
            authority_address
        );
        assert_eq!(
            deployment.contracts.input_box.unwrap().address,
            input_box_address
        );
    }
}
