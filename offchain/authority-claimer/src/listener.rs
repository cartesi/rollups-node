// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use rollups_events::{
    Address, Broker, BrokerConfig, BrokerError, DAppMetadata, RollupsClaim,
    RollupsClaimsStream, INITIAL_ID,
};
use snafu::ResultExt;
use std::{collections::HashMap, fmt::Debug};

/// The `BrokerListener` listens for new claims from the broker.
#[async_trait]
pub trait BrokerListener: Debug {
    type Error: snafu::Error + 'static;

    async fn listen(&mut self) -> Result<RollupsClaim, Self::Error>;
}

#[derive(Debug, snafu::Snafu)]
pub enum BrokerListenerError {
    #[snafu(display("broker error"))]
    BrokerError { source: BrokerError },
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

impl DefaultBrokerListener {
    pub async fn new(
        broker_config: BrokerConfig,
        chain_id: u64,
        dapp_address: Address,
    ) -> Result<Self, BrokerError> {
        tracing::trace!("Connecting to the broker ({:?})", broker_config);
        let broker = Broker::new(broker_config).await?;
        let dapp_metadata = DAppMetadata {
            chain_id,
            dapp_address,
        };
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

// ------------------------------------------------------------------------------------------------
// MultidappBrokerListener
// ------------------------------------------------------------------------------------------------

#[derive(Debug)]
pub struct MultidappBrokerListener {
    broker: Broker,
    streams: HashMap<RollupsClaimsStream, String>, // stream => last-claim-id
}

impl MultidappBrokerListener {
    pub async fn new(
        broker_config: BrokerConfig,
        dapps: Vec<(u64, Address)>, // Vec<(chain_id, app_address)>
    ) -> Result<Self, BrokerError> {
        tracing::trace!("Connecting to the broker ({:?})", broker_config);
        let broker = Broker::new(broker_config).await?;

        // Converts to the stream to last-consumed-id map.
        let streams: Vec<_> = dapps
            .into_iter()
            .map(|(chain_id, dapp_address)| {
                let dapp_metadata = &DAppMetadata {
                    chain_id,
                    dapp_address,
                };
                let stream = RollupsClaimsStream::new(dapp_metadata);
                let initial_id = INITIAL_ID.to_string();
                (stream, initial_id)
            })
            .collect();
        let streams = HashMap::from_iter(streams);

        Ok(Self { broker, streams })
    }
}

#[async_trait]
impl BrokerListener for MultidappBrokerListener {
    type Error = BrokerListenerError;

    async fn listen(&mut self) -> Result<RollupsClaim, Self::Error> {
        tracing::trace!("Waiting for claim");

        let (stream, event) = self
            .broker
            .consume_blocking_from_multiple_streams(self.streams.clone())
            .await
            .context(BrokerSnafu)?;

        // Updates the last-consumed-id from the stream.
        let replaced = self.streams.insert(stream.clone(), event.id);
        assert!(replaced.is_some());

        Ok(event.payload)
    }
}

// ------------------------------------------------------------------------------------------------
// Tests
// ------------------------------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use std::time::Duration;
    use testcontainers::clients::Cli;

    use test_fixtures::{broker::ClaimerMultidappBrokerFixture, BrokerFixture};

    use backoff::ExponentialBackoffBuilder;
    use rollups_events::{
        Address, BrokerConfig, BrokerEndpoint, BrokerError, RedactedUrl,
        RollupsClaim, Url,
    };

    use crate::listener::BrokerListener;

    use super::{DefaultBrokerListener, MultidappBrokerListener};

    // --------------------------------------------------------------------------------------------
    // Broker Mock
    // --------------------------------------------------------------------------------------------

    fn config(redis_endpoint: BrokerEndpoint) -> BrokerConfig {
        BrokerConfig {
            redis_endpoint,
            consume_timeout: 300000,
            backoff: ExponentialBackoffBuilder::new()
                .with_initial_interval(Duration::from_millis(1000))
                .with_max_elapsed_time(Some(Duration::from_millis(3000)))
                .build(),
        }
    }

    // --------------------------------------------------------------------------------------------
    // DefaultListener Tests
    // --------------------------------------------------------------------------------------------

    async fn setup_default_broker_listener(
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
        let broker = DefaultBrokerListener::new(
            config(redis_endpoint),
            fixture.chain_id(),
            fixture.dapp_address().clone(),
        )
        .await?;
        Ok((fixture, broker))
    }

    async fn default_produce_claims(
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
    async fn default_produce_last_claim(
        fixture: &BrokerFixture<'_>,
        epoch_index: usize,
    ) -> Vec<RollupsClaim> {
        default_produce_claims(fixture, 1, epoch_index).await
    }

    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn instantiate_new_default_broker_listener_ok() {
        let docker = Cli::default();
        let _ = setup_default_broker_listener(&docker, false).await;
    }

    #[tokio::test]
    async fn instantiate_new_default_broker_listener_error() {
        let docker = Cli::default();
        let result = setup_default_broker_listener(&docker, true).await;
        assert!(result.is_err(), "setup didn't fail as it should");
        let error = result.err().unwrap().to_string();
        assert_eq!(error, "error connecting to Redis");
    }

    #[tokio::test]
    async fn start_default_broker_listener_with_one_claim_enqueued() {
        let docker = Cli::default();
        let (fixture, mut broker_listener) =
            setup_default_broker_listener(&docker, false).await.unwrap();
        let n = 5;
        default_produce_claims(&fixture, n, 0).await;
        default_produce_last_claim(&fixture, n).await;
        let result = broker_listener.listen().await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn start_default_broker_listener_with_claims_enqueued() {
        let docker = Cli::default();
        let (fixture, mut broker_listener) =
            setup_default_broker_listener(&docker, false).await.unwrap();
        default_produce_last_claim(&fixture, 0).await;
        let claim = broker_listener.listen().await;
        assert!(claim.is_ok());
    }

    #[tokio::test]
    async fn start_default_broker_listener_listener_with_no_claims_enqueued() {
        let docker = Cli::default();
        let (fixture, mut broker_listener) =
            setup_default_broker_listener(&docker, false).await.unwrap();
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
        default_produce_claims(&fixture, x, 0).await;

        println!("Going to sleep for 2 seconds.");
        tokio::time::sleep(Duration::from_secs(2)).await;

        let y = 5;
        println!("Creating {} claims.", y);
        default_produce_claims(&fixture, y, x).await;

        assert_eq!(x + y, n);
        default_produce_last_claim(&fixture, n).await;

        broker_listener_thread.await.unwrap();
    }

    // --------------------------------------------------------------------------------------------
    // MultidappListener Tests
    // --------------------------------------------------------------------------------------------

    async fn setup_multidapp_listener(
        docker: &Cli,
        should_fail: bool,
    ) -> Result<
        (
            ClaimerMultidappBrokerFixture,
            MultidappBrokerListener,
            Vec<Address>,
        ),
        BrokerError,
    > {
        let chain_id: u64 = 0;
        let dapp_addresses: Vec<Address> = vec![
            [3; 20].into(), //
            [5; 20].into(), //
            [7; 20].into(), //
        ];
        let dapps: Vec<_> = dapp_addresses
            .clone()
            .into_iter()
            .map(|dapp_address| (chain_id, dapp_address))
            .collect();

        let fixture =
            ClaimerMultidappBrokerFixture::setup(docker, dapps.clone()).await;
        let redis_endpoint = if should_fail {
            BrokerEndpoint::Single(RedactedUrl::new(
                Url::parse("https://invalid.com").unwrap(),
            ))
        } else {
            fixture.redis_endpoint().clone()
        };

        let broker =
            MultidappBrokerListener::new(config(redis_endpoint), dapps).await?;
        Ok((fixture, broker, dapp_addresses))
    }

    // For each index in indexes, this function produces a claim
    // with rollups_claim.dapp_address = dapps[index]
    // and rollups_claim.epoch_index = epochs[index].
    // It then increments epochs[index].
    async fn multidapp_produce_claims(
        fixture: &ClaimerMultidappBrokerFixture<'_>,
        epochs: &mut Vec<u64>,
        dapps: &Vec<Address>,
        indexes: &Vec<usize>,
    ) {
        for &index in indexes {
            let epoch = *epochs.get(index).unwrap();

            let mut rollups_claim = RollupsClaim::default();
            rollups_claim.dapp_address = dapps.get(index).unwrap().clone();
            rollups_claim.epoch_index = epoch;
            fixture.produce_rollups_claim(rollups_claim.clone()).await;

            epochs[index] = epoch + 1;
        }
    }

    // Asserts that listener.listen() will return indexes.len() claims,
    // and that for each index in indexes
    // there is an unique claim for which claim.dapp_address = dapps[index].
    async fn assert_listen(
        listener: &mut MultidappBrokerListener,
        dapps: &Vec<Address>,
        indexes: &Vec<usize>,
    ) {
        let mut dapps: Vec<_> = indexes
            .iter()
            .map(|&index| dapps.get(index).unwrap().clone())
            .collect();
        for _ in indexes.clone() {
            println!("Listening...");
            let result = listener.listen().await;
            assert!(result.is_ok());
            let dapp = result.unwrap().dapp_address;

            let index = dapps.iter().position(|expected| *expected == dapp);
            assert!(index.is_some());
            dapps.remove(index.unwrap());
        }
        assert!(dapps.is_empty());
    }

    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn instantiate_multidapp_broker_listener_ok() {
        let docker = Cli::default();
        let _ = setup_multidapp_listener(&docker, false).await;
    }

    #[tokio::test]
    async fn instantiate_multidapp_broker_listener_error() {
        let docker = Cli::default();
        let result = setup_multidapp_listener(&docker, true).await;
        assert!(result.is_err(), "setup didn't fail as it should");
        let error = result.err().unwrap().to_string();
        assert_eq!(error, "error connecting to Redis");
    }

    #[tokio::test]
    async fn multidapp_listen_with_one_claim_enqueued() {
        let docker = Cli::default();
        let (fixture, mut listener, dapps) =
            setup_multidapp_listener(&docker, false).await.unwrap();

        let mut epochs = vec![0; dapps.len()];
        let index = 0;
        multidapp_produce_claims(&fixture, &mut epochs, &dapps, &vec![index])
            .await;

        let result = listener.listen().await;
        assert!(result.is_ok());

        let expected_dapp = dapps.get(index).unwrap().clone();
        let actual_dapp = result.unwrap().dapp_address;
        assert_eq!(expected_dapp, actual_dapp);
    }

    #[tokio::test]
    async fn multidapp_listen_with_multiple_claims_enqueued() {
        let docker = Cli::default();
        let (fixture, mut listener, dapps) =
            setup_multidapp_listener(&docker, false).await.unwrap();

        let mut epochs = vec![0; dapps.len()];
        let indexes = vec![2, 1, 1, 2];
        multidapp_produce_claims(&fixture, &mut epochs, &dapps, &indexes).await;
        assert_listen(&mut listener, &dapps, &indexes).await;
    }

    #[tokio::test]
    async fn multidapp_listen_with_one_claim_for_each_dapp_enqueued() {
        let docker = Cli::default();
        let (fixture, mut listener, dapps) =
            setup_multidapp_listener(&docker, false).await.unwrap();

        let mut epochs = vec![0; dapps.len()];
        let indexes = vec![2, 1, 0];
        multidapp_produce_claims(&fixture, &mut epochs, &dapps, &indexes).await;
        assert_listen(&mut listener, &dapps, &indexes).await;
    }

    #[tokio::test]
    async fn multidapp_listen_with_no_claims_enqueued() {
        let docker = Cli::default();
        let (fixture, mut listener, dapps) =
            setup_multidapp_listener(&docker, false).await.unwrap();

        let mut epochs = vec![0; dapps.len()];
        let first_batch = vec![0, 1, 2, 0];
        let second_batch = vec![2, 1, 0, 0, 2, 1];

        let broker_listener_thread = {
            let _dapps = dapps.clone();
            let _first_batch = first_batch.clone();
            let _second_batch = second_batch.clone();
            tokio::spawn(async move {
                println!("Spawned the broker-listener thread.");
                assert_listen(&mut listener, &_dapps, &_first_batch).await;
                println!("All good with the first batch!");
                assert_listen(&mut listener, &_dapps, &_second_batch).await;
                println!("All good with the second batch!");
            })
        };

        println!("Going to sleep for 1 second.");
        tokio::time::sleep(Duration::from_secs(1)).await;

        println!("Producing the first batch of claims.");
        multidapp_produce_claims(&fixture, &mut epochs, &dapps, &first_batch)
            .await;
        println!("Epochs: {:?}", epochs);

        println!("Going to sleep for 2 seconds.");
        tokio::time::sleep(Duration::from_secs(2)).await;

        println!("Producing the second batch of claims.");
        multidapp_produce_claims(&fixture, &mut epochs, &dapps, &second_batch)
            .await;
        println!("Epochs: {:?}", epochs);

        broker_listener_thread.await.unwrap();
    }
}
