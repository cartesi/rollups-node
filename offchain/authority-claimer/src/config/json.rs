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

pub(crate) fn read_json_file<T: DeserializeOwned>(
    path: PathBuf,
) -> Result<T, AuthorityClaimerConfigError> {
    let file =
        File::open(&path).context(ReadFileSnafu { path: path.clone() })?;
    let reader = BufReader::new(file);
    serde_json::from_reader(reader).context(JsonParseSnafu { path })
}
