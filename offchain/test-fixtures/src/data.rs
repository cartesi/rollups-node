// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use rollups_data::run_migrations;
use testcontainers::{clients::Cli, images::postgres::Postgres, Container};

pub const POSTGRES_DB: &str = "postgres";
pub const POSTGRES_USER: &str = "postgres";
pub const POSTGRES_PASSWORD: &str = "pw";
pub const POSTGRES_HOST: &str = "localhost";

pub struct DataFixture<'d> {
    _node: Container<'d, Postgres>,
    pub user: String,
    pub password: String,
    pub hostname: String,
    pub port: u16,
    pub db: String,
    pub endpoint: String,
}

impl DataFixture<'_> {
    #[tracing::instrument(level = "trace", skip_all)]
    pub fn setup(docker: &Cli) -> DataFixture<'_> {
        tracing::info!("setting up postgres fixture");

        tracing::trace!("starting postgres docker container");

        let image = testcontainers::RunnableImage::from(
            testcontainers::images::postgres::Postgres::default(),
        )
        .with_env_var(("POSTGRES_DB".to_owned(), POSTGRES_DB))
        .with_env_var(("POSTGRES_USER".to_owned(), POSTGRES_USER))
        .with_env_var(("POSTGRES_PASSWORD".to_owned(), POSTGRES_PASSWORD))
        .with_tag("13-alpine");

        let node = docker.run(image);
        let port = node.get_host_port_ipv4(5432);
        let pg_endpoint = format!(
            "postgres://{}:{}@{}:{}/{}",
            POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_HOST, port, POSTGRES_DB
        );

        run_migrations(&pg_endpoint).unwrap();

        DataFixture {
            _node: node,
            user: POSTGRES_USER.to_string(),
            password: POSTGRES_PASSWORD.to_string(),
            hostname: POSTGRES_HOST.to_string(),
            port,
            db: POSTGRES_DB.to_string(),
            endpoint: pg_endpoint,
        }
    }

    pub fn port(&self) -> u16 {
        self.port
    }

    pub fn endpoint(&self) -> &str {
        &self.endpoint
    }
}
