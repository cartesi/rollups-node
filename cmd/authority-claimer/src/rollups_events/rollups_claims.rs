// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::{Address, BrokerStream, Hash};
use serde::{Deserialize, Serialize};

#[derive(Debug)]
pub struct RollupsClaimsStream {
    key: String,
}

impl BrokerStream for RollupsClaimsStream {
    type Payload = RollupsClaim;

    fn key(&self) -> &str {
        &self.key
    }
}

impl RollupsClaimsStream {
    pub fn new(chain_id: u64) -> Self {
        Self {
            key: format!("{{chain-{}}}:rollups-claims", chain_id),
        }
    }
}

/// Event generated when the Cartesi Rollups epoch finishes
#[derive(Debug, Default, Clone, Eq, PartialEq, Serialize, Deserialize)]
pub struct RollupsClaim {
    // Claim id
    pub id: u64,

    // Last block processed in the claim
    pub last_block: u64,

    // DApp address
    pub dapp_address: Address,

    /// Hash of the output merkle root
    pub output_merkle_root_hash: Hash,
}
