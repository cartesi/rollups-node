// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use rollups_events::{Address, Hash};
use serde::{de::DeserializeOwned, Deserialize};
use snafu::ResultExt;
use std::{fs::File, io::BufReader, path::PathBuf};

use crate::config::error::{
    AuthorityClaimerConfigError, JsonParseSnafu, ReadFileSnafu,
};

#[derive(Clone, Debug, Deserialize)]
pub(crate) struct DappDeployment {
    #[serde(rename = "address")]
    pub dapp_address: Address,

    #[serde(rename = "blockHash")]
    pub dapp_deploy_block_hash: Hash,
}

#[derive(Clone, Debug, Deserialize)]
pub struct RollupsDeploymentJson {
    contracts: RollupsDeployment,
}

#[derive(Clone, Debug, Deserialize)]
pub(crate) struct RollupsDeployment {
    #[serde(rename = "History")]
    pub history_address: Address,

    #[serde(rename = "Authority")]
    pub authority_address: Address,

    #[serde(rename = "InputBox")]
    pub input_box_address: Address,
}

impl From<RollupsDeploymentJson> for RollupsDeployment {
    fn from(r: RollupsDeploymentJson) -> Self {
        let contracts = r.contracts;
        Self {
            history_address: contracts.history_address,
            authority_address: contracts.authority_address,
            input_box_address: contracts.input_box_address,
        }
    }
}

pub(crate) fn read_json_file<T: DeserializeOwned>(
    path: PathBuf,
) -> Result<T, AuthorityClaimerConfigError> {
    let file =
        File::open(&path).context(ReadFileSnafu { path: path.clone() })?;
    let reader = BufReader::new(file);
    serde_json::from_reader(reader).context(JsonParseSnafu { path })
}
