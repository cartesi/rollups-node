use clap::Parser;
use eth_state_server_lib::config::{
    Result, StateServerConfig, StateServerEnvCLIConfig,
};
use log::{LogConfig, LogEnvCliConfig};

#[derive(Parser)]
#[command(name = "state_server_config")]
#[command(about = "Configuration for state-server")]
pub struct EnvCLIConfig {
    #[command(flatten)]
    pub state_server_config: StateServerEnvCLIConfig,

    #[command(flatten)]
    pub log_config: LogEnvCliConfig,
}

#[derive(Debug, Clone)]
pub struct Config {
    pub state_server_config: StateServerConfig,
    pub log_config: LogConfig,
}

impl Config {
    pub fn initialize(env_cli_config: EnvCLIConfig) -> Result<Self> {
        let state_server_config =
            StateServerConfig::initialize(env_cli_config.state_server_config);
        let log_config = LogConfig::initialize(env_cli_config.log_config);

        Ok(Self {
            state_server_config: state_server_config?,
            log_config,
        })
    }

    pub fn initialize_from_args() -> Result<Self> {
        let env_cli_config = EnvCLIConfig::parse();
        Self::initialize(env_cli_config)
    }
}
