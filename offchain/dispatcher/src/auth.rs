// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//! This module handles the authentication configuration used by the transaction manager.
//!
//! It supports local authentication (given a mnemonic) and AWS KMS authentication.

use clap::Parser;
use rusoto_core::{region::ParseRegionError, Region};
use snafu::{ResultExt, Snafu};
use std::{fs, str::FromStr};

#[derive(Debug, Snafu)]
pub enum AuthError {
    #[snafu(display("Configuration missing mnemonic/key-id"))]
    MissingConfiguration,

    #[snafu(display(
        "Could not read mnemonic file at path `{}`: {}",
        path,
        source
    ))]
    MnemonicFileError {
        path: String,
        source: std::io::Error,
    },

    #[snafu(display("Missing AWS region"))]
    MissingRegion,

    #[snafu(display("Invalid AWS region"))]
    InvalidRegion { source: ParseRegionError },
}

#[derive(Debug, Clone, Parser)]
#[command(name = "auth_config")]
#[command(about = "Configuration for signing authentication")]
pub struct AuthEnvCLIConfig {
    /// Signer mnemonic, overrides `auth_mnemonic_file` and `auth_aws_kms_*`
    #[arg(long, env)]
    pub auth_mnemonic: Option<String>,

    /// Signer mnemonic file path, overrides `auth_aws_kms_*`
    #[arg(long, env)]
    pub auth_mnemonic_file: Option<String>,

    /// Mnemonic account index
    #[arg(long, env)]
    pub auth_mnemonic_account_index: Option<u32>,

    /// AWS KMS signer key-id
    #[arg(long, env)]
    pub auth_aws_kms_key_id: Option<String>,

    /// AWS KMS signer region
    #[arg(long, env)]
    pub auth_aws_kms_region: Option<String>,
}

#[derive(Debug, Clone)]
pub enum AuthConfig {
    Mnemonic {
        mnemonic: String,
        account_index: Option<u32>,
    },

    Aws {
        key_id: String,
        region: Region,
    },
}

impl AuthConfig {
    pub fn initialize(cli: AuthEnvCLIConfig) -> Result<AuthConfig, AuthError> {
        let account_index = cli.auth_mnemonic_account_index;
        if let Some(mnemonic) = cli.auth_mnemonic {
            Ok(AuthConfig::Mnemonic {
                mnemonic,
                account_index,
            })
        } else if let Some(path) = cli.auth_mnemonic_file {
            let mnemonic = fs::read_to_string(path.clone())
                .context(MnemonicFileSnafu { path })?
                .trim()
                .to_string();
            Ok(AuthConfig::Mnemonic {
                mnemonic,
                account_index,
            })
        } else {
            match (cli.auth_aws_kms_key_id, cli.auth_aws_kms_region) {
                (None, _) => Err(AuthError::MissingConfiguration),
                (Some(_), None) => Err(AuthError::MissingRegion),
                (Some(key_id), Some(region)) => {
                    let region = Region::from_str(&region)
                        .context(InvalidRegionSnafu)?;
                    Ok(AuthConfig::Aws { key_id, region })
                }
            }
        }
    }
}
