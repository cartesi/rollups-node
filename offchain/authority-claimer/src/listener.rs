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

    use crate::{
        broker_mock,
        listener::{BrokerListener, DefaultBrokerListener},
    };

    async fn setup(docker: &Cli) -> (BrokerFixture, DefaultBrokerListener) {
        broker_mock::setup_broker(docker, false).await.unwrap()
    }

    #[tokio::test]
    async fn instantiate_new_broker_listener_ok() {
        let docker = Cli::default();
        let _ = setup(&docker).await;
    }

    #[tokio::test]
    async fn instantiate_new_broker_listener_error() {
        let docker = Cli::default();
        let result = broker_mock::setup_broker(&docker, true).await;
        assert!(result.is_err(), "setup_broker didn't fail as it should");
        let error = result.err().unwrap().to_string();
        assert_eq!(error, "error connecting to Redis");
    }

    #[tokio::test]
    async fn start_broker_listener_with_one_claim_enqueued() {
        let docker = Cli::default();
        let (fixture, mut broker_listener) = setup(&docker).await;
        let n = 5;
        broker_mock::produce_rollups_claims(&fixture, n, 0).await;
        broker_mock::produce_last_claim(&fixture, n).await;
        let result = broker_listener.listen().await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn start_broker_listener_with_claims_enqueued() {
        let docker = Cli::default();
        let (fixture, mut broker_listener) = setup(&docker).await;
        broker_mock::produce_last_claim(&fixture, 0).await;
        let claim = broker_listener.listen().await;
        assert!(claim.is_ok());
    }

    #[tokio::test]
    async fn start_broker_listener_listener_with_no_claims_enqueued() {
        let docker = Cli::default();
        let (fixture, mut broker_listener) = setup(&docker).await;
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
        broker_mock::produce_rollups_claims(&fixture, x, 0).await;

        println!("Going to sleep for 2 seconds.");
        tokio::time::sleep(Duration::from_secs(2)).await;

        let y = 5;
        println!("Creating {} claims.", y);
        broker_mock::produce_rollups_claims(&fixture, y, x).await;

        assert_eq!(x + y, n);
        broker_mock::produce_last_claim(&fixture, n).await;

        broker_listener_thread.await.unwrap();
    }
}
