// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use rollups_events::{
    Broker, BrokerConfig, BrokerError, DAppMetadata, Event, RollupsClaim,
    RollupsClaimsStream, RollupsData, RollupsInput, RollupsInputsStream,
    RollupsOutput, RollupsOutputsStream, INITIAL_ID,
};
use snafu::{ResultExt, Snafu};

#[derive(Debug, Snafu)]
pub enum BrokerFacadeError {
    #[snafu(display("broker internal error"))]
    BrokerInternalError { source: BrokerError },

    #[snafu(display("failed to consume input event"))]
    ConsumeError { source: BrokerError },

    #[snafu(display(
        "failed to find finish epoch input event epoch={}",
        epoch
    ))]
    FindFinishEpochInputError { epoch: u64 },

    #[snafu(display("processed event not found in broker"))]
    ProcessedEventNotFound {},
}

pub type Result<T> = std::result::Result<T, BrokerFacadeError>;

pub struct BrokerFacade {
    client: Broker,
    inputs_stream: RollupsInputsStream,
    outputs_stream: RollupsOutputsStream,
    claims_stream: RollupsClaimsStream,
}

impl BrokerFacade {
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn new(
        config: BrokerConfig,
        dapp_metadata: DAppMetadata,
    ) -> Result<Self> {
        tracing::trace!(?config, "connecting to broker");
        let inputs_stream = RollupsInputsStream::new(&dapp_metadata);
        let outputs_stream = RollupsOutputsStream::new(&dapp_metadata);
        let claims_stream = RollupsClaimsStream::new(&dapp_metadata);
        let client = Broker::new(config).await.context(BrokerInternalSnafu)?;
        Ok(Self {
            client,
            inputs_stream,
            outputs_stream,
            claims_stream,
        })
    }

    /// Search the input event stream for the finish epoch event of the previous epoch
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn find_previous_finish_epoch(
        &mut self,
        mut epoch: u64,
    ) -> Result<String> {
        tracing::trace!(epoch, "getting previous finish epoch");

        if epoch == 0 {
            tracing::trace!("returning initial id for epoch 0");
            return Ok(INITIAL_ID.to_owned());
        } else {
            // This won't underflow because we know the epoch is not 0
            epoch -= 1;
        }

        tracing::trace!(epoch, "searching for finish epoch input event");
        let mut last_id = INITIAL_ID.to_owned();
        loop {
            let event = self
                .client
                .consume_nonblocking(&self.inputs_stream, &last_id)
                .await
                .context(ConsumeSnafu)?
                .ok_or(BrokerFacadeError::FindFinishEpochInputError {
                    epoch,
                })?;
            if matches!(
                event.payload,
                RollupsInput {
                    data: RollupsData::FinishEpoch {},
                    epoch_index,
                    ..
                } if epoch_index == epoch
            ) {
                tracing::trace!(event_id = last_id, "returning event id");
                return Ok(event.id);
            }
            last_id = event.id;
        }
    }

    /// Consume rollups input event
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn consume_input(
        &mut self,
        last_id: &str,
    ) -> Result<Event<RollupsInput>> {
        tracing::trace!(last_id, "consuming rollups input event");

        loop {
            let result = self
                .client
                .consume_blocking(&self.inputs_stream, last_id)
                .await;
            if matches!(result, Err(BrokerError::ConsumeTimeout)) {
                tracing::trace!("consume timed out, retrying");
            } else {
                return result.context(BrokerInternalSnafu);
            }
        }
    }

    /// Produce the rollups claim if it isn't in the stream yet
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn produce_rollups_claim(
        &mut self,
        rollups_claim: RollupsClaim,
    ) -> Result<()> {
        tracing::trace!(rollups_claim.epoch_index,
            ?rollups_claim.epoch_hash,
            "producing rollups claim"
        );

        let result = self
            .client
            .peek_latest(&self.claims_stream)
            .await
            .context(BrokerInternalSnafu)?;

        let claim_produced = match result {
            Some(event) => {
                tracing::trace!(?event, "got last claim produced");
                rollups_claim.epoch_index <= event.payload.epoch_index
            }
            None => {
                tracing::trace!("no claims in the stream");
                false
            }
        };

        if !claim_produced {
            self.client
                .produce(&self.claims_stream, rollups_claim)
                .await
                .context(BrokerInternalSnafu)?;
        }

        Ok(())
    }

    /// Produce outputs to the rollups-outputs stream
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn produce_outputs(
        &mut self,
        outputs: Vec<RollupsOutput>,
    ) -> Result<()> {
        tracing::trace!(?outputs, "producing rollups outputs");

        for output in outputs {
            self.client
                .produce(&self.outputs_stream, output)
                .await
                .context(BrokerInternalSnafu)?;
        }

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use backoff::ExponentialBackoff;
    use rollups_events::{
        DAppMetadata, Hash, InputMetadata, Payload, RollupsAdvanceStateInput,
        HASH_SIZE,
    };
    use test_fixtures::BrokerFixture;
    use testcontainers::clients::Cli;

    struct TestState<'d> {
        fixture: BrokerFixture<'d>,
        facade: BrokerFacade,
    }

