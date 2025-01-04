// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use serde::{Deserialize, Serialize};

use crate::{rollups_stream::decl_broker_stream, Address, Hash, Payload};

decl_broker_stream!(RollupsInputsStream, RollupsInput, "rollups-inputs");

/// Cartesi Rollups event
#[derive(Debug, Clone, Eq, PartialEq, Serialize, Deserialize)]
pub struct RollupsInput {
    /// Id of the parent of the event
    /// This field must be supplied by the producer of the event.
    /// Notice that the parent might not be the latest event in the stream;
    /// this happens during a reorg.
    /// The parent of the first event should be INITIAL_ID.
    pub parent_id: String,

    /// Epoch index
    pub epoch_index: u64,

    /// Number of sent inputs for all epochs
    pub inputs_sent_count: u64,

    /// Data that depends on the kind of event
    pub data: RollupsData,
}

/// Rollups data enumeration
#[derive(Debug, Clone, Eq, PartialEq, Serialize, Deserialize)]
pub enum RollupsData {
    /// Input that advances the Cartesi Rollups epoch
    AdvanceStateInput(RollupsAdvanceStateInput),

    /// End of a Cartesi Rollups epoch
    FinishEpoch {},
}

/// Input that advances the Cartesi Rollups epoch
#[derive(Default, Debug, Clone, Eq, PartialEq, Serialize, Deserialize)]
pub struct RollupsAdvanceStateInput {
    /// Information sent via the input metadata memory range
    pub metadata: InputMetadata,

    /// Payload of the input
    pub payload: Payload,

    /// Transaction hash
    pub tx_hash: Hash,
}

#[derive(Default, Debug, Clone, Eq, PartialEq, Serialize, Deserialize)]
pub struct InputMetadata {
    /// Address of the message sender
    pub msg_sender: Address,

    /// Block number when the input was posted
    pub block_number: u64,

    /// Timestamp of the block
    pub timestamp: u64,

    /// Epoch index
    pub epoch_index: u64,

    /// Input index in epoch
    pub input_index: u64,
}
