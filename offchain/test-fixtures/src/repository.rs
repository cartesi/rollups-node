// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use backoff::ExponentialBackoffBuilder;
use rollups_data::{RedactedUrl, Repository, RepositoryConfig, Url};
use std::time::Duration;
use testcontainers::clients::Cli;

use crate::data::DataFixture;

const REPOSITORY_MAX_ELAPSED_TIME: u64 = 10;

/// Fixture that creates a database and connects to it using the
/// rollups_data::Repository struct.
pub struct RepositoryFixture<'d> {
    data: DataFixture<'d>,
    repository: Repository,
}

impl RepositoryFixture<'_> {
    pub fn setup(docker: &Cli) -> RepositoryFixture {
        let data = DataFixture::setup(docker);
        let config = create_repository_config(data.port());
        let repository =
            Repository::new(config).expect("failed to create repository");
        RepositoryFixture { data, repository }
    }

    pub fn config(&self) -> RepositoryConfig {
        create_repository_config(self.data.port())
    }

    pub fn repository(&self) -> &Repository {
        &self.repository
    }

    /// Calls f until it returns Ok or an error different from ItemNotFound.
    /// This function is async to allow other services to run in background.
    pub async fn retry<F, T>(&self, mut f: F) -> T
    where
        F: FnMut(&Repository) -> Result<T, rollups_data::Error>
            + Send
            + 'static,
        T: Send + 'static,
    {
        let backoff = ExponentialBackoffBuilder::new()
            .with_max_elapsed_time(Some(Duration::from_secs(
                REPOSITORY_MAX_ELAPSED_TIME,
            )))
            .build();
        let repository = self.repository.clone();
        tokio::task::spawn_blocking(move || {
            backoff::retry(backoff, || {
                f(&repository).map_err(|e| match &e {
                    rollups_data::Error::ItemNotFound { item_type } => {
                        tracing::info!("{} not found", item_type);
                        backoff::Error::transient(e)
                    }
                    _ => backoff::Error::permanent(e),
                })
            })
            .expect("failed to get input from DB")
        })
        .await
        .expect("failed to wait for task")
    }
}

fn create_repository_config(postgres_port: u16) -> RepositoryConfig {
    use crate::data::*;
    let redacted_endpoint = Some(RedactedUrl::new(
        Url::parse(&format!(
            "postgres://{}:{}@{}:{}/{}",
            POSTGRES_USER,
            POSTGRES_PASSWORD,
            POSTGRES_HOST,
            postgres_port,
            POSTGRES_DB,
        ))
        .expect("failed to generate Postgres endpoint"),
    ));
    RepositoryConfig {
        redacted_endpoint,
        connection_pool_size: 1,
        backoff: Default::default(),
    }
}
