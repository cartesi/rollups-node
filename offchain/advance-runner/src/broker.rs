// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use rollups_events::{
    Broker, BrokerConfig, BrokerError, DAppMetadata, RollupsClaim,
    RollupsClaimsStream, RollupsInput, RollupsInputsStream, RollupsOutput,
    RollupsOutputsStream, INITIAL_ID,
};
use snafu::{ResultExt, Snafu};

#[derive(Debug, Snafu)]
pub enum BrokerFacadeError {
    #[snafu(display("broker internal error"))]
    BrokerInternalError { source: BrokerError },

    #[snafu(display(
        "expected first_index from claim to be {}, but got {}",
        expected,
        got
    ))]
    InvalidIndexes { expected: u128, got: u128 },

    #[snafu(display("failed to consume input event"))]
    ConsumeError { source: BrokerError },

    #[snafu(display(
        "failed to find finish epoch input event epoch={}",
        epoch
    ))]
    FindFinishEpochInputError { epoch: u64 },

    #[snafu(display("processed event not found in broker"))]
    ProcessedEventNotFound {},

    #[snafu(display(
        "parent id doesn't match expected={} got={}",
        expected,
        got
    ))]
    ParentIdMismatchError { expected: String, got: String },
}

pub type Result<T> = std::result::Result<T, BrokerFacadeError>;

pub struct BrokerFacade {
    client: Broker,
    inputs_stream: RollupsInputsStream,
    outputs_stream: RollupsOutputsStream,
    claims_stream: RollupsClaimsStream,
    reader_mode: bool,
    last_id: String,
}

impl BrokerFacade {
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn new(
        config: BrokerConfig,
        dapp_metadata: DAppMetadata,
        reader_mode: bool,
    ) -> Result<Self> {
        tracing::trace!(?config, "connecting to broker");
        let client = Broker::new(config).await.context(BrokerInternalSnafu)?;
        let inputs_stream = RollupsInputsStream::new(&dapp_metadata);
        let outputs_stream = RollupsOutputsStream::new(&dapp_metadata);
        let claims_stream = RollupsClaimsStream::new(&dapp_metadata);
        Ok(Self {
            client,
            inputs_stream,
            outputs_stream,
            claims_stream,
            reader_mode,
            last_id: INITIAL_ID.to_owned(),
        })
    }

    /// Consume rollups input event
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn consume_input(&mut self) -> Result<RollupsInput> {
        tracing::trace!(self.last_id, "consuming rollups input event");
        let event = self
            .client
            .consume_blocking(&self.inputs_stream, &self.last_id)
            .await
            .context(BrokerInternalSnafu)?;
        if event.payload.parent_id != self.last_id {
            Err(BrokerFacadeError::ParentIdMismatchError {
                expected: self.last_id.to_owned(),
                got: event.payload.parent_id,
            })
        } else {
            self.last_id = event.id;
            Ok(event.payload)
        }
    }

