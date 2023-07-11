// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use snafu::{ResultExt, Snafu};
use tokio::sync::{self, Mutex};

use rollups_events::{
    Broker, BrokerConfig, BrokerError, DAppMetadata, Event, InputMetadata,
    RollupsAdvanceStateInput, RollupsClaim, RollupsClaimsStream, RollupsData,
    RollupsInput, RollupsInputsStream, INITIAL_ID,
};
use types::foldables::input_box::Input;

use super::{BrokerReceive, BrokerSend, BrokerStatus, RollupStatus};

#[derive(Debug, Snafu)]
pub enum BrokerFacadeError {
    #[snafu(display("error connecting to the broker"))]
    BrokerConnectionError { source: BrokerError },

    #[snafu(display("error peeking at the end of the stream"))]
    PeekInputError { source: BrokerError },

    #[snafu(display("error producing input event"))]
    ProduceInputError { source: BrokerError },

    #[snafu(display("error producing finish-epoch event"))]
    ProduceFinishError { source: BrokerError },

    #[snafu(display("error consuming claim event"))]
    ConsumeClaimError { source: BrokerError },

    #[snafu(whatever, display("{message}"))]
    Whatever {
        message: String,
        #[snafu(source(from(Box<dyn std::error::Error>, Some)))]
        source: Option<Box<dyn std::error::Error>>,
    },
}

#[derive(Debug)]
pub struct BrokerFacade {
    broker: Mutex<Broker>,
    inputs_stream: RollupsInputsStream,
    claims_stream: RollupsClaimsStream,
    last_claim_id: Mutex<String>,
}

struct BrokerStreamStatus {
    id: String,
    epoch_number: u64,
    status: RollupStatus,
}

impl BrokerFacade {
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn new(
        config: BrokerConfig,
        dapp_metadata: DAppMetadata,
    ) -> Result<Self, BrokerFacadeError> {
        tracing::trace!(?config, "connection to the broker");
        Ok(Self {
            broker: Mutex::new(
                Broker::new(config).await.context(BrokerConnectionSnafu)?,
            ),
            inputs_stream: RollupsInputsStream::new(&dapp_metadata),
            claims_stream: RollupsClaimsStream::new(&dapp_metadata),
            last_claim_id: Mutex::new(INITIAL_ID.to_owned()),
        })
    }

    #[tracing::instrument(level = "trace", skip_all)]
    async fn broker_status(
        &self,
        broker: &mut sync::MutexGuard<'_, Broker>,
    ) -> Result<BrokerStreamStatus, BrokerFacadeError> {
        let event = self.peek(broker).await?;
        Ok(event.into())
    }

    #[tracing::instrument(level = "trace", skip_all)]
    async fn peek(
        &self,
        broker: &mut sync::MutexGuard<'_, Broker>,
    ) -> Result<Option<Event<RollupsInput>>, BrokerFacadeError> {
        tracing::trace!("peeking last produced event");
        let response = broker
            .peek_latest(&self.inputs_stream)
            .await
            .context(PeekInputSnafu)?;
        tracing::trace!(?response, "got response");

        Ok(response)
    }

    #[tracing::instrument(level = "trace", skip_all)]
    async fn claim(
        &self,
        id: &str,
    ) -> Result<Option<Event<RollupsClaim>>, BrokerFacadeError> {
        let mut broker = self.broker.lock().await;
        let event = broker
            .consume_nonblocking(&self.claims_stream, id)
            .await
            .context(ConsumeClaimSnafu)?;

        tracing::trace!(?event, "consumed event");

        Ok(event)
    }
}

#[async_trait]
impl BrokerStatus for BrokerFacade {
    #[tracing::instrument(level = "trace", skip_all)]
    async fn status(&self) -> Result<RollupStatus, BrokerFacadeError> {
        tracing::trace!("querying broker status");
        let mut broker = self.broker.lock().await;
        let status = self.broker_status(&mut broker).await?.status;
        tracing::trace!(?status, "returning rollup status");
        Ok(status)
    }
}

