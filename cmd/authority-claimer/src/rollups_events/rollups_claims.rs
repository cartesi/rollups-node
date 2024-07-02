// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::{Address, Hash};
use serde::{Deserialize, Serialize};

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
