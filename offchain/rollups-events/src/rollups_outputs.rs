// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//! For more information about each type, see the GraphQL API definition in
//! `offchain/graphql-server/schema.graphql`
use serde::{Deserialize, Serialize};

use crate::{rollups_stream::decl_broker_stream, Address, Hash, Payload};

decl_broker_stream!(RollupsOutputsStream, RollupsOutput, "rollups-outputs");

/// Cartesi  output
#[derive(Debug, Clone, Eq, PartialEq, Serialize, Deserialize)]
pub enum RollupsOutput {
    Voucher(RollupsVoucher),
    Notice(RollupsNotice),
    Report(RollupsReport),
    Proof(RollupsProof),
}

#[derive(Debug, Clone, Default, Eq, PartialEq, Serialize, Deserialize)]
pub struct RollupsVoucher {
    pub index: u64,
    pub input_index: u64,
    pub destination: Address,
    pub payload: Payload,
}

#[derive(Debug, Clone, Default, Eq, PartialEq, Serialize, Deserialize)]
pub struct RollupsNotice {
    pub index: u64,
    pub input_index: u64,
    pub payload: Payload,
}

#[derive(Debug, Clone, Default, Eq, PartialEq, Serialize, Deserialize)]
pub struct RollupsReport {
    pub index: u64,
    pub input_index: u64,
    pub payload: Payload,
}

#[derive(Debug, Clone, Default, Eq, PartialEq, Serialize, Deserialize)]
pub enum RollupsOutputEnum {
    #[default]
    Voucher,
    Notice,
}

#[derive(Debug, Clone, Default, Eq, PartialEq, Serialize, Deserialize)]
pub struct RollupsProof {
    pub input_index: u64,
    pub output_index: u64,
    pub output_enum: RollupsOutputEnum,
    pub validity: RollupsOutputValidityProof,
    pub context: Payload,
}

#[derive(Debug, Clone, Default, Eq, PartialEq, Serialize, Deserialize)]
pub struct RollupsOutputValidityProof {
    pub input_index_within_epoch: u64,
    pub output_index_within_input: u64,
    pub output_hashes_root_hash: Hash,
    pub vouchers_epoch_root_hash: Hash,
    pub notices_epoch_root_hash: Hash,
    pub machine_state_hash: Hash,
    pub output_hash_in_output_hashes_siblings: Vec<Hash>,
    pub output_hashes_in_epoch_siblings: Vec<Hash>,
}