macro_rules! input_sanity_check {
    ($event:expr, $input_index:expr) => {
        assert_eq!($event.inputs_sent_count, $input_index + 1);
        assert!(matches!(
            $event.data,
            RollupsData::AdvanceStateInput(RollupsAdvanceStateInput {
                metadata: InputMetadata {
                    epoch_index,
                    ..
                },
                ..
            }) if epoch_index == 0
        ));
        assert!(matches!(
            $event.data,
            RollupsData::AdvanceStateInput(RollupsAdvanceStateInput {
                metadata: InputMetadata {
                    input_index,
                    ..
                },
                ..
            }) if input_index == $input_index
        ));
    };
}

macro_rules! epoch_sanity_check {
    ($event:expr, $inputs_sent_count:expr) => {
        assert_eq!($event.inputs_sent_count, $inputs_sent_count);
        assert!(matches!($event.data, RollupsData::FinishEpoch { .. }));
    };
}

#[async_trait]
impl BrokerSend for BrokerFacade {
    #[tracing::instrument(level = "trace", skip_all)]
    async fn enqueue_input(
        &self,
        input_index: u64,
        input: &Input,
    ) -> Result<(), BrokerFacadeError> {
        tracing::info!(?input_index, ?input, "enqueueing input");

        let mut broker = self.broker.lock().await;
        let status = self.broker_status(&mut broker).await?;

        let event = build_next_input(input, &status);
        tracing::trace!(?event, "producing input event");

        input_sanity_check!(event, input_index);

        let id = broker
            .produce(&self.inputs_stream, event)
            .await
            .context(ProduceInputSnafu)?;
        tracing::trace!(id, "produced event with id");

        Ok(())
    }

    #[tracing::instrument(level = "trace", skip_all)]
    async fn finish_epoch(
        &self,
        inputs_sent_count: u64,
    ) -> Result<(), BrokerFacadeError> {
        tracing::info!(?inputs_sent_count, "finishing epoch");

        let mut broker = self.broker.lock().await;
        let status = self.broker_status(&mut broker).await?;

        let event = build_next_finish_epoch(&status);
        tracing::trace!(?event, "producing finish epoch event");

        epoch_sanity_check!(event, inputs_sent_count);

        let id = broker
            .produce(&self.inputs_stream, event)
            .await
            .context(ProduceFinishSnafu)?;

        tracing::trace!(id, "produce event with id");

        Ok(())
    }
}

#[async_trait]
impl BrokerReceive for BrokerFacade {
    #[tracing::instrument(level = "trace", skip_all)]
    async fn next_claim(
        &self,
    ) -> Result<Option<RollupsClaim>, BrokerFacadeError> {
        let mut last_id = self.last_claim_id.lock().await;
        tracing::trace!(?last_id, "getting next epoch claim");

        match self.claim(&last_id).await? {
            Some(event) => {
                *last_id = event.id.clone();
                Ok(Some(event.payload))
            }
            None => Ok(None),
        }
    }
}

impl From<RollupsInput> for RollupStatus {
    fn from(payload: RollupsInput) -> Self {
        let inputs_sent_count = payload.inputs_sent_count;

        match payload.data {
            RollupsData::AdvanceStateInput { .. } => RollupStatus {
                inputs_sent_count,
                last_event_is_finish_epoch: false,
            },

            RollupsData::FinishEpoch { .. } => RollupStatus {
                inputs_sent_count,
                last_event_is_finish_epoch: true,
            },
        }
    }
}

impl From<Event<RollupsInput>> for BrokerStreamStatus {
    fn from(event: Event<RollupsInput>) -> Self {
        let id = event.id;
        let payload = event.payload;
        let epoch_index = payload.epoch_index;

        match payload.data {
            RollupsData::AdvanceStateInput { .. } => Self {
                id,
                epoch_number: epoch_index,
                status: payload.into(),
            },

            RollupsData::FinishEpoch { .. } => Self {
                id,
                epoch_number: epoch_index + 1,
                status: payload.into(),
            },
        }
    }
}

