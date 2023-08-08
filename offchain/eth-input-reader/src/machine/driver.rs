// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::Context;

use super::{rollups_broker::BrokerFacadeError, BrokerSend};

use eth_state_fold_types::{ethereum_types::Address, Block};
use types::foldables::input_box::{DAppInputBox, Input, InputBox};

use tracing::{debug, instrument, trace};

pub struct MachineDriver {
    dapp_address: Address,
}

impl MachineDriver {
    pub fn new(dapp_address: Address) -> Self {
        Self { dapp_address }
    }

    #[instrument(level = "trace", skip_all)]
    pub async fn react(
        &self,
        context: &mut Context,
        block: &Block,
        input_box: &InputBox,
        broker: &impl BrokerSend,
    ) -> Result<(), BrokerFacadeError> {
        let dapp_input_box =
            match input_box.dapp_input_boxes.get(&self.dapp_address) {
                None => {
                    debug!("No inputs for dapp {}", self.dapp_address);
                    return Ok(());
                }

                Some(d) => d,
            };

        self.process_inputs(context, dapp_input_box, broker).await?;

        context
            .finish_epoch_if_needed(block.timestamp.as_u64(), broker)
            .await?;

        Ok(())
    }
}

impl MachineDriver {
    #[instrument(level = "trace", skip_all)]
    async fn process_inputs(
        &self,
        context: &mut Context,
        dapp_input_box: &DAppInputBox,
        broker: &impl BrokerSend,
    ) -> Result<(), BrokerFacadeError> {
        trace!(
            "Last input sent to machine manager `{}`, current input `{}`",
            context.inputs_sent_count(),
            dapp_input_box.inputs.len()
        );

        let input_slice = dapp_input_box
            .inputs
            .skip(context.inputs_sent_count() as usize);

        for input in input_slice {
            self.process_input(context, &input, broker).await?;
        }

        Ok(())
    }

