// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use std::sync::Arc;

use super::Context;

use crate::{
    drivers::context::timestamp_to_string,
    machine::{rollups_broker::BrokerFacadeError, BrokerSend},
};

use eth_state_fold_types::{ethereum_types::Address, Block};
use types::foldables::{DAppInputBox, Input, InputBox};

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
        println!("---");
        println!(
            "Início do react! block.timestamp: {}",
            timestamp_to_string(block.timestamp.as_u64())
        );
        println!("---");

        let dapp_input_box =
            match input_box.dapp_input_boxes.get(&self.dapp_address) {
                None => {
                    debug!("No inputs for dapp {}", self.dapp_address);
                    return Ok(());
                }

                Some(d) => d,
            };

        let last_input =
            self.process_inputs(context, dapp_input_box, broker).await?;

        if let Some(last_input) = last_input {
            context
                .finish_epoch_if_needed(
                    last_input.block_added.timestamp.as_u64(),
                    broker,
                )
                .await?;
        }

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
    ) -> Result<Option<Arc<Input>>, BrokerFacadeError> {
        tracing::trace!(
            "Last input sent to machine manager `{}`, current input `{}`",
            context.inputs_sent_count(),
            dapp_input_box.inputs.len()
        );

        let input_slice = dapp_input_box
            .inputs
            .skip(context.inputs_sent_count() as usize);

        let last_input = input_slice.last().cloned();

        for input in input_slice {
            self.process_input(context, &input, broker).await?;
        }

        Ok(last_input)
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
    use eth_state_fold_types::{
        ethereum_types::{Address, H160},
        Block,
    };
    use rollups_events::DAppMetadata;
    use serial_test::serial;
    use std::sync::Arc;
    use types::foldables::InputBox;

    use crate::{
        drivers::{
            mock::{self, Broker, SendInteraction},
            Context,
        },
        machine::RollupStatus,
        metrics::DispatcherMetrics,
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
            DAppMetadata::default(),
            DispatcherMetrics::default(),
            rollup_status,
        );
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
            DAppMetadata::default(),
            DispatcherMetrics::default(),
            rollup_status,
        );
        let machine_driver = MachineDriver::new(H160::random());
        let dapp_input_box = types::foldables::DAppInputBox {
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

    struct ReactState {
        context: Context,
        input_box: InputBox,
        broker: Broker,

        dapp_address: Address,
        machine_driver: MachineDriver,
    }

    impl ReactState {
        fn new(context: Context, rollup_status: RollupStatus) -> Self {
            let dapp_address = H160::random();
            return ReactState {
                context,
                input_box: mock::new_input_box(),
                broker: mock::Broker::new(vec![rollup_status], Vec::new()),
                dapp_address,
                machine_driver: MachineDriver::new(dapp_address),
            };
        }

        async fn test(
            mut self,
            block: Block,
            inputs: Vec<u32>, // timestamps
            expected: Vec<SendInteraction>,
        ) -> Self {
            self.input_box = mock::update_input_box(
                self.input_box,
                self.dapp_address,
                inputs,
            );

            let result = self
                .machine_driver
                .react(&mut self.context, &block, &self.input_box, &self.broker)
                .await;
            assert!(result.is_ok());

            self.broker.assert_send_interactions(expected);

            return self;
        }
    }

    fn default_context(
        genesis_timestamp: u64,
        epoch_length: u64,
        rollups_status: RollupStatus,
    ) -> Context {
        Context::new(
            genesis_timestamp,
            epoch_length,
            DAppMetadata::default(),
            DispatcherMetrics::default(),
            rollups_status,
        )
    }

    #[tokio::test]
    #[serial]
    async fn react_bug_buster_original() {
        println!("original ==============================================");

        let genesis_timestamp = 1716392210;
        let epoch_length = 86400;
        let rollup_status = RollupStatus {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: false,
        };
        let context =
            default_context(genesis_timestamp, epoch_length, rollup_status);
        let mut state = ReactState::new(context, rollup_status);

        let block1 = mock::new_block(1716774006);
        let block2 = mock::new_block(1716858268);

        let inputs1 = vec![
            1716495424, //
            1716514994, //
            1716550722, //
            1716551814, //
            1716552408, //
            1716558302, //
            1716558322, //
            1716564194, //
            1716564306, //
            1716564696, //
            1716568314, //
            1716568652, //
            1716569100, //
            1716569136, //
            1716578858, //
            1716578948, //
        ];
        let mut inputs2 = inputs1.clone();
        inputs2.append(&mut vec![
            1716858268, //
            1716858428, //
            1716859860, //
        ]);

        let expected1 = vec![
            SendInteraction::EnqueuedInput(0),
            SendInteraction::FinishedEpoch(1),
            SendInteraction::EnqueuedInput(1),
            SendInteraction::EnqueuedInput(2),
            SendInteraction::EnqueuedInput(3),
            SendInteraction::EnqueuedInput(4),
            SendInteraction::EnqueuedInput(5),
            SendInteraction::EnqueuedInput(6),
            SendInteraction::EnqueuedInput(7),
            SendInteraction::EnqueuedInput(8),
            SendInteraction::EnqueuedInput(9),
            SendInteraction::FinishedEpoch(10),
            SendInteraction::EnqueuedInput(10),
            SendInteraction::EnqueuedInput(11),
            SendInteraction::EnqueuedInput(12),
            SendInteraction::EnqueuedInput(13),
            SendInteraction::EnqueuedInput(14),
            SendInteraction::EnqueuedInput(15),
        ];
        let mut expected2 = expected1.clone();
        expected2.append(&mut vec![
            SendInteraction::FinishedEpoch(16),
            SendInteraction::EnqueuedInput(16),
            SendInteraction::EnqueuedInput(17),
            SendInteraction::EnqueuedInput(18),
        ]);

        state = state.test(block1, inputs1, expected1).await;
        _ = state.test(block2, inputs2, expected2).await;
    }

    #[tokio::test]
    #[serial]
    async fn react_bug_buster_reconstruction() {
        println!(
            "reconstruction =============================================="
        );

        let genesis_timestamp = 1716392210;
        let epoch_length = 86400;
        let rollup_status = RollupStatus {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: false,
        };
        let context =
            default_context(genesis_timestamp, epoch_length, rollup_status);
        let state = ReactState::new(context, rollup_status);

        let block = mock::new_block(1716859860);
        let inputs = vec![
            1716495424, //
            1716514994, //
            1716550722, //
            1716551814, //
            1716552408, //
            1716558302, //
            1716558322, //
            1716564194, //
            1716564306, //
            1716564696, //
            1716568314, //
            1716568652, //
            1716569100, //
            1716569136, //
            1716578858, //
            1716578948, //
            1716858268, // extra
            1716858428, // extra
            1716859860, // extra
        ];
        let expected = vec![
            SendInteraction::EnqueuedInput(0),
            SendInteraction::FinishedEpoch(1),
            SendInteraction::EnqueuedInput(1),
            SendInteraction::EnqueuedInput(2),
            SendInteraction::EnqueuedInput(3),
            SendInteraction::EnqueuedInput(4),
            SendInteraction::EnqueuedInput(5),
            SendInteraction::EnqueuedInput(6),
            SendInteraction::EnqueuedInput(7),
            SendInteraction::EnqueuedInput(8),
            SendInteraction::EnqueuedInput(9),
            SendInteraction::FinishedEpoch(10),
            SendInteraction::EnqueuedInput(10),
            SendInteraction::EnqueuedInput(11),
            SendInteraction::EnqueuedInput(12),
            SendInteraction::EnqueuedInput(13),
            SendInteraction::EnqueuedInput(14),
            SendInteraction::EnqueuedInput(15),
            SendInteraction::FinishedEpoch(16),
            SendInteraction::EnqueuedInput(16),
            SendInteraction::EnqueuedInput(17),
            SendInteraction::EnqueuedInput(18),
        ];
        _ = state.test(block, inputs, expected).await;
    }

    //   #[tokio::test]
    //   async fn react_without_finish_epoch() {
    //       let block = mock::new_block(3);
    //       let rollup_status = RollupStatus {
    //           inputs_sent_count: 0,
    //           last_event_is_finish_epoch: false,
    //       };
    //       let input_timestamps = vec![1, 2];
    //       let send_interactions = vec![
    //           SendInteraction::EnqueuedInput(0),
    //           SendInteraction::EnqueuedInput(1),
    //       ];
    //       test_react(block, rollup_status, input_timestamps, send_interactions)
    //           .await;
    //   }
    //
    //   #[tokio::test]
    //   async fn react_with_finish_epoch() {
    //       let block = mock::new_block(5);
    //       let rollup_status = RollupStatus {
    //           inputs_sent_count: 0,
    //           last_event_is_finish_epoch: false,
    //       };
    //       let input_timestamps = vec![1, 2];
    //       let send_interactions = vec![
    //           SendInteraction::EnqueuedInput(0),
    //           SendInteraction::EnqueuedInput(1),
    //           SendInteraction::FinishedEpoch(2),
    //       ];
    //       test_react(block, rollup_status, input_timestamps, send_interactions)
    //           .await;
    //   }
    //
    //   #[tokio::test]
    //   async fn react_with_internal_finish_epoch() {
    //       let block = mock::new_block(5);
    //       let rollup_status = RollupStatus {
    //           inputs_sent_count: 0,
    //           last_event_is_finish_epoch: false,
    //       };
    //       let input_timestamps = vec![4, 5];
    //       let send_interactions = vec![
    //           SendInteraction::EnqueuedInput(0),
    //           SendInteraction::FinishedEpoch(1),
    //           SendInteraction::EnqueuedInput(1),
    //       ];
    //       test_react(block, rollup_status, input_timestamps, send_interactions)
    //           .await;
    //   }
    //
    //   #[tokio::test]
    //   async fn react_without_inputs() {
    //       let rollup_status = RollupStatus {
    //           inputs_sent_count: 0,
    //           last_event_is_finish_epoch: false,
    //       };
    //       let broker = mock::Broker::new(vec![rollup_status], Vec::new());
    //       let mut context = Context::new(
    //           0,
    //           5,
    //           DAppMetadata::default(),
    //           DispatcherMetrics::default(),
    //           rollup_status,
    //       );
    //       let block = mock::new_block(5);
    //       let input_box = mock::new_input_box();
    //       let machine_driver = MachineDriver::new(H160::random());
    //       let result = machine_driver
    //           .react(&mut context, &block, &input_box, &broker)
    //           .await;
    //       assert!(result.is_ok());
    //       broker.assert_send_interactions(vec![]);
    //   }
    //
    //   #[tokio::test]
    //   async fn react_with_inputs_after_first_epoch_length() {
    //       let block = mock::new_block(5);
    //       let rollup_status = RollupStatus {
    //           inputs_sent_count: 0,
    //           last_event_is_finish_epoch: false,
    //       };
    //       let input_timestamps = vec![7, 8];
    //       let send_interactions = vec![
    //           SendInteraction::EnqueuedInput(0),
    //           SendInteraction::FinishedEpoch(1),
    //           SendInteraction::EnqueuedInput(1),
    //       ];
    //       test_react(block, rollup_status, input_timestamps, send_interactions)
    //           .await;
    //   }
}