impl From<Option<Event<RollupsInput>>> for BrokerStreamStatus {
    fn from(event: Option<Event<RollupsInput>>) -> Self {
        match event {
            Some(e) => e.into(),

            None => Self {
                id: INITIAL_ID.to_owned(),
                epoch_number: 0,
                status: RollupStatus::default(),
            },
        }
    }
}

fn build_next_input(
    input: &Input,
    status: &BrokerStreamStatus,
) -> RollupsInput {
    let metadata = InputMetadata {
        msg_sender: input.sender.to_fixed_bytes().into(),
        block_number: input.block_added.number.as_u64(),
        timestamp: input.block_added.timestamp.as_u64(),
        epoch_index: 0,
        input_index: status.status.inputs_sent_count,
    };

    let data = RollupsData::AdvanceStateInput(RollupsAdvanceStateInput {
        metadata,
        payload: input.payload.clone().into(),
        tx_hash: input.tx_hash.0.into(),
    });

    RollupsInput {
        parent_id: status.id.clone(),
        epoch_index: status.epoch_number,
        inputs_sent_count: status.status.inputs_sent_count + 1,
        data,
    }
}

fn build_next_finish_epoch(status: &BrokerStreamStatus) -> RollupsInput {
    RollupsInput {
        parent_id: status.id.clone(),
        epoch_index: status.epoch_number,
        inputs_sent_count: status.status.inputs_sent_count,
        data: RollupsData::FinishEpoch {},
    }
}

#[cfg(test)]
mod broker_facade_tests {
    use std::{sync::Arc, time::Duration};

    use backoff::ExponentialBackoffBuilder;
    use eth_state_fold_types::{
        ethereum_types::{Bloom, H160, H256, U256, U64},
        Block,
    };
    use rollups_events::{
        BrokerConfig, BrokerEndpoint, DAppMetadata, Hash, InputMetadata,
        Payload, RedactedUrl, RollupsAdvanceStateInput, RollupsClaim,
        RollupsData, Url, HASH_SIZE,
    };
    use test_fixtures::broker::BrokerFixture;
    use testcontainers::clients::Cli;
    use types::foldables::input_box::Input;

    use crate::machine::{
        rollups_broker::BrokerFacadeError, BrokerReceive, BrokerSend,
        BrokerStatus,
    };

    use super::BrokerFacade;

    // --------------------------------------------------------------------------------------------
    // new
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn new_ok() {
        let docker = Cli::default();
        let (_fixture, _broker) = setup(&docker).await;
    }

    #[tokio::test]
    async fn new_error() {
        let docker = Cli::default();
        let error = failable_setup(&docker, true)
            .await
            .err()
            .expect("'status' function has not failed")
            .to_string();
        // BrokerFacadeError::BrokerConnectionError
        assert_eq!(error, "error connecting to the broker");
    }

    // --------------------------------------------------------------------------------------------
    // status
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn status_inputs_sent_count_equals_0() {
        let docker = Cli::default();
        let (_fixture, broker) = setup(&docker).await;
        let status = broker.status().await.expect("'status' function failed");
        assert_eq!(status.inputs_sent_count, 0);
        assert!(!status.last_event_is_finish_epoch);
    }

    #[tokio::test]
    async fn status_inputs_sent_count_equals_1() {
        let docker = Cli::default();
        let (fixture, broker) = setup(&docker).await;
        produce_advance_state_inputs(&fixture, 1).await;
        let status = broker.status().await.expect("'status' function failed");
        assert_eq!(status.inputs_sent_count, 1);
        assert!(!status.last_event_is_finish_epoch);
    }

