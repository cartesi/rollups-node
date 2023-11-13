// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use eth_tx_manager::config::Error as TxManagerConfigError;
use rusoto_core::region::ParseRegionError;
use snafu::Snafu;
use types::blockchain_config::BlockchainConfigError;

#[derive(Debug, Snafu)]
#[snafu(visibility(pub(crate)))]
pub enum AuthorityClaimerConfigError {
    #[snafu(display("TxManager configuration error"))]
    TxManagerError { source: TxManagerConfigError },

    #[snafu(display("TxSigning configuration error"))]
    TxSigningError { source: TxSigningConfigError },

    #[snafu(display("Blockchain configuration error"))]
    BlockchainError { source: BlockchainConfigError },
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
