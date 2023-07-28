// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;
use std::time::Duration;

#[derive(Clone, Debug, Parser)]
#[command(name = "bh_config")]
#[command(about = "Configuration for block-history")]
pub struct BHEnvCLIConfig {
    /// URL of websocket endpoint for block history
    #[arg(long, env)]
    pub bh_ws_endpoint: Option<String>,

    /// URL of http endpoint for block history
    #[arg(long, env)]
    pub bh_http_endpoint: Option<String>,

    /// Timeout value (secs) for block subscription
    #[arg(long, env)]
    pub bh_block_timeout: Option<u64>,

    /// How far back can we look into the block history from the most recent
    /// block index
    #[arg(long, env)]
    pub bh_max_depth: Option<usize>,
}

#[derive(Clone, Debug)]
pub struct BHConfig {
    pub ws_endpoint: String,
    pub http_endpoint: String,
    pub block_timeout: Duration,
    pub max_depth: usize,
}

// default values
const DEFAULT_WS_ENDPOINT: &str = "ws://localhost:8545";
const DEFAULT_HTTP_ENDPOINT: &str = "http://localhost:8545";
const DEFAULT_MAX_DEPTH: usize = 1000;
const DEFAULT_TIMEOUT: u64 = 60;

impl BHConfig {
    pub fn initialize_from_args() -> Self {
        let env_cli_config = BHEnvCLIConfig::parse();
        Self::initialize(env_cli_config)
    }

    pub fn initialize(env_cli_config: BHEnvCLIConfig) -> Self {
        let ws_endpoint = env_cli_config
            .bh_ws_endpoint
            .unwrap_or(DEFAULT_WS_ENDPOINT.to_string());

        let http_endpoint = env_cli_config
            .bh_http_endpoint
            .unwrap_or(DEFAULT_HTTP_ENDPOINT.to_string());

        let block_timeout = Duration::from_secs(
            env_cli_config.bh_block_timeout.unwrap_or(DEFAULT_TIMEOUT),
        );

        let max_depth =
            env_cli_config.bh_max_depth.unwrap_or(DEFAULT_MAX_DEPTH);

        BHConfig {
            ws_endpoint,
            http_endpoint,
            block_timeout,
            max_depth,
        }
    }
}