    /// Produce the rollups claim if it isn't in the stream yet
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn produce_rollups_claim(
        &mut self,
        rollups_claim: RollupsClaim,
    ) -> Result<()> {
        if self.reader_mode {
            return Ok(());
        }

        tracing::trace!(rollups_claim.epoch_index,
            ?rollups_claim.epoch_hash,
            "producing rollups claim for stream {:?}",
            self.claims_stream,
        );

        let last_claim_event = self
            .client
            .peek_latest(&self.claims_stream)
            .await
            .context(BrokerInternalSnafu)?;

        let should_enqueue_claim = match last_claim_event {
            Some(event) => {
                let last_claim = event.payload;
                tracing::trace!(
                    ?last_claim,
                    "got last claim from broker stream"
                );
                let should_enqueue_claim =
                    rollups_claim.epoch_index > last_claim.epoch_index;

                // If this happens, then something is wrong with the dispatcher.
                let invalid_indexes =
                    rollups_claim.first_index != last_claim.last_index + 1;
                if should_enqueue_claim && invalid_indexes {
                    tracing::debug!("rollups_claim.first_index = {}, last_claim.last_index = {}",
                        rollups_claim.first_index, last_claim.last_index);
                    return Err(BrokerFacadeError::InvalidIndexes {
                        expected: last_claim.last_index + 1,
                        got: rollups_claim.first_index,
                    });
                };

                should_enqueue_claim
            }
            None => {
                tracing::trace!("no claims in the stream");
                true
            }
        };

        if should_enqueue_claim {
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
        Address, DAppMetadata, Hash, InputMetadata, Payload,
        RollupsAdvanceStateInput, RollupsData, ADDRESS_SIZE, HASH_SIZE,
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
            let facade = BrokerFacade::new(config, dapp_metadata, false)
                .await
                .expect("failed to create broker facade");
            TestState { fixture, facade }
        }
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
            state.facade.consume_input().await.unwrap(),
            RollupsInput {
                parent_id: INITIAL_ID.to_owned(),
                epoch_index: 0,
                inputs_sent_count: 1,
                data: inputs[0].clone(),
            },
        );
        assert_eq!(
            state.facade.consume_input().await.unwrap(),
            RollupsInput {
                parent_id: ids[0].clone(),
                epoch_index: 0,
                inputs_sent_count: 1,
                data: inputs[1].clone(),
            },
        );
        assert_eq!(
            state.facade.consume_input().await.unwrap(),
            RollupsInput {
                parent_id: ids[1].clone(),
                epoch_index: 1,
                inputs_sent_count: 2,
                data: inputs[2].clone(),
            },
        );
    }

    #[test_log::test(tokio::test)]
    async fn test_it_does_not_produce_claim_when_it_was_already_produced() {
        let docker = Cli::default();
        let mut state = TestState::setup(&docker).await;
        let rollups_claim = RollupsClaim {
            dapp_address: Address::new([0xa0; ADDRESS_SIZE]),
            epoch_index: 0,
            epoch_hash: Hash::new([0xb0; HASH_SIZE]),
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
            dapp_address: Address::new([0xa0; ADDRESS_SIZE]),
            epoch_index: 0,
            epoch_hash: Hash::new([0xb0; HASH_SIZE]),
            first_index: 0,
            last_index: 0,
        };
        let rollups_claim1 = RollupsClaim {
            dapp_address: Address::new([0xa1; ADDRESS_SIZE]),
            epoch_index: 1,
            epoch_hash: Hash::new([0xb1; HASH_SIZE]),
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

    #[test_log::test(tokio::test)]
    async fn test_invalid_indexes_overlapping() {
        let docker = Cli::default();
        let mut state = TestState::setup(&docker).await;
        let rollups_claim1 = RollupsClaim {
            dapp_address: Address::new([0xa0; ADDRESS_SIZE]),
            epoch_index: 0,
            epoch_hash: Hash::new([0xb0; HASH_SIZE]),
            first_index: 0,
            last_index: 6,
        };
        let rollups_claim2 = RollupsClaim {
            dapp_address: Address::new([0xa0; ADDRESS_SIZE]),
            epoch_index: 1,
            epoch_hash: Hash::new([0xb0; HASH_SIZE]),
            first_index: 6,
            last_index: 7,
        };
        state
            .fixture
            .produce_rollups_claim(rollups_claim1.clone())
            .await;
        let result = state
            .facade
            .produce_rollups_claim(rollups_claim2.clone())
            .await;
        assert!(result.is_err());
        assert_eq!(
            BrokerFacadeError::InvalidIndexes {
                expected: 7,
                got: 6
            }
            .to_string(),
            result.unwrap_err().to_string()
        )
    }

    #[test_log::test(tokio::test)]
    async fn test_invalid_indexes_nonsequential() {
        let docker = Cli::default();
        let mut state = TestState::setup(&docker).await;
        let rollups_claim1 = RollupsClaim {
            dapp_address: Address::new([0xa0; ADDRESS_SIZE]),
            epoch_index: 0,
            epoch_hash: Hash::new([0xb0; HASH_SIZE]),
            first_index: 0,
            last_index: 6,
        };
        let rollups_claim2 = RollupsClaim {
            dapp_address: Address::new([0xa0; ADDRESS_SIZE]),
            epoch_index: 1,
            epoch_hash: Hash::new([0xb0; HASH_SIZE]),
            first_index: 11,
            last_index: 14,
        };
        state
            .fixture
            .produce_rollups_claim(rollups_claim1.clone())
            .await;
        let result = state
            .facade
            .produce_rollups_claim(rollups_claim2.clone())
            .await;
        assert!(result.is_err());
        assert_eq!(
            BrokerFacadeError::InvalidIndexes {
                expected: 7,
                got: 11
            }
            .to_string(),
            result.unwrap_err().to_string()
        )
    }
}
