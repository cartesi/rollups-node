// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

mod grpc_client;
mod interfaces;

pub mod config;
pub mod error;

pub use grpc_client::GrpcStateFoldClient;
pub use interfaces::BlockServer;
pub use interfaces::StateServer;