    #[tokio::test]
    async fn status_inputs_sent_count_equals_10() {
        let docker = Cli::default();
        let (fixture, broker) = setup(&docker).await;
        produce_advance_state_inputs(&fixture, 10).await;
        let status = broker.status().await.expect("'status' function failed");
        assert_eq!(status.inputs_sent_count, 10);
        assert!(!status.last_event_is_finish_epoch);
    }

    #[tokio::test]
    async fn status_is_finish_epoch() {
        let docker = Cli::default();
        let (fixture, broker) = setup(&docker).await;
        produce_finish_epoch_input(&fixture).await;
        let status = broker.status().await.expect("'status' function failed");
        assert_eq!(status.inputs_sent_count, 0);
        assert!(status.last_event_is_finish_epoch);
    }

    #[tokio::test]
    async fn status_inputs_with_finish_epoch() {
        let docker = Cli::default();
        let (fixture, broker) = setup(&docker).await;
        produce_advance_state_inputs(&fixture, 5).await;
        produce_finish_epoch_input(&fixture).await;
        let status = broker.status().await.expect("'status' function failed");
        assert_eq!(status.inputs_sent_count, 5);
        assert!(status.last_event_is_finish_epoch);
    }

    // --------------------------------------------------------------------------------------------
    // enqueue_input
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn enqueue_input_ok() {
        let docker = Cli::default();
        let (_fixture, broker) = setup(&docker).await;
        for i in 0..3 {
            assert!(broker
                .enqueue_input(i, &new_enqueue_input())
                .await
                .is_ok());
        }
    }

    #[tokio::test]
    #[should_panic(expected = "left: `1`,\n right: `6`")]
    async fn enqueue_input_assertion_error_1() {
        let docker = Cli::default();
        let (_fixture, broker) = setup(&docker).await;
        let _ = broker.enqueue_input(5, &new_enqueue_input()).await;
    }

    #[tokio::test]
    #[should_panic(expected = "left: `5`,\n right: `6`")]
    async fn enqueue_input_assertion_error_2() {
        let docker = Cli::default();
        let (_fixture, broker) = setup(&docker).await;
        for i in 0..4 {
            assert!(broker
                .enqueue_input(i, &new_enqueue_input())
                .await
                .is_ok());
        }
        let _ = broker.enqueue_input(5, &new_enqueue_input()).await;
    }

    // NOTE: cannot test result error because the dependency is not injectable.

    // --------------------------------------------------------------------------------------------
    // finish_epoch
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn finish_epoch_ok_1() {
        let docker = Cli::default();
        let (_fixture, broker) = setup(&docker).await;
        assert!(broker.finish_epoch(0).await.is_ok());
        // BONUS TEST: testing for a finished epoch with no inputs
        assert!(broker.finish_epoch(0).await.is_ok());
    }

    #[tokio::test]
    async fn finish_epoch_ok_2() {
        let docker = Cli::default();
        let (fixture, broker) = setup(&docker).await;
        let first_epoch_inputs = 3;
        produce_advance_state_inputs(&fixture, first_epoch_inputs).await;
        produce_finish_epoch_input(&fixture).await;
        let second_epoch_inputs = 7;
        produce_advance_state_inputs(&fixture, second_epoch_inputs).await;
        let total_inputs = first_epoch_inputs + second_epoch_inputs;
        assert!(broker.finish_epoch(total_inputs as u64).await.is_ok());
    }

    #[tokio::test]
    #[should_panic(expected = "left: `0`,\n right: `1`")]
    async fn finish_epoch_assertion_error() {
        let docker = Cli::default();
        let (_fixture, broker) = setup(&docker).await;
        let _ = broker.finish_epoch(1).await;
    }

    // NOTE: cannot test result error because the dependency is not injectable.

    // --------------------------------------------------------------------------------------------
    // next_claim
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn next_claim_is_none() {
        let docker = Cli::default();
        let (_fixture, broker) = setup(&docker).await;
        let option = broker
            .next_claim()
            .await
            .expect("'next_claim' function failed");
        assert!(option.is_none());
    }

