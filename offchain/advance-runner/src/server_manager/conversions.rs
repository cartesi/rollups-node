// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//! This module contains functions to convert from gRPC types to
//! rollups-events types
use grpc_interfaces::cartesi_machine::Hash;
use grpc_interfaces::cartesi_server_manager::{
    Address, OutputEnum, OutputValidityProof, Proof,
};
use rollups_events::{
    Address as RollupsAddress, Hash as RollupsHash, Payload, RollupsOutputEnum,
    RollupsOutputValidityProof, RollupsProof, ADDRESS_SIZE, HASH_SIZE,
};

use super::error::ServerManagerError;

/// Try to get the field from an option, otherwise return an error
macro_rules! get_field {
    ($field: expr) => {
        match $field {
            Some(value) => value,
            None => {
                return Err(
                    super::error::ServerManagerError::MissingFieldError {
                        name: stringify!($field).to_owned(),
                    },
                );
            }
        }
    };
}

// Export the get_field macro for other modules to use
pub(super) use get_field;

/// Convert gRPC hash to broker equivalent
pub fn convert_hash(hash: Hash) -> Result<RollupsHash, ServerManagerError> {
    hash.data.try_into().map(RollupsHash::new).map_err(|data| {
        ServerManagerError::WrongArraySizeError {
            name: "hash".to_owned(),
            expected: HASH_SIZE,
            got: data.len(),
        }
    })
}

/// Convert gRPC address to broker equivalent
pub fn convert_address(
    address: Address,
) -> Result<RollupsAddress, ServerManagerError> {
    address
        .data
        .try_into()
        .map(RollupsAddress::new)
        .map_err(|data| ServerManagerError::WrongArraySizeError {
            name: "address".to_owned(),
            expected: ADDRESS_SIZE,
            got: data.len(),
        })
}

/// Convert from gRPC proof to broker equivalent
pub fn convert_proof(proof: Proof) -> Result<RollupsProof, ServerManagerError> {
    let output_enum = match proof.output_enum() {
        OutputEnum::Voucher => RollupsOutputEnum::Voucher,
        OutputEnum::Notice => RollupsOutputEnum::Notice,
    };
    let validity = convert_validity(get_field!(proof.validity))?;
    let context = Payload::new(proof.context);
    Ok(RollupsProof {
        input_index: proof.input_index,
        output_index: proof.output_index,
        output_enum,
        validity,
        context,
    })
}

/// Convert from gRPC output validity proof to broker equivalent
fn convert_validity(
    validity: OutputValidityProof,
) -> Result<RollupsOutputValidityProof, ServerManagerError> {
    let output_hashes_root_hash =
        convert_hash(get_field!(validity.output_hashes_root_hash))?;
    let vouchers_epoch_root_hash =
        convert_hash(get_field!(validity.vouchers_epoch_root_hash))?;
    let notices_epoch_root_hash =
        convert_hash(get_field!(validity.notices_epoch_root_hash))?;
    let machine_state_hash =
        convert_hash(get_field!(validity.machine_state_hash))?;
    let output_hash_in_output_hashes_siblings = validity
        .output_hash_in_output_hashes_siblings
        .into_iter()
        .map(convert_hash)
        .collect::<Result<Vec<RollupsHash>, ServerManagerError>>()?;
    let output_hashes_in_epoch_siblings = validity
        .output_hashes_in_epoch_siblings
        .into_iter()
        .map(convert_hash)
        .collect::<Result<Vec<RollupsHash>, ServerManagerError>>()?;
    Ok(RollupsOutputValidityProof {
        input_index_within_epoch: validity.input_index_within_epoch,
        output_index_within_input: validity.output_index_within_input,
        output_hashes_root_hash,
        vouchers_epoch_root_hash,
        notices_epoch_root_hash,
        machine_state_hash,
        output_hash_in_output_hashes_siblings,
        output_hashes_in_epoch_siblings,
    })
}
