// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use serde::{Deserialize, Serialize};

use crate::{rollups_stream::decl_broker_stream, Address, Hash};

decl_broker_stream!(RollupsClaimsStream, RollupsClaim, "rollups-claims");

/// Event generated when the Cartesi Rollups epoch finishes
#[derive(Debug, Default, Clone, Eq, PartialEq, Serialize, Deserialize)]
pub struct RollupsClaim {
    // DApp address
    pub dapp_address: Address,

    /// Epoch index
    pub epoch_index: u64,

    /// Hash of the Epoch
    pub epoch_hash: Hash,

    /// Index of the first input of the Epoch
    pub first_index: u128,

    /// Index of the last input of the Epoch
    pub last_index: u128,
}
