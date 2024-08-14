// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use rollups_events::{InputMetadata, RollupsData};
use snafu::{ResultExt, Snafu};

use crate::broker::{BrokerFacade, BrokerFacadeError};
use crate::server_manager::{ServerManagerError, ServerManagerFacade};

#[derive(Debug, Snafu)]
pub enum RunnerError {
    #[snafu(display("failed to send advance-state input to server-manager"))]
    AdvanceError { source: ServerManagerError },

    #[snafu(display("failed to finish epoch in server-manager"))]
    FinishEpochError { source: ServerManagerError },

    #[snafu(display("failed to get epoch claim from server-manager"))]
    GetEpochClaimError { source: ServerManagerError },

    #[snafu(display("failed to find finish epoch input event"))]
    FindFinishEpochInputError { source: BrokerFacadeError },

    #[snafu(display("failed to consume input from broker"))]
    ConsumeInputError { source: BrokerFacadeError },

    #[snafu(display("failed to get whether claim was produced"))]
    PeekClaimError { source: BrokerFacadeError },

    #[snafu(display("failed to produce claim in broker"))]
    ProduceClaimError { source: BrokerFacadeError },

    #[snafu(display("failed to produce outputs in broker"))]
    ProduceOutputsError { source: BrokerFacadeError },
}

type Result<T> = std::result::Result<T, RunnerError>;

pub struct Runner {
    server_manager: ServerManagerFacade,
    broker: BrokerFacade,
}

impl Runner {
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn start(
        server_manager: ServerManagerFacade,
        broker: BrokerFacade,
    ) -> Result<()> {
        let mut runner = Self {
            server_manager,
            broker,
        };

        tracing::info!("starting runner main loop");
        loop {
            let event = runner
                .broker
                .consume_input()
                .await
                .context(ConsumeInputSnafu)?;
            tracing::info!(?event, "consumed input event");

            match event.data {
                RollupsData::AdvanceStateInput(input) => {
                    runner
                        .handle_advance(
                            event.epoch_index,
                            event.inputs_sent_count,
                            input.metadata,
                            input.payload.into_inner(),
                        )
                        .await?;
                }
                RollupsData::FinishEpoch {} => {
                    runner.handle_finish(event.epoch_index).await?;
                }
            }
            tracing::info!("waiting for the next input event");
        }
    }

    #[tracing::instrument(level = "trace", skip_all)]
    async fn handle_advance(
        &mut self,
        epoch_index: u64,
        inputs_sent_count: u64,
        input_metadata: InputMetadata,
        input_payload: Vec<u8>,
    ) -> Result<()> {
        tracing::trace!("handling advance state");

        let input_index = inputs_sent_count - 1;
        let outputs = self
            .server_manager
            .advance_state(
                epoch_index,
                input_index,
                input_metadata,
                input_payload,
            )
            .await
            .context(AdvanceSnafu)?;
        tracing::trace!("advance state sent to server-manager");

        self.broker
            .produce_outputs(outputs)
            .await
            .context(ProduceOutputsSnafu)?;
        tracing::trace!("produced outputs in broker");

        Ok(())
    }

    #[tracing::instrument(level = "trace", skip_all)]
    async fn handle_finish(&mut self, epoch_index: u64) -> Result<()> {
        tracing::trace!("handling finish");

        let result = self.server_manager.finish_epoch(epoch_index).await;
        tracing::trace!("finished epoch in server-manager");

        match result {
            Ok((rollups_claim, proofs)) => {
                self.broker
                    .produce_outputs(proofs)
                    .await
                    .context(ProduceOutputsSnafu)?;
                tracing::trace!("produced outputs in broker stream");

                self.broker
                    .produce_rollups_claim(rollups_claim)
                    .await
                    .context(ProduceClaimSnafu)?;
                tracing::info!("produced epoch claim in broker stream");
            }
            Err(source) => {
                if let ServerManagerError::EmptyEpochError { .. } = source {
                    tracing::warn!("{}", source)
                } else {
                    return Err(RunnerError::FinishEpochError { source });
                }
            }
        }
        Ok(())
    }
}
