// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::{command, Parser};
use rollups_events::{Address, Hash};
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

    /// DApp deploy block hash
    #[arg(long, env)]
    pub dapp_deploy_block_hash: Option<String>,

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
    #[arg(long, env, default_value = "./dapp_deployment.json")]
    pub dapp_deployment_file: PathBuf,

    /// Path to file with deployment json of the rollups
    #[arg(long, env, default_value = "./rollups_deployment.json")]
    pub rollups_deployment_file: PathBuf,
}

#[derive(Clone, Debug)]
pub struct BlockchainConfig {
    pub dapp_address: Address,
    pub dapp_deploy_block_hash: Hash,
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

fn env_or_file<T: DeserializeOwned>(
    env_value: Option<String>,
    file_value: Option<T>,
    name: &str,
) -> Result<T, BlockchainConfigError> {
    if let Some(s) = env_value {
        deserialize(s)
    } else if let Some(value) = file_value {
        Ok(value)
    } else {
        Err(BlockchainConfigError::MissingConfig {
            name: name.to_string(),
        })
    }
}

impl TryFrom<BlockchainCLIConfig> for BlockchainConfig {
    type Error = BlockchainConfigError;

    fn try_from(cli: BlockchainCLIConfig) -> Result<Self, Self::Error> {
        // read the files
        let dapp: Contract = read(cli.dapp_deployment_file)?;
        let rollups: RollupsDeployment = read(cli.rollups_deployment_file)?;

        // try to get the values from the environment values
        // default to the files
        let dapp_address =
            env_or_file(cli.dapp_address, dapp.address, "dapp_address")?;
        let dapp_deploy_block_hash = env_or_file(
            cli.dapp_deploy_block_hash,
            dapp.block_hash,
            "dapp_deploy_block_hash",
        )?;
        let history_address = env_or_file(
            cli.history_address,
            rollups.contracts.history.address,
            "history_address",
        )?;
        let authority_address = env_or_file(
            cli.authority_address,
            rollups.contracts.authority.address,
            "authority_address",
        )?;
        let input_box_address = env_or_file(
            cli.input_box_address,
            rollups.contracts.input_box.address,
            "input_box_address",
        )?;

        Ok(BlockchainConfig {
            dapp_address,
            dapp_deploy_block_hash,
            history_address,
            authority_address,
            input_box_address,
        })
    }
}

#[derive(Clone, Debug, Deserialize)]
struct Contract {
    #[serde(rename = "address")]
    address: Option<Address>,

    #[serde(rename = "blockHash")]
    block_hash: Option<Hash>,
}

#[derive(Clone, Debug, Deserialize)]
struct RollupsContracts {
    #[serde(rename = "History")]
    history: Contract,

    #[serde(rename = "Authority")]
    authority: Contract,

    #[serde(rename = "InputBox")]
    input_box: Contract,
}

#[derive(Clone, Debug, Deserialize)]
struct RollupsDeployment {
    contracts: RollupsContracts,
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

        assert_eq!(deployment.contracts.history.address, history_address);
        assert_eq!(deployment.contracts.authority.address, authority_address);
        assert_eq!(deployment.contracts.input_box.address, input_box_address);
    }
}
