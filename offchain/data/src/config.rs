// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use backoff::{ExponentialBackoff, ExponentialBackoffBuilder};
use clap::Parser;
pub use redacted::{RedactedUrl, Url};
use std::time::Duration;

#[derive(Debug)]
pub struct RepositoryConfig {
    pub redacted_endpoint: Option<RedactedUrl>,
    pub connection_pool_size: u32,
    pub backoff: ExponentialBackoff,
}

impl RepositoryConfig {
    /// Get the string with the endpoint if it is set, otherwise return an empty string
    pub fn endpoint(&self) -> String {
        match &self.redacted_endpoint {
            None => String::from(""),
            Some(endpoint) => endpoint.inner().to_string(),
        }
    }
}

#[derive(Debug, Parser)]
pub struct RepositoryCLIConfig {
    /// Postgres endpoint in the format 'postgres://user:password@hostname:port/database'.
    ///
    /// If not set, or set to empty string, will defer the behaviour to the Pg driver.
    /// See: https://www.postgresql.org/docs/current/libpq-envars.html
    ///
    /// It is also possible to set the endpoint without a password and load it from Postgres'
    /// passfile.
    /// See: https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNECT-PASSFILE
    #[arg(long, env)]
    postgres_endpoint: Option<String>,

    /// Number of connections to the database
    #[arg(long, env, default_value_t = 3)]
    postgres_connection_pool_size: u32,

    /// Max elapsed time for timeout
    #[arg(long, env, default_value = "120000")]
    postgres_backoff_max_elapsed_duration: u64,
}

impl From<RepositoryCLIConfig> for RepositoryConfig {
    fn from(cli_config: RepositoryCLIConfig) -> RepositoryConfig {
        let redacted_endpoint = match cli_config.postgres_endpoint {
            None => None,
            Some(endpoint) => {
                if endpoint.is_empty() {
                    None
                } else {
                    Some(RedactedUrl::new(
                        Url::parse(endpoint.as_str())
                            .expect("failed to parse Postgres URL"),
                    ))
                }
            }
        };
        let connection_pool_size = cli_config.postgres_connection_pool_size;
        let backoff_max_elapsed_duration = Duration::from_millis(
            cli_config.postgres_backoff_max_elapsed_duration,
        );
        let backoff = ExponentialBackoffBuilder::new()
            .with_max_elapsed_time(Some(backoff_max_elapsed_duration))
            .build();
        RepositoryConfig {
            redacted_endpoint,
            connection_pool_size,
            backoff,
        }
    }
}