    #[instrument(level = "trace", skip_all)]
    async fn process_input(
        &self,
        context: &mut Context,
        input: &Input,
        broker: &impl BrokerSend,
    ) -> Result<(), BrokerFacadeError> {
        let input_timestamp = input.block_added.timestamp.as_u64();
        trace!(?context, ?input_timestamp);

        context
            .finish_epoch_if_needed(input_timestamp, broker)
            .await?;

        context.enqueue_input(input, broker).await?;

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use eth_state_fold_types::{ethereum_types::H160, Block};
    use rollups_events::DAppMetadata;
    use std::sync::Arc;

    use crate::{
        machine::{
            mock::{self, SendInteraction},
            Context, RollupStatus,
        },
        metrics::EthInputReaderMetrics,
    };

    use super::MachineDriver;

    // --------------------------------------------------------------------------------------------
    // process_input
    // --------------------------------------------------------------------------------------------

    async fn test_process_input(
        rollup_status: RollupStatus,
        input_timestamps: Vec<u32>,
        expected: Vec<SendInteraction>,
    ) {
        let broker = mock::Broker::new(vec![rollup_status], Vec::new());
        let mut context = Context::new(
            0,
            5,
            &broker,
            DAppMetadata::default(),
            EthInputReaderMetrics::default(),
        )
        .await
        .unwrap(); // zero indexed!
        let machine_driver = MachineDriver::new(H160::random());
        for block_timestamp in input_timestamps {
            let input = mock::new_input(block_timestamp);
            let result = machine_driver
                .process_input(&mut context, &input, &broker)
                .await;
            assert!(result.is_ok());
        }

        broker.assert_send_interactions(expected);
    }

    #[tokio::test]
    async fn process_input_right_before_finish_epoch() {
        let rollup_status = RollupStatus {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: false,
        };
        let input_timestamps = vec![4];
        let send_interactions = vec![SendInteraction::EnqueuedInput(0)];
        test_process_input(rollup_status, input_timestamps, send_interactions)
            .await;
    }

    #[tokio::test]
    async fn process_input_at_finish_epoch() {
        let rollup_status = RollupStatus {
            inputs_sent_count: 1,
            last_event_is_finish_epoch: false,
        };
        let input_timestamps = vec![5];
        let send_interactions = vec![
            SendInteraction::FinishedEpoch(1),
            SendInteraction::EnqueuedInput(1),
        ];
        test_process_input(rollup_status, input_timestamps, send_interactions)
            .await;
    }

    #[tokio::test]
    async fn process_input_last_event_is_finish_epoch() {
        let rollup_status = RollupStatus {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: true,
        };
        let input_timestamps = vec![5];
        let send_interactions = vec![SendInteraction::EnqueuedInput(0)];
        test_process_input(rollup_status, input_timestamps, send_interactions)
            .await;
    }

    #[tokio::test]
    async fn process_input_after_finish_epoch() {
        let rollup_status = RollupStatus {
            inputs_sent_count: 3,
            last_event_is_finish_epoch: false,
        };
        let input_timestamps = vec![6, 7];
        let send_interactions = vec![
            SendInteraction::FinishedEpoch(3),
            SendInteraction::EnqueuedInput(3),
            SendInteraction::EnqueuedInput(4),
        ];
        test_process_input(rollup_status, input_timestamps, send_interactions)
            .await;
    }

    #[tokio::test]
    async fn process_input_crossing_two_epochs() {
        let rollup_status = RollupStatus {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: false,
        };
        let input_timestamps = vec![3, 4, 5, 6, 7, 9, 10, 11];
        let send_interactions = vec![
            SendInteraction::EnqueuedInput(0),
            SendInteraction::EnqueuedInput(1),
            SendInteraction::FinishedEpoch(2),
            SendInteraction::EnqueuedInput(2),
            SendInteraction::EnqueuedInput(3),
            SendInteraction::EnqueuedInput(4),
            SendInteraction::EnqueuedInput(5),
            SendInteraction::FinishedEpoch(6),
            SendInteraction::EnqueuedInput(6),
            SendInteraction::EnqueuedInput(7),
        ];
        test_process_input(rollup_status, input_timestamps, send_interactions)
            .await;
    }

    // --------------------------------------------------------------------------------------------
    // process_inputs
    // --------------------------------------------------------------------------------------------

    async fn test_process_inputs(
        rollup_status: RollupStatus,
        input_timestamps: Vec<u32>,
        expected: Vec<SendInteraction>,
    ) {
        let broker = mock::Broker::new(vec![rollup_status], Vec::new());
        let mut context = Context::new(
            0,
            5,
            &broker,
            DAppMetadata::default(),
            EthInputReaderMetrics::default(),
        )
        .await
        .unwrap(); // zero indexed!
        let machine_driver = MachineDriver::new(H160::random());
        let dapp_input_box = types::foldables::input_box::DAppInputBox {
            inputs: input_timestamps
                .iter()
                .map(|timestamp| Arc::new(mock::new_input(*timestamp)))
                .collect::<Vec<_>>()
                .into(),
        };
        let result = machine_driver
            .process_inputs(&mut context, &dapp_input_box, &broker)
            .await;
        assert!(result.is_ok());

        broker.assert_send_interactions(expected);
    }

    #[tokio::test]
    async fn test_process_inputs_without_skipping() {
        let rollup_status = RollupStatus {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: false,
        };
        let input_timestamps = vec![1, 2, 3, 4];
        let send_interactions = vec![
            SendInteraction::EnqueuedInput(0),
            SendInteraction::EnqueuedInput(1),
            SendInteraction::EnqueuedInput(2),
            SendInteraction::EnqueuedInput(3),
        ];
        test_process_inputs(rollup_status, input_timestamps, send_interactions)
            .await;
    }

    #[tokio::test]
    async fn process_inputs_with_some_skipping() {
        let rollup_status = RollupStatus {
            inputs_sent_count: 3,
            last_event_is_finish_epoch: false,
        };
        let input_timestamps = vec![1, 2, 3, 4];
        let send_interactions = vec![SendInteraction::EnqueuedInput(3)];
        test_process_inputs(rollup_status, input_timestamps, send_interactions)
            .await;
    }

    #[tokio::test]
    async fn process_inputs_skipping_all() {
        let rollup_status = RollupStatus {
            inputs_sent_count: 4,
            last_event_is_finish_epoch: false,
        };
        let input_timestamps = vec![1, 2, 3, 4];
        let send_interactions = vec![];
        test_process_inputs(rollup_status, input_timestamps, send_interactions)
            .await;
    }

    // --------------------------------------------------------------------------------------------
    // react
    // --------------------------------------------------------------------------------------------

    async fn test_react(
        block: Block,
        rollup_status: RollupStatus,
        input_timestamps: Vec<u32>,
        expected: Vec<SendInteraction>,
    ) {
        let broker = mock::Broker::new(vec![rollup_status], Vec::new());
        let mut context = Context::new(
            0,
            5,
            &broker,
            DAppMetadata::default(),
            EthInputReaderMetrics::default(),
        )
        .await
        .unwrap(); // zero indexed!

        let dapp_address = H160::random();
        let machine_driver = MachineDriver::new(dapp_address);

        let input_box = mock::new_input_box();
        let input_box =
            mock::update_input_box(input_box, dapp_address, input_timestamps);

        let result = machine_driver
            .react(&mut context, &block, &input_box, &broker)
            .await;
        assert!(result.is_ok());

        broker.assert_send_interactions(expected);
    }

    #[tokio::test]
    async fn react_without_finish_epoch() {
        let block = mock::new_block(3);
        let rollup_status = RollupStatus {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: false,
        };
        let input_timestamps = vec![1, 2];
        let send_interactions = vec![
            SendInteraction::EnqueuedInput(0),
            SendInteraction::EnqueuedInput(1),
        ];
        test_react(block, rollup_status, input_timestamps, send_interactions)
            .await;
    }

    #[tokio::test]
    async fn react_with_finish_epoch() {
        let block = mock::new_block(5);
        let rollup_status = RollupStatus {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: false,
        };
        let input_timestamps = vec![1, 2];
        let send_interactions = vec![
            SendInteraction::EnqueuedInput(0),
            SendInteraction::EnqueuedInput(1),
            SendInteraction::FinishedEpoch(2),
        ];
        test_react(block, rollup_status, input_timestamps, send_interactions)
            .await;
    }

    #[tokio::test]
    async fn react_with_internal_finish_epoch() {
        let block = mock::new_block(5);
        let rollup_status = RollupStatus {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: false,
        };
        let input_timestamps = vec![4, 5];
        let send_interactions = vec![
            SendInteraction::EnqueuedInput(0),
            SendInteraction::FinishedEpoch(1),
            SendInteraction::EnqueuedInput(1),
        ];
        test_react(block, rollup_status, input_timestamps, send_interactions)
            .await;
    }

    #[tokio::test]
    async fn react_without_inputs() {
        let rollup_status = RollupStatus {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: false,
        };
        let broker = mock::Broker::new(vec![rollup_status], Vec::new());
        let mut context = Context::new(
            0,
            5,
            &broker,
            DAppMetadata::default(),
            EthInputReaderMetrics::default(),
        )
        .await
        .unwrap(); // zero indexed!
        let block = mock::new_block(5);
        let input_box = mock::new_input_box();
        let machine_driver = MachineDriver::new(H160::random());
        let result = machine_driver
            .react(&mut context, &block, &input_box, &broker)
            .await;
        assert!(result.is_ok());
        broker.assert_send_interactions(vec![]);
    }

    #[tokio::test]
    async fn react_with_inputs_after_first_epoch_length() {
        let block = mock::new_block(5);
        let rollup_status = RollupStatus {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: false,
        };
        let input_timestamps = vec![7, 8];
        let send_interactions = vec![
            SendInteraction::EnqueuedInput(0),
            SendInteraction::FinishedEpoch(1),
            SendInteraction::EnqueuedInput(1),
        ];
        test_react(block, rollup_status, input_timestamps, send_interactions)
            .await;
    }
}
