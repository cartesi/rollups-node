// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use rollups_events::{
    Broker, BrokerConfig, BrokerError, DAppMetadata, RollupsClaim,
    RollupsClaimsStream, INITIAL_ID,
};
use snafu::ResultExt;
use std::fmt::Debug;

/// The `BrokerListener` listens for new claims from the broker
#[async_trait]
pub trait BrokerListener: Debug {
    type Error: snafu::Error + 'static;

    /// Listen to claims
    async fn listen(&mut self) -> Result<RollupsClaim, Self::Error>;
}

// ------------------------------------------------------------------------------------------------
// DefaultBrokerListener
// ------------------------------------------------------------------------------------------------

#[derive(Debug)]
pub struct DefaultBrokerListener {
    broker: Broker,
    stream: RollupsClaimsStream,
    last_claim_id: String,
}

#[derive(Debug, snafu::Snafu)]
pub enum BrokerListenerError {
    #[snafu(display("broker error"))]
    BrokerError { source: BrokerError },
}

impl DefaultBrokerListener {
    pub async fn new(
        broker_config: BrokerConfig,
        dapp_metadata: DAppMetadata,
    ) -> Result<Self, BrokerError> {
        tracing::trace!("Connecting to the broker ({:?})", broker_config);
        let broker = Broker::new(broker_config).await?;
        let stream = RollupsClaimsStream::new(&dapp_metadata);
        let last_claim_id = INITIAL_ID.to_string();
        Ok(Self {
            broker,
            stream,
            last_claim_id,
        })
    }
}

#[async_trait]
impl BrokerListener for DefaultBrokerListener {
    type Error = BrokerListenerError;

    async fn listen(&mut self) -> Result<RollupsClaim, Self::Error> {
        tracing::trace!("Waiting for claim with id {}", self.last_claim_id);
        let event = self
            .broker
            .consume_blocking(&self.stream, &self.last_claim_id)
            .await
            .context(BrokerSnafu)?;

        self.last_claim_id = event.id;

        Ok(event.payload)
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;
    use testcontainers::clients::Cli;

    use test_fixtures::BrokerFixture;

    use crate::listener::{BrokerListener, DefaultBrokerListener};

    use backoff::ExponentialBackoffBuilder;
    use rollups_events::{
        BrokerConfig, BrokerEndpoint, BrokerError, DAppMetadata, RedactedUrl,
        RollupsClaim, Url,
    };
    use snafu::Snafu;

    // ------------------------------------------------------------------------------------------------
    // Broker Mock
    // ------------------------------------------------------------------------------------------------

    #[derive(Clone, Debug, Snafu)]
    pub enum MockError {
        EndError,
        InternalError,
        MockError,
    }

    pub async fn setup_broker(
        docker: &Cli,
        should_fail: bool,
    ) -> Result<(BrokerFixture, DefaultBrokerListener), BrokerError> {
        let fixture = BrokerFixture::setup(docker).await;

        let redis_endpoint = if should_fail {
            BrokerEndpoint::Single(RedactedUrl::new(
                Url::parse("https://invalid.com").unwrap(),
            ))
        } else {
            fixture.redis_endpoint().clone()
        };

        let config = BrokerConfig {
            redis_endpoint,
            consume_timeout: 300000,
            backoff: ExponentialBackoffBuilder::new()
                .with_initial_interval(Duration::from_millis(1000))
                .with_max_elapsed_time(Some(Duration::from_millis(3000)))
                .build(),
        };
        let metadata = DAppMetadata {
            chain_id: fixture.chain_id(),
            dapp_address: fixture.dapp_address().clone(),
        };
        let broker = DefaultBrokerListener::new(config, metadata).await?;
        Ok((fixture, broker))
    }

    pub async fn produce_rollups_claims(
        fixture: &BrokerFixture<'_>,
        n: usize,
        epoch_index_start: usize,
    ) -> Vec<RollupsClaim> {
        let mut rollups_claims = Vec::new();
        for i in 0..n {
            let mut rollups_claim = RollupsClaim::default();
            rollups_claim.epoch_index = (i + epoch_index_start) as u64;
            fixture.produce_rollups_claim(rollups_claim.clone()).await;
            rollups_claims.push(rollups_claim);
        }
        rollups_claims
    }

    /// The last claim should trigger an `EndError` error.
    pub async fn produce_last_claim(
        fixture: &BrokerFixture<'_>,
        epoch_index: usize,
    ) -> Vec<RollupsClaim> {
        produce_rollups_claims(fixture, 1, epoch_index).await
    }

    // ------------------------------------------------------------------------------------------------
    // Listener Unit Tests
    // ------------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn instantiate_new_broker_listener_ok() {
        let docker = Cli::default();
        let _ = setup_broker(&docker, false).await;
    }

    #[tokio::test]
    async fn instantiate_new_broker_listener_error() {
        let docker = Cli::default();
        let result = setup_broker(&docker, true).await;
        assert!(result.is_err(), "setup_broker didn't fail as it should");
        let error = result.err().unwrap().to_string();
        assert_eq!(error, "error connecting to Redis");
    }

    #[tokio::test]
    async fn start_broker_listener_with_one_claim_enqueued() {
        let docker = Cli::default();
        let (fixture, mut broker_listener) =
            setup_broker(&docker, false).await.unwrap();
        let n = 5;
        produce_rollups_claims(&fixture, n, 0).await;
        produce_last_claim(&fixture, n).await;
        let result = broker_listener.listen().await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn start_broker_listener_with_claims_enqueued() {
        let docker = Cli::default();
        let (fixture, mut broker_listener) =
            setup_broker(&docker, false).await.unwrap();
        produce_last_claim(&fixture, 0).await;
        let claim = broker_listener.listen().await;
        assert!(claim.is_ok());
    }

    #[tokio::test]
    async fn start_broker_listener_listener_with_no_claims_enqueued() {
        let docker = Cli::default();
        let (fixture, mut broker_listener) =
            setup_broker(&docker, false).await.unwrap();
        let n = 7;

        let broker_listener_thread = tokio::spawn(async move {
            println!("Spawned the broker-listener thread.");
            let claim = broker_listener.listen().await;
            assert!(claim.is_ok());
        });

        println!("Going to sleep for 1 second.");
        tokio::time::sleep(Duration::from_secs(1)).await;

        let x = 2;
        println!("Creating {} claims.", x);
        produce_rollups_claims(&fixture, x, 0).await;

        println!("Going to sleep for 2 seconds.");
        tokio::time::sleep(Duration::from_secs(2)).await;

        let y = 5;
        println!("Creating {} claims.", y);
        produce_rollups_claims(&fixture, y, x).await;

        assert_eq!(x + y, n);
        produce_last_claim(&fixture, n).await;

        broker_listener_thread.await.unwrap();
    }
}