    impl TestState<'_> {
        async fn setup(docker: &Cli) -> TestState<'_> {
            let fixture = BrokerFixture::setup(docker).await;
            let backoff = ExponentialBackoff::default();
            let dapp_metadata = DAppMetadata {
                chain_id: fixture.chain_id(),
                dapp_address: fixture.dapp_address().to_owned(),
            };
            let config = BrokerConfig {
                redis_endpoint: fixture.redis_endpoint().to_owned(),
                consume_timeout: 10,
                backoff,
            };
            let facade = BrokerFacade::new(config, dapp_metadata)
                .await
                .expect("failed to create broker facade");
            TestState { fixture, facade }
        }
    }

    #[test_log::test(tokio::test)]
    async fn test_it_finds_previous_finish_of_first_epoch() {
        let docker = Cli::default();
        let mut state = TestState::setup(&docker).await;
        let id = state.facade.find_previous_finish_epoch(0).await.unwrap();
        assert_eq!(id, INITIAL_ID);
    }

    #[test_log::test(tokio::test)]
    async fn test_it_finds_previous_finish_of_nth_epoch() {
        let docker = Cli::default();
        let mut state = TestState::setup(&docker).await;
        let inputs =
            vec![RollupsData::FinishEpoch {}, RollupsData::FinishEpoch {}];
        let mut ids = Vec::new();
        for input in inputs {
            ids.push(state.fixture.produce_input_event(input).await);
        }
        assert_eq!(
            state.facade.find_previous_finish_epoch(1).await.unwrap(),
            ids[0]
        );
        assert_eq!(
            state.facade.find_previous_finish_epoch(2).await.unwrap(),
            ids[1]
        );
    }

    #[test_log::test(tokio::test)]
    async fn test_it_fails_to_find_previous_epoch_when_it_is_missing() {
        let docker = Cli::default();
        let mut state = TestState::setup(&docker).await;
        let inputs =
            vec![RollupsData::FinishEpoch {}, RollupsData::FinishEpoch {}];
        let mut ids = Vec::new();
        for input in inputs {
            ids.push(state.fixture.produce_input_event(input).await);
        }
        assert!(matches!(
            state
                .facade
                .find_previous_finish_epoch(3)
                .await
                .unwrap_err(),
            BrokerFacadeError::FindFinishEpochInputError { epoch: 2 }
        ));
    }

    #[test_log::test(tokio::test)]
    async fn test_it_consumes_inputs() {
        let docker = Cli::default();
        let mut state = TestState::setup(&docker).await;
        let inputs = vec![
            RollupsData::AdvanceStateInput(RollupsAdvanceStateInput {
                metadata: InputMetadata {
                    epoch_index: 0,
                    input_index: 0,
                    ..Default::default()
                },
                payload: Payload::new(vec![0, 0]),
                tx_hash: Hash::default(),
            }),
            RollupsData::FinishEpoch {},
            RollupsData::AdvanceStateInput(RollupsAdvanceStateInput {
                metadata: InputMetadata {
                    epoch_index: 1,
                    input_index: 1,
                    ..Default::default()
                },
                payload: Payload::new(vec![1, 1]),
                tx_hash: Hash::default(),
            }),
        ];
        let mut ids = Vec::new();
        for input in inputs.iter() {
            ids.push(state.fixture.produce_input_event(input.clone()).await);
        }
        assert_eq!(
            state.facade.consume_input(INITIAL_ID).await.unwrap(),
            Event {
                id: ids[0].clone(),
                payload: RollupsInput {
                    parent_id: INITIAL_ID.to_owned(),
                    epoch_index: 0,
                    inputs_sent_count: 1,
                    data: inputs[0].clone(),
                },
            }
        );
        assert_eq!(
            state.facade.consume_input(&ids[0]).await.unwrap(),
            Event {
                id: ids[1].clone(),
                payload: RollupsInput {
                    parent_id: ids[0].clone(),
                    epoch_index: 0,
                    inputs_sent_count: 1,
                    data: inputs[1].clone(),
                },
            }
        );
        assert_eq!(
            state.facade.consume_input(&ids[1]).await.unwrap(),
            Event {
                id: ids[2].clone(),
                payload: RollupsInput {
                    parent_id: ids[1].clone(),
                    epoch_index: 1,
                    inputs_sent_count: 2,
                    data: inputs[2].clone(),
                },
            }
        );
    }

    #[test_log::test(tokio::test)]
    async fn test_it_does_not_produce_claim_when_it_was_already_produced() {
        let docker = Cli::default();
        let mut state = TestState::setup(&docker).await;
        let rollups_claim = RollupsClaim {
            epoch_index: 0,
            epoch_hash: Hash::new([0xa0; HASH_SIZE]),
            first_index: 0,
            last_index: 6,
        };
        state
            .fixture
            .produce_rollups_claim(rollups_claim.clone())
            .await;
        state
            .facade
            .produce_rollups_claim(rollups_claim.clone())
            .await
            .unwrap();
        assert_eq!(
            state.fixture.consume_all_claims().await,
            vec![rollups_claim]
        );
    }

    #[test_log::test(tokio::test)]
    async fn test_it_produces_claims() {
        let docker = Cli::default();
        let mut state = TestState::setup(&docker).await;
        let rollups_claim0 = RollupsClaim {
            epoch_index: 0,
            epoch_hash: Hash::new([0xa0; HASH_SIZE]),
            first_index: 0,
            last_index: 0,
        };
        let rollups_claim1 = RollupsClaim {
            epoch_index: 1,
            epoch_hash: Hash::new([0xa1; HASH_SIZE]),
            first_index: 1,
            last_index: 1,
        };
        state
            .facade
            .produce_rollups_claim(rollups_claim0.clone())
            .await
            .unwrap();
        state
            .facade
            .produce_rollups_claim(rollups_claim1.clone())
            .await
            .unwrap();
        assert_eq!(
            state.fixture.consume_all_claims().await,
            vec![rollups_claim0, rollups_claim1]
        );
    }
}
