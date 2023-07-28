// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

pub mod versioning {
    tonic::include_proto!("versioning");
}

pub mod cartesi_machine {
    tonic::include_proto!("cartesi_machine");
}

pub mod cartesi_server_manager {
    tonic::include_proto!("cartesi_server_manager");
}

pub mod state_fold_server {
    tonic::include_proto!("state_fold_server");
}

pub mod conversions;
