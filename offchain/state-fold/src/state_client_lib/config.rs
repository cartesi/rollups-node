// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;
use snafu::{ResultExt, Snafu};

#[derive(Clone, Debug, Parser)]
#[command(name = "sc_config")]
#[command(about = "Configuration for state-client-lib")]
pub struct SCEnvCLIConfig {
    /// URL of state-fold server grpc
    #[arg(long, env)]
    pub sc_grpc_endpoint: Option<String>,

    /// Default confirmations
    #[arg(long, env)]
    pub sc_default_confirmations: Option<usize>,
}

#[derive(Clone, Debug)]
pub struct SCConfig {
    pub grpc_endpoint: String,
    pub default_confirmations: usize,
}

#[derive(Debug, Snafu)]
pub enum Error {
    #[snafu(display("Configuration missing server manager endpoint"))]
    MissingEndpoint {},
}

pub type Result<T> = std::result::Result<T, Error>;

const DEFAULT_DEFAULT_CONFIRMATIONS: usize = 7;

impl SCConfig {
    pub fn initialize_from_args() -> Result<Self> {
        let env_cli_config = SCEnvCLIConfig::parse();
        Self::initialize(env_cli_config)
    }

    pub fn initialize(env_cli_config: SCEnvCLIConfig) -> Result<Self> {
        let grpc_endpoint = env_cli_config
            .sc_grpc_endpoint
            .ok_or(snafu::NoneError)
            .context(MissingEndpointSnafu)?;

        let default_confirmations = env_cli_config
            .sc_default_confirmations
            .unwrap_or(DEFAULT_DEFAULT_CONFIRMATIONS);

        Ok(SCConfig {
            grpc_endpoint,
            default_confirmations,
        })
    }
}