    #[tokio::test]
    async fn next_claim_is_some() {
        let docker = Cli::default();
        let (fixture, broker) = setup(&docker).await;

        let fixture_rollups_claims = produce_claims(&fixture, 1).await;
        let fixture_rollups_claim = fixture_rollups_claims.first().unwrap();
        let broker_rollups_claim = broker
            .next_claim()
            .await
            .expect("'next_claim' function failed")
            .expect("no claims retrieved");
        assert_eq!(broker_rollups_claim, *fixture_rollups_claim);
    }

    #[tokio::test]
    async fn next_claim_is_some_sequential() {
        let docker = Cli::default();
        let (fixture, broker) = setup(&docker).await;

        let n = 3;
        let rollups_claims = produce_claims(&fixture, n).await;
        for i in 0..n {
            let rollups_claim = broker
                .next_claim()
                .await
                .expect("'next_claim' function failed")
                .expect("no claims retrieved");
            assert_eq!(rollups_claim, rollups_claims[i as usize]);
        }
    }

    #[tokio::test]
    async fn next_claim_is_some_interleaved() {
        let docker = Cli::default();
        let (fixture, broker) = setup(&docker).await;

        for i in 0..5 {
            let fixture_rollups_claim = RollupsClaim {
                epoch_index: i,
                epoch_hash: Hash::new([i as u8; HASH_SIZE]),
                first_index: i as u128,
                last_index: i as u128,
            };
            fixture
                .produce_rollups_claim(fixture_rollups_claim.clone())
                .await;
            let broker_rollups_claim = broker
                .next_claim()
                .await
                .expect("'next_claim' function failed")
                .expect("no claims retrieved");
            assert_eq!(fixture_rollups_claim, broker_rollups_claim);
        }
    }

    // --------------------------------------------------------------------------------------------
    // auxiliary
    // --------------------------------------------------------------------------------------------

    async fn failable_setup(
        docker: &Cli,
        should_fail: bool,
    ) -> Result<(BrokerFixture, BrokerFacade), BrokerFacadeError> {
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
        let broker = BrokerFacade::new(config, metadata).await?;
        Ok((fixture, broker))
    }

    async fn setup(docker: &Cli) -> (BrokerFixture, BrokerFacade) {
        failable_setup(docker, false).await.unwrap()
    }

    fn new_enqueue_input() -> Input {
        Input {
            sender: Arc::new(H160::random()),
            payload: vec![],
            block_added: Arc::new(Block {
                hash: H256::random(),
                number: U64::zero(),
                parent_hash: H256::random(),
                timestamp: U256::zero(),
                logs_bloom: Bloom::default(),
            }),
            dapp: Arc::new(H160::random()),
            tx_hash: Arc::new(H256::random()),
        }
    }

    async fn produce_advance_state_inputs(fixture: &BrokerFixture<'_>, n: u32) {
        for _ in 0..n {
            let _ = fixture
                .produce_input_event(RollupsData::AdvanceStateInput(
                    RollupsAdvanceStateInput {
                        metadata: InputMetadata::default(),
                        payload: Payload::default(),
                        tx_hash: Hash::default(),
                    },
                ))
                .await;
        }
    }

    async fn produce_finish_epoch_input(fixture: &BrokerFixture<'_>) {
        let _ = fixture
            .produce_input_event(RollupsData::FinishEpoch {})
            .await;
    }

    async fn produce_claims(
        fixture: &BrokerFixture<'_>,
        n: u64,
    ) -> Vec<RollupsClaim> {
        let mut rollups_claims = Vec::new();
        for i in 0..n {
            let rollups_claim = RollupsClaim {
                epoch_index: i,
                epoch_hash: Hash::new([i as u8; HASH_SIZE]),
                first_index: i as u128,
                last_index: i as u128,
            };
            fixture.produce_rollups_claim(rollups_claim.clone()).await;
            rollups_claims.push(rollups_claim);
        }
        rollups_claims
    }
}
