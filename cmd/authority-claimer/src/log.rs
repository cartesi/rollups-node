// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;
use std::fmt::Debug;
use tracing::info;
use tracing_subscriber::filter::{EnvFilter, LevelFilter};

#[derive(Debug, Parser)]
#[command(name = "log_config")]
pub struct LogEnvCliConfig {
    #[arg(long, env, default_value = "false")]
    pub log_enable_timestamp: bool,

    #[arg(long, env, default_value = "false")]
    pub log_enable_color: bool,
}

#[derive(Clone, Debug, Default)]
pub struct LogConfig {
    pub enable_timestamp: bool,
    pub enable_color: bool,
}

impl LogConfig {
    pub fn initialize(env_cli_config: LogEnvCliConfig) -> Self {
        let enable_timestamp = env_cli_config.log_enable_timestamp;

        let enable_color = env_cli_config.log_enable_color;

        LogConfig {
            enable_timestamp,
            enable_color,
        }
    }
}

impl From<LogEnvCliConfig> for LogConfig {
    fn from(cli_config: LogEnvCliConfig) -> LogConfig {
        LogConfig::initialize(cli_config)
    }
}

pub fn configure(config: &LogConfig) {
    let filter = EnvFilter::builder()
        .with_default_directive(LevelFilter::INFO.into())
        .from_env_lossy();

    let subscribe_builder = tracing_subscriber::fmt()
        .compact()
        .with_env_filter(filter)
        .with_ansi(config.enable_color);

    if !config.enable_timestamp {
        subscribe_builder.without_time().init();
    } else {
        subscribe_builder.init();
    }
}

pub fn log_service_start<C: Debug>(config: &C, service: &str) {
    let message = format!("Starting {} with config {:?}", service, config);
    info!(message);
}
