// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use eth_tx_manager::config::Error as TxManagerConfigError;
use rusoto_core::region::ParseRegionError;
use snafu::Snafu;

use super::ContractsConfigError;

#[derive(Debug, Snafu)]
#[snafu(visibility(pub(crate)))]
pub enum AuthorityClaimerConfigError {
    #[snafu(display("TxManager configuration error"))]
    TxManager { source: TxManagerConfigError },

    #[snafu(display("TxSigning configuration error"))]
    TxSigning { source: TxSigningConfigError },

    #[snafu(display("Contracts configuration error"))]
    Contracts { source: ContractsConfigError },
}

#[derive(Debug, Snafu)]
#[snafu(visibility(pub(crate)))]
pub enum TxSigningConfigError {
    #[snafu(display("Missing auth configuration"))]
    AuthConfigMissing,

    #[snafu(display("Could not read mnemonic file at path `{}`", path,))]
    MnemonicFileError {
        path: String,
        source: std::io::Error,
    },

    #[snafu(display("Missing AWS region"))]
    MissingRegion,

    #[snafu(display("Invalid AWS region"))]
    InvalidRegion { source: ParseRegionError },
}
