// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//! Convert from rollups-events types to rollups-data types.
//! This code cannot use the From trait because both types are defined in
//! external crates.
use chrono::naive::NaiveDateTime;

use rollups_events::{
    RollupsAdvanceStateInput, RollupsNotice, RollupsOutputEnum, RollupsProof,
    RollupsReport, RollupsVoucher,
};

use rollups_data::{Input, Notice, OutputEnum, Proof, Report, Voucher};

pub fn convert_input(input: RollupsAdvanceStateInput) -> Input {
    let timestamp = match NaiveDateTime::from_timestamp_millis(
        input.metadata.timestamp as i64,
    ) {
        Some(timestamp) => timestamp,
        None => {
            tracing::warn!(
                input.metadata.timestamp,
                "failed to parse timestamp"
            );
            Default::default()
        }
    };
    Input {
        index: input.metadata.input_index as i32,
        msg_sender: input.metadata.msg_sender.into_inner().into(),
        tx_hash: input.tx_hash.into_inner().into(),
        block_number: input.metadata.block_number as i64,
        timestamp,
        payload: input.payload.into_inner(),
    }
}

pub fn convert_voucher(voucher: RollupsVoucher) -> Voucher {
    Voucher {
        input_index: voucher.input_index as i32,
        index: voucher.index as i32,
        destination: voucher.destination.into_inner().into(),
        payload: voucher.payload.into_inner(),
    }
}

pub fn convert_notice(notice: RollupsNotice) -> Notice {
    Notice {
        input_index: notice.input_index as i32,
        index: notice.index as i32,
        payload: notice.payload.into_inner(),
    }
}

pub fn convert_report(report: RollupsReport) -> Report {
    Report {
        input_index: report.input_index as i32,
        index: report.index as i32,
        payload: report.payload.into_inner(),
    }
}

pub fn convert_proof(proof: RollupsProof) -> Proof {
    Proof {
        input_index: proof.input_index as i32,
        output_index: proof.output_index as i32,
        output_enum: match proof.output_enum {
            RollupsOutputEnum::Voucher => OutputEnum::Voucher,
            RollupsOutputEnum::Notice => OutputEnum::Notice,
        },
        validity_input_index_within_epoch: proof
            .validity
            .input_index_within_epoch
            as i32,
        validity_output_index_within_input: proof
            .validity
            .output_index_within_input
            as i32,
        validity_output_hashes_root_hash: proof
            .validity
            .output_hashes_root_hash
            .into_inner()
            .into(),
        validity_vouchers_epoch_root_hash: proof
            .validity
            .vouchers_epoch_root_hash
            .into_inner()
            .into(),
        validity_notices_epoch_root_hash: proof
            .validity
            .notices_epoch_root_hash
            .into_inner()
            .into(),
        validity_machine_state_hash: proof
            .validity
            .machine_state_hash
            .into_inner()
            .into(),
        validity_output_hash_in_output_hashes_siblings: proof
            .validity
            .output_hash_in_output_hashes_siblings
            .into_iter()
            .map(|hash| Some(hash.into_inner().into()))
            .collect(),
        validity_output_hashes_in_epoch_siblings: proof
            .validity
            .output_hashes_in_epoch_siblings
            .into_iter()
            .map(|hash| Some(hash.into_inner().into()))
            .collect(),
        context: proof.context.into_inner(),
    }
}
