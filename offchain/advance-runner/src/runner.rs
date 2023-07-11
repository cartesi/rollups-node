// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use rollups_events::{Event, InputMetadata, RollupsData, RollupsInput};
use snafu::{ResultExt, Snafu};

use crate::broker::{BrokerFacade, BrokerFacadeError};
use crate::server_manager::{ServerManagerError, ServerManagerFacade};
use crate::snapshot::SnapshotManager;

#[derive(Debug, Snafu)]
pub enum RunnerError<SnapError: snafu::Error + 'static> {
    #[snafu(display("failed to to create session in server-manager"))]
    CreateSessionError { source: ServerManagerError },

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

    #[snafu(display("failed to get storage directory"))]
    GetStorageDirectoryError { source: SnapError },

    #[snafu(display("failed to get latest snapshot"))]
    GetLatestSnapshotError { source: SnapError },

    #[snafu(display("failed to set latest snapshot"))]
    SetLatestSnapshotError { source: SnapError },

    #[snafu(display(
        "parent id doesn't match expected={} got={}",
        expected,
        got
    ))]
    ParentIdMismatchError { expected: String, got: String },
}

type Result<T, SnapError> = std::result::Result<T, RunnerError<SnapError>>;

pub struct Runner<Snap: SnapshotManager> {
    server_manager: ServerManagerFacade,
    broker: BrokerFacade,
    snapshot_manager: Snap,
}

impl<Snap: SnapshotManager + std::fmt::Debug + 'static> Runner<Snap> {
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn start(
        server_manager: ServerManagerFacade,
        broker: BrokerFacade,
        snapshot_manager: Snap,
    ) -> Result<(), Snap::Error> {
        let mut runner = Self {
            server_manager,
            broker,
            snapshot_manager,
        };
        let mut last_id = runner.setup().await?;

        tracing::info!(last_id, "starting runner main loop");
        loop {
            let event = runner.consume_next(&last_id).await?;
            tracing::info!(?event, "consumed input event");

            match event.payload.data {
                RollupsData::AdvanceStateInput(input) => {
                    runner
                        .handle_advance(
                            event.payload.epoch_index,
                            event.payload.inputs_sent_count,
                            input.metadata,
                            input.payload.into_inner(),
                        )
                        .await?;
                }
                RollupsData::FinishEpoch {} => {
                    runner
                        .handle_finish(
                            event.payload.epoch_index,
                            event.payload.inputs_sent_count,
                        )
                        .await?;
                }
            }

            last_id = event.id;
            tracing::info!(last_id, "waiting for the next input event");
        }
    }

    #[tracing::instrument(level = "trace", skip_all)]
    async fn setup(&mut self) -> Result<String, Snap::Error> {
        tracing::trace!("setting up runner");

        let snapshot = self
            .snapshot_manager
            .get_latest()
            .await
            .context(GetLatestSnapshotSnafu)?;
        tracing::info!(?snapshot, "got latest snapshot");

        let event_id = self
            .broker
            .find_previous_finish_epoch(snapshot.epoch)
            .await
            .context(FindFinishEpochInputSnafu)?;
        tracing::trace!(event_id, "found finish epoch input event");

        self.server_manager
            .start_session(
                &snapshot.path,
                snapshot.epoch,
                snapshot.processed_input_count,
            )
            .await
            .context(CreateSessionSnafu)?;

        Ok(event_id)
    }

    #[tracing::instrument(level = "trace", skip_all)]
    async fn consume_next(
        &mut self,
        last_id: &str,
    ) -> Result<Event<RollupsInput>, Snap::Error> {
        tracing::trace!("consuming next event input");

        let event = self
            .broker
            .consume_input(last_id)
            .await
            .context(ConsumeInputSnafu)?;
        tracing::trace!("input event consumed from broker");

        if event.payload.parent_id != last_id {
            Err(RunnerError::ParentIdMismatchError {
                expected: last_id.to_owned(),
                got: event.payload.parent_id,
            })
        } else {
            Ok(event)
        }
    }

    #[tracing::instrument(level = "trace", skip_all)]
    async fn handle_advance(
        &mut self,
        epoch_index: u64,
        inputs_sent_count: u64,
        input_metadata: InputMetadata,
        input_payload: Vec<u8>,
    ) -> Result<(), Snap::Error> {
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
    async fn handle_finish(
        &mut self,
        epoch_index: u64,
        inputs_sent_count: u64,
    ) -> Result<(), Snap::Error> {
        tracing::trace!("handling finish");

        // We add one to the epoch index because the snapshot is for the one after we are closing
        let snapshot = self
            .snapshot_manager
            .get_storage_directory(epoch_index + 1, inputs_sent_count)
            .await
            .context(GetStorageDirectorySnafu)?;
        tracing::trace!(?snapshot, "got storage directory");

        let result = self
            .server_manager
            .finish_epoch(epoch_index, &snapshot.path)
            .await;
        tracing::trace!("finished epoch in server-manager");

        match result {
            Ok((rollups_claim, proofs)) => {
                self.broker
                    .produce_outputs(proofs)
                    .await
                    .context(ProduceOutputsSnafu)?;
                tracing::trace!("produced outputs in broker");

                self.broker
                    .produce_rollups_claim(rollups_claim)
                    .await
                    .context(ProduceClaimSnafu)?;
                tracing::info!("produced epoch claim");
            }
            Err(source) => {
                if let ServerManagerError::EmptyEpochError { .. } = source {
                    tracing::warn!("{}", source)
                } else {
                    return Err(RunnerError::FinishEpochError { source });
                }
            }
        }

        self.snapshot_manager
            .set_latest(snapshot)
            .await
            .context(SetLatestSnapshotSnafu)?;
        tracing::trace!("set latest snapshot");

        Ok(())
    }
}
