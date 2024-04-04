// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

mod aws_credentials;
mod aws_signer;
mod signer;

pub use signer::{ConditionalSigner, ConditionalSignerError};
