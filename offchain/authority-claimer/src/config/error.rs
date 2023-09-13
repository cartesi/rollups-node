// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use eth_tx_manager::config::Error as TxManagerConfigError;
use rusoto_core::region::ParseRegionError;
use snafu::Snafu;
use std::path::PathBuf;

use crate::auth::AuthError;

#[derive(Debug, Snafu)]
#[snafu(visibility(pub(crate)))]
pub enum AuthorityClaimerConfigError {
    #[snafu(display("TxManager configuration error"))]
    TxManagerError { source: TxManagerConfigError },

    #[snafu(display("TxSigning configuration error"))]
    TxSigningError { source: TxSigningConfigError },

    #[snafu(display("Auth configuration error: {}", source))]
    AuthError { source: AuthError },

    #[snafu(display("Read file error ({})", path.display()))]
    ReadFileError {
        path: PathBuf,
        source: std::io::Error,
    },

    #[snafu(display("Json parse error ({})", path.display()))]
    JsonParseError {
        path: PathBuf,
        source: serde_json::Error,
    },
}

#[derive(Debug, Snafu)]
#[snafu(visibility(pub(crate)))]
pub enum TxSigningConfigError {
    #[snafu(display("Missing auth configuration"))]
    MissingConfiguration,

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
