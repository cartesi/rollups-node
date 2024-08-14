// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::Context;

use crate::machine::{rollups_broker::BrokerFacadeError, BrokerSend};

use eth_state_fold_types::{ethereum_types::Address, Block};
use types::foldables::{DAppInputBox, InputBox};

use tracing::{debug, instrument};

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
        match input_box.dapp_input_boxes.get(&self.dapp_address) {
            None => {
                debug!("No inputs for dapp {}", self.dapp_address);
            }
            Some(dapp_input_box) => {
                self.process_inputs(context, dapp_input_box, broker).await?
            }
        };

        let block_number = block.number.as_u64();
        tracing::debug!("reacting to standalone block {}", block_number);
        context.finish_epoch_if_needed(block_number, broker).await?;

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
        tracing::trace!(
            "Last input sent to advance-runner: `{}`; current input: `{}`",
            context.inputs_sent(),
            dapp_input_box.inputs.len()
        );

        let input_slice =
            dapp_input_box.inputs.skip(context.inputs_sent() as usize);

        for input in input_slice {
            context.enqueue_input(&input, broker).await?;
        }

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use eth_state_fold_types::ethereum_types::H160;
    use rollups_events::DAppMetadata;
    use types::foldables::InputBox;

    use crate::{
        drivers::{
            machine::MachineDriver,
            mock::{self, Broker},
            Context,
        },
        machine::RollupStatus,
        metrics::DispatcherMetrics,
    };

    fn new_context(genesis_block: u64, epoch_length: u64) -> Context {
        let context = Context::new(
            genesis_block,
            epoch_length,
            DAppMetadata::default(),
            DispatcherMetrics::default(),
        );
        context
    }

    fn new_broker(context: &Context) -> Broker {
        mock::Broker::new(
            vec![RollupStatus {
                inputs_sent_count: context.inputs_sent(),
            }],
            Vec::new(),
        )
    }

    // --------------------------------------------------------------------------------------------
    // process_inputs
    // --------------------------------------------------------------------------------------------

    async fn test_process_inputs(
        mut context: Context,
        broker: Broker,
        input_blocks: Vec<u64>,
        expected: Vec<mock::Event>,
    ) {
        let machine_driver = MachineDriver::new(H160::random());
        let dapp_input_box = types::foldables::DAppInputBox {
            inputs: input_blocks
                .iter()
                .map(|block| Arc::new(mock::new_input(*block)))
                .collect::<Vec<_>>()
                .into(),
        };
        let result = machine_driver
            .process_inputs(&mut context, &dapp_input_box, &broker)
            .await;
        assert!(result.is_ok());

        broker.assert_state(expected);
    }

    #[tokio::test]
    async fn process_inputs_without_skipping_inputs() {
        let context = new_context(0, 10);
        let broker = new_broker(&context);
        let input_blocks = vec![0, 1, 2, 3];
        let expected = vec![
            mock::Event::Input(0),
            mock::Event::Input(1),
            mock::Event::Input(2),
            mock::Event::Input(3),
        ];
        test_process_inputs(context, broker, input_blocks, expected).await;
    }

    #[tokio::test]
    async fn process_inputs_with_some_skipped_inputs() {
        let mut context = new_context(0, 10);
        let mut throwaway_broker = new_broker(&context);
        for i in 0..=1 {
            assert!(context
                .enqueue_input(&mock::new_input(i), &mut throwaway_broker)
                .await
                .is_ok());
        }
        assert_eq!(2, context.inputs_sent());

        let broker = new_broker(&context);
        let input_blocks = vec![0, 1, 2, 3];
        let expected = vec![mock::Event::Input(2), mock::Event::Input(3)];
        test_process_inputs(context, broker, input_blocks, expected).await;
    }

    #[tokio::test]
    async fn process_inputs_skipping_all_inputs() {
        let mut context = new_context(0, 10);
        let mut throwaway_broker = new_broker(&context);
        for i in 0..=3 {
            assert!(context
                .enqueue_input(&mock::new_input(i), &mut throwaway_broker)
                .await
                .is_ok());
        }
        assert_eq!(4, context.inputs_sent());

        let broker = new_broker(&context);
        let input_blocks = vec![0, 1, 2, 3];
        let expected = vec![];
        test_process_inputs(context, broker, input_blocks, expected).await;
    }

    // --------------------------------------------------------------------------------------------
    // react
    // --------------------------------------------------------------------------------------------

    async fn test_react(
        block: u64,
        mut context: Context,
        broker: Option<Broker>,
        input_box: Option<InputBox>,
        input_blocks: Vec<u64>,
        expected: Vec<mock::Event>,
    ) -> (Context, Broker, InputBox) {
        let rollup_status = RollupStatus {
            inputs_sent_count: context.inputs_sent(),
        };
        let broker = broker
            .unwrap_or(mock::Broker::new(vec![rollup_status], Vec::new()));
        let dapp_address = H160::random();
        let machine_driver = MachineDriver::new(dapp_address);

        let input_box = input_box.unwrap_or(mock::new_input_box());
        let input_box =
            mock::update_input_box(input_box, dapp_address, input_blocks);

        let result = machine_driver
            .react(&mut context, &mock::new_block(block), &input_box, &broker)
            .await;
        assert!(result.is_ok());

        broker.assert_state(expected);

        (context, broker, input_box)
    }

    #[tokio::test]
    async fn react_without_finish_epoch() {
        let block = 3;
        let context = new_context(0, 10);
        let input_blocks = vec![1, 2];
        let expected = vec![mock::Event::Input(0), mock::Event::Input(1)];
        test_react(block, context, None, None, input_blocks, expected).await;
    }

    #[tokio::test]
    async fn react_with_finish_epoch() {
        let block = 10;
        let context = new_context(0, 10);
        let input_blocks = vec![1, 2];
        let expected = vec![
            mock::Event::Input(0),
            mock::Event::Input(1),
            mock::Event::FinishEpoch(0),
        ];
        test_react(block, context, None, None, input_blocks, expected).await;
    }

    #[tokio::test]
    async fn react_with_internal_finish_epoch() {
        let block = 14;
        let context = new_context(0, 10);
        let input_blocks = vec![9, 10];
        let expected = vec![
            mock::Event::Input(0),
            mock::Event::FinishEpoch(0),
            mock::Event::Input(1),
        ];
        test_react(block, context, None, None, input_blocks, expected).await;
    }

    #[tokio::test]
    async fn react_without_inputs() {
        let block = 10;
        let context = new_context(0, 10);
        let input_blocks = vec![];
        let expected = vec![];
        test_react(block, context, None, None, input_blocks, expected).await;
    }

    // NOTE: this test shows we DON'T close the epoch after the first input!
    #[tokio::test]
    async fn react_with_inputs_after_first_epoch_length() {
        let block = 20;
        let context = new_context(0, 10);
        let input_blocks = vec![14, 16, 18, 20];
        let expected = vec![
            mock::Event::Input(0),
            mock::Event::Input(1),
            mock::Event::Input(2),
            mock::Event::FinishEpoch(0),
            mock::Event::Input(3),
        ];
        test_react(block, context, None, None, input_blocks, expected).await;
    }

    #[tokio::test]
    async fn react_is_deterministic() {
        let final_expected = vec![
            mock::Event::Input(0),
            mock::Event::FinishEpoch(0),
            mock::Event::Input(1),
            mock::Event::Input(2),
            mock::Event::Input(3),
            mock::Event::Input(4),
            mock::Event::Input(5),
            mock::Event::Input(6),
            mock::Event::Input(7),
            mock::Event::Input(8),
            mock::Event::Input(9),
            mock::Event::FinishEpoch(1),
            mock::Event::Input(10),
            mock::Event::Input(11),
            mock::Event::Input(12),
            mock::Event::Input(13),
            mock::Event::Input(14),
            mock::Event::Input(15),
            mock::Event::FinishEpoch(2),
            mock::Event::Input(16),
            mock::Event::Input(17),
            mock::Event::Input(18),
        ];

        {
            // original
            let block1 = 3100;
            let block2 = 6944;

            let context = new_context(0, 1000);

            let input_blocks1 = vec![
                56, //
                //
                1078, //
                1091, //
                1159, //
                1204, //
                1227, //
                1280, //
                1298, //
                1442, //
                1637, //
                //
                2827, //
                2881, //
                2883, //
                2887, //
                2891, //
                2934, //
            ];
            let mut input_blocks2 = input_blocks1.clone();
            input_blocks2.append(&mut vec![
                6160, //
                6864, //
                6944, //
            ]);

            let expected1 = vec![
                mock::Event::Input(0),
                mock::Event::FinishEpoch(0),
                mock::Event::Input(1),
                mock::Event::Input(2),
                mock::Event::Input(3),
                mock::Event::Input(4),
                mock::Event::Input(5),
                mock::Event::Input(6),
                mock::Event::Input(7),
                mock::Event::Input(8),
                mock::Event::Input(9),
                mock::Event::FinishEpoch(1),
                mock::Event::Input(10),
                mock::Event::Input(11),
                mock::Event::Input(12),
                mock::Event::Input(13),
                mock::Event::Input(14),
                mock::Event::Input(15),
                mock::Event::FinishEpoch(2),
            ];

            let (context, broker, input_box) = test_react(
                block1,
                context,
                None,
                None,
                input_blocks1,
                expected1,
            )
            .await;

            test_react(
                block2,
                context,
                Some(broker),
                Some(input_box),
                input_blocks2,
                final_expected.clone(),
            )
            .await;
        }

        {
            // reconstruction
            let block = 6944;
            let context = new_context(0, 1000);
            let input_blocks = vec![
                56, //
                //
                1078, //
                1091, //
                1159, //
                1204, //
                1227, //
                1280, //
                1298, //
                1442, //
                1637, //
                //
                2827, //
                2881, //
                2883, //
                2887, //
                2891, //
                2934, //
                //
                6160, //
                6864, //
                6944, //
            ];
            test_react(
                block,
                context,
                None,
                None,
                input_blocks,
                final_expected,
            )
            .await;
        }
    }
}
