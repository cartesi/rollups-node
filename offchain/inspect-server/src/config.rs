// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

/// Configuration can be provided using command-line options, environment variables or
/// configuration file.
/// Command-line parameters take precedence over environment variables and environment variables
/// take precedence over same parameter from file configuration.
use clap::Parser;
use log::{LogConfig, LogEnvCliConfig};
use serde::Deserialize;
use snafu::{ResultExt, Snafu};

#[derive(Debug, Snafu)]
pub enum ConfigError {
    #[snafu(display("parse configuration file error"))]
    FileError { source: std::io::Error },

    #[snafu(display("parse configuration file error"))]
    ParseError { source: toml::de::Error },

    #[snafu(whatever, display("{message}"))]
    Whatever {
        message: String,
        #[snafu(source(from(Box<dyn std::error::Error>, Some)))]
        source: Option<Box<dyn std::error::Error>>,
    },
}
#[derive(Debug)]
pub struct InspectServerConfig {
    pub log_config: LogConfig,
    pub inspect_server_address: String,
    pub server_manager_address: String,
    pub session_id: String,
    pub queue_size: usize,
    pub healthcheck_port: u16,
}

#[derive(Parser)]
pub struct CLIConfig {
    #[command(flatten)]
    pub log_config: LogEnvCliConfig,

    /// HTTP address for the inspect server
    #[arg(long, env)]
    inspect_server_address: Option<String>,

    /// Server manager gRPC address
    #[arg(long, env)]
    server_manager_address: Option<String>,

    /// Server manager session id
    #[arg(long, env)]
    session_id: Option<String>,

    /// Queue size for concurrent inspect requests
    #[arg(long, env)]
    queue_size: Option<usize>,

    /// Path to the config file
    #[arg(long, env)]
    pub config_path: Option<String>,

    /// Port of health check
    #[arg(
        long,
        env = "INSPECT_SERVER_HEALTHCHECK_PORT",
        default_value_t = 8080
    )]
    pub healthcheck_port: u16,
}

impl From<CLIConfig> for InspectServerConfig {
    fn from(cli_config: CLIConfig) -> Self {
        let file_config: FileConfig = load_config_file(cli_config.config_path)
            .expect("couldn't read config file");

        let inspect_server_address: String = cli_config
            .inspect_server_address
            .or(file_config.inspect_server_address)
            .expect("couldn't retrieve inspect server address");

        let server_manager_address: String = cli_config
            .server_manager_address
            .or(file_config.server_manager_address)
            .expect("couldn't retrieve server manager address");

        let session_id: String = cli_config
            .session_id
            .or(file_config.session_id)
            .expect("couldn't retrieve session id");

        let queue_size: usize = cli_config
            .queue_size
            .or(file_config.queue_size)
            .unwrap_or(100);

        Self {
            log_config: cli_config.log_config.into(),
            inspect_server_address,
            server_manager_address,
            session_id,
            queue_size,
            healthcheck_port: cli_config.healthcheck_port,
        }
    }
}

#[derive(Clone, Debug, Deserialize, Default)]
struct FileConfig {
    inspect_server_address: Option<String>,
    server_manager_address: Option<String>,
    session_id: Option<String>,
    queue_size: Option<usize>,
}

fn load_config_file<T: Default + serde::de::DeserializeOwned>(
    // path to the config file if provided
    config_file: Option<String>,
) -> Result<T, ConfigError> {
    match config_file {
        Some(config) => {
            let s = std::fs::read_to_string(config).context(FileSnafu)?;

            let file_config: T = toml::from_str(&s).context(ParseSnafu)?;

            Ok(file_config)
        }
        None => Ok(T::default()),
    }
}
