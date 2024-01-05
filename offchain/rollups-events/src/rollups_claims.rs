// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use serde::{Deserialize, Serialize};

use crate::{Address, BrokerStream, DAppMetadata, Hash};

#[derive(Debug)]
pub struct RollupsClaimsStream {
    key: String,
}

impl BrokerStream for RollupsClaimsStream {
    type Payload = (Address, RollupsClaim);

    fn key(&self) -> &str {
        &self.key
    }
}

impl RollupsClaimsStream {
    pub fn new(metadata: &DAppMetadata) -> Self {
        Self {
            key: format!("{{chain-{}}}:rollups-claim", metadata.chain_id),
        }
    }
}

/// Event generated when the Cartesi Rollups epoch finishes
#[derive(Debug, Default, Clone, Eq, PartialEq, Serialize, Deserialize)]
pub struct RollupsClaim {
    /// Epoch index
    pub epoch_index: u64,

    /// Hash of the Epoch
    pub epoch_hash: Hash,

    /// Index of the first input of the Epoch
    pub first_index: u128,

    /// Index of the last input of the Epoch
    pub last_index: u128,
}
