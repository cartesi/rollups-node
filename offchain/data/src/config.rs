// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use backoff::{ExponentialBackoff, ExponentialBackoffBuilder};
use clap::Parser;
pub use redacted::Redacted;
use std::time::Duration;

#[derive(Debug)]
pub struct RepositoryConfig {
    pub user: String,
    pub password: Redacted<String>,
    pub hostname: String,
    pub port: u16,
    pub db: String,
    pub connection_pool_size: u32,
    pub backoff: ExponentialBackoff,
}

impl RepositoryConfig {
    pub fn endpoint(&self) -> Redacted<String> {
        Redacted::new(format!(
            "postgres://{}:{}@{}:{}/{}",
            urlencoding::encode(&self.user),
            urlencoding::encode(self.password.inner()),
            urlencoding::encode(&self.hostname),
            self.port,
            urlencoding::encode(&self.db)
        ))
    }
}

#[derive(Parser)]
pub struct RepositoryCLIConfig {
    #[arg(long, env, default_value = "postgres")]
    postgres_user: String,

    #[arg(long, env)]
    postgres_password: Option<String>,

    #[arg(long, env)]
    postgres_password_file: Option<String>,

    #[arg(long, env, default_value = "127.0.0.1")]
    postgres_hostname: String,

    #[arg(long, env, default_value_t = 5432)]
    postgres_port: u16,

    #[arg(long, env, default_value = "postgres")]
    postgres_db: String,

    #[arg(long, env, default_value_t = 3)]
    postgres_connection_pool_size: u32,

    #[arg(long, env, default_value = "120000")]
    postgres_backoff_max_elapsed_duration: u64,
}

impl From<RepositoryCLIConfig> for RepositoryConfig {
    fn from(cli_config: RepositoryCLIConfig) -> RepositoryConfig {
        let password = if let Some(filename) = cli_config.postgres_password_file
        {
            if cli_config.postgres_password.is_some() {
                panic!("Both `postgres_password` and `postgres_password_file` arguments are set");
            }
            match std::fs::read_to_string(filename) {
                Ok(password) => password,
                Err(e) => {
                    panic!("Failed to read password from file: {:?}", e);
                }
            }
        } else {
            cli_config
                .postgres_password
                .expect("Database Postgres password was not provided")
        };
        let connection_pool_size = cli_config.postgres_connection_pool_size;
        let backoff_max_elapsed_duration = Duration::from_millis(
            cli_config.postgres_backoff_max_elapsed_duration,
        );
        let backoff = ExponentialBackoffBuilder::new()
            .with_max_elapsed_time(Some(backoff_max_elapsed_duration))
            .build();
        RepositoryConfig {
            user: cli_config.postgres_user,
            password: Redacted::new(password),
            hostname: cli_config.postgres_hostname,
            port: cli_config.postgres_port,
            db: cli_config.postgres_db,
            connection_pool_size,
            backoff,
        }
    }
}
