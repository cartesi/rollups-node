// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::{
    machine::{rollups_broker::BrokerFacadeError, BrokerSend},
    metrics::DispatcherMetrics,
};

use rollups_events::DAppMetadata;
use types::foldables::Input;

#[derive(Debug)]
pub struct Context {
    inputs_sent: u64,
    last_input_epoch: Option<u64>,
    last_finished_epoch: Option<u64>,

    // constants
    genesis_block: u64,
    epoch_length: u64,

    // metrics
    dapp_metadata: DAppMetadata,
    metrics: DispatcherMetrics,
}

impl Context {
    pub fn new(
        genesis_block: u64,
        epoch_length: u64,
        dapp_metadata: DAppMetadata,
        metrics: DispatcherMetrics,
        inputs_sent: u64,
        last_input_epoch: Option<u64>,
        last_finished_epoch: Option<u64>,
    ) -> Self {
        assert!(epoch_length > 0);
        Self {
            inputs_sent,
            last_input_epoch,
            last_finished_epoch,
            genesis_block,
            epoch_length,
            dapp_metadata,
            metrics,
        }
    }

    pub fn inputs_sent(&self) -> u64 {
        self.inputs_sent
    }

    pub async fn finish_epoch_if_needed(
        &mut self,
        block: u64,
        broker: &impl BrokerSend,
    ) -> Result<(), BrokerFacadeError> {
        let epoch = self.calculate_epoch(block);
        if self.should_finish_epoch(epoch) {
            self.finish_epoch(broker).await?;
        }
        Ok(())
    }

    pub async fn enqueue_input(
        &mut self,
        input: &Input,
        broker: &impl BrokerSend,
    ) -> Result<(), BrokerFacadeError> {
        let input_block = input.block_added.number.as_u64();
        self.finish_epoch_if_needed(input_block, broker).await?;

        broker.enqueue_input(self.inputs_sent, input).await?;

        self.metrics
            .advance_inputs_sent
            .get_or_create(&self.dapp_metadata)
            .inc();

        self.inputs_sent += 1;
        self.last_input_epoch =
            Some(self.calculate_epoch(input.block_added.number.as_u64()));

        Ok(())
    }
}

impl Context {
    fn calculate_epoch(&self, block: u64) -> u64 {
        assert!(block >= self.genesis_block);
        (block - self.genesis_block) / self.epoch_length
    }

    fn should_finish_epoch(&self, epoch: u64) -> bool {
        match (self.last_input_epoch, self.last_finished_epoch) {
            (Some(input), Some(finished)) => assert!(input >= finished),
            _ => (),
        }

        if self.last_finished_epoch == self.last_input_epoch {
            return false; // if the current epoch is empty
        }

        if epoch == self.last_input_epoch.unwrap() {
            return false; // if the current epoch is still not over
        }

        epoch > self.last_finished_epoch.unwrap_or(0)
    }

    async fn finish_epoch(
        &mut self,
        broker: &impl BrokerSend,
    ) -> Result<(), BrokerFacadeError> {
        broker.finish_epoch(self.inputs_sent).await?;
        self.metrics
            .finish_epochs_sent
            .get_or_create(&self.dapp_metadata)
            .inc();

        self.last_finished_epoch = self.last_input_epoch;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::collections::VecDeque;

    use crate::drivers::mock::Event;
    use rollups_events::DAppMetadata;
    use serial_test::serial;

    use crate::{drivers::mock, metrics::DispatcherMetrics};

    use super::Context;

    impl Default for Context {
        fn default() -> Self {
            Context::new(
                /* genesis_block */ 0,
                /* epoch_length */ 10,
                /* dapp_metadata */ DAppMetadata::default(),
                /* metrics */ DispatcherMetrics::default(),
                /* number_of_inputs_sent */ 0,
                /* last_input_epoch */ None,
                /* last_finished_epoch */ None,
            )
        }
    }

    // --------------------------------------------------------------------------------------------
    // calculate_epoch
    // --------------------------------------------------------------------------------------------

    #[test]
    fn calculate_epoch_with_zero_genesis() {
        let mut context = Context::default();
        context.genesis_block = 0;
        context.epoch_length = 10;

        let number_of_epochs = 10;
        let mut tested = 0;
        for current_epoch in 0..number_of_epochs {
            let block_lower_bound = current_epoch * context.epoch_length;
            let block_upper_bound = (current_epoch + 1) * context.epoch_length;
            for i in block_lower_bound..block_upper_bound {
                assert_eq!(context.calculate_epoch(i), current_epoch);
                tested += 1;
            }
        }

        assert_eq!(tested, number_of_epochs * context.epoch_length);
        assert_eq!(
            context.calculate_epoch(context.epoch_length * number_of_epochs),
            context.epoch_length
        );
    }

    #[test]
    fn calculate_epoch_with_offset_genesis() {
        let mut context = Context::default();
        context.genesis_block = 2;
        context.epoch_length = 2;

        assert_eq!(context.calculate_epoch(2), 0);
        assert_eq!(context.calculate_epoch(3), 0);
        assert_eq!(context.calculate_epoch(4), 1);
        assert_eq!(context.calculate_epoch(5), 1);
        assert_eq!(context.calculate_epoch(6), 2);
    }

    #[test]
    #[should_panic]
    fn calculate_epoch_should_panic_because_block_came_before_genesis() {
        let mut context = Context::default();
        context.genesis_block = 4;
        context.epoch_length = 4;
        context.calculate_epoch(2);
    }

    // --------------------------------------------------------------------------------------------
    // should_finish_epoch -- first epoch
    // --------------------------------------------------------------------------------------------

    #[test]
    fn should_finish_the_first_epoch() {
        let mut context = Context::default();
        context.inputs_sent = 1;
        context.last_input_epoch = Some(0);
        context.last_finished_epoch = None;
        let epoch = context.calculate_epoch(10);
        assert_eq!(context.should_finish_epoch(epoch), true);
    }

    #[test]
    fn should_finish_the_first_epoch_after_several_blocks() {
        let mut context = Context::default();
        context.inputs_sent = 110;
        context.last_input_epoch = Some(9);
        context.last_finished_epoch = None;
        let epoch = context.calculate_epoch(100);
        assert_eq!(context.should_finish_epoch(epoch), true);
    }

    #[test]
    fn should_not_finish_an_empty_first_epoch() {
        let mut context = Context::default();
        context.inputs_sent = 0;
        context.last_input_epoch = None;
        context.last_finished_epoch = None;
        let epoch = context.calculate_epoch(10);
        assert_eq!(context.should_finish_epoch(epoch), false);
    }

    #[test]
    fn should_not_finish_a_very_late_empty_first_epoch() {
        let mut context = Context::default();
        context.inputs_sent = 0;
        context.last_input_epoch = None;
        context.last_finished_epoch = None;
        let epoch = context.calculate_epoch(2340);
        assert_eq!(context.should_finish_epoch(epoch), false);
    }

    #[test]
    fn should_not_finish_a_timely_first_epoch() {
        let mut context = Context::default();
        context.inputs_sent = 1;
        context.last_input_epoch = Some(0);
        context.last_finished_epoch = None;
        let epoch = context.calculate_epoch(9);
        assert_eq!(context.should_finish_epoch(epoch), false);
    }

    // --------------------------------------------------------------------------------------------
    // should_finish_epoch -- other epochs
    // --------------------------------------------------------------------------------------------

    #[test]
    fn should_finish_epoch() {
        let mut context = Context::default();
        context.inputs_sent = 42;
        context.last_input_epoch = Some(4);
        context.last_finished_epoch = Some(3);
        let epoch = context.calculate_epoch(54);
        assert_eq!(context.should_finish_epoch(epoch), true);
    }

    #[test]
    fn should_finish_epoch_by_a_lot() {
        let mut context = Context::default();
        context.inputs_sent = 142;
        context.last_input_epoch = Some(15);
        context.last_finished_epoch = Some(2);
        let epoch = context.calculate_epoch(190);
        assert_eq!(context.should_finish_epoch(epoch), true);
    }

    #[test]
    fn should_not_finish_an_empty_epoch() {
        let mut context = Context::default();
        context.inputs_sent = 120;
        context.last_input_epoch = Some(9);
        context.last_finished_epoch = Some(9);
        let epoch = context.calculate_epoch(105);
        assert_eq!(context.should_finish_epoch(epoch), false);
    }

    #[test]
    fn should_not_finish_a_very_late_empty_epoch() {
        let mut context = Context::default();
        context.inputs_sent = 120;
        context.last_input_epoch = Some(15);
        context.last_finished_epoch = Some(15);
        let epoch = context.calculate_epoch(1000);
        assert_eq!(context.should_finish_epoch(epoch), false);
    }

    #[test]
    fn should_not_finish_a_timely_epoch() {
        let mut context = Context::default();
        context.inputs_sent = 230;
        context.last_input_epoch = Some(11);
        context.last_finished_epoch = Some(10);
        let epoch = context.calculate_epoch(110);
        assert_eq!(context.should_finish_epoch(epoch), false);
    }

    // --------------------------------------------------------------------------------------------
    // finish_epoch
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn finish_epoch_ok() {
        let mut context = Context::default();
        context.inputs_sent = 1;
        context.last_input_epoch = Some(0);
        context.last_finished_epoch = None;

        let broker = mock::Broker::new(vec![], vec![]);
        let result = context.finish_epoch(&broker).await;
        assert!(result.is_ok());
        assert_eq!(context.inputs_sent, 1);
        assert_eq!(context.last_input_epoch, Some(0));
        assert_eq!(context.last_finished_epoch, Some(0));
    }

    #[tokio::test]
    async fn finish_epoch_broker_error() {
        let mut context = Context::default();
        let broker = mock::Broker::with_finish_epoch_error();
        let result = context.finish_epoch(&broker).await;
        assert!(result.is_err());
        assert_eq!(context.inputs_sent, 0);
        assert_eq!(context.last_input_epoch, None);
        assert_eq!(context.last_finished_epoch, None);
    }

    // --------------------------------------------------------------------------------------------
    // new
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn new_ok() {
        let genesis_block = 42;
        let epoch_length = 24;
        let number_of_inputs_sent = 150;
        let last_input_epoch = Some(14);
        let last_finished_epoch = Some(37);

        let context = Context::new(
            genesis_block,
            epoch_length,
            DAppMetadata::default(),
            DispatcherMetrics::default(),
            number_of_inputs_sent,
            last_input_epoch,
            last_finished_epoch,
        );

        assert_eq!(context.genesis_block, genesis_block);
        assert_eq!(context.epoch_length, epoch_length);
        assert_eq!(context.dapp_metadata, DAppMetadata::default());
        assert_eq!(context.inputs_sent, number_of_inputs_sent);
        assert_eq!(context.last_input_epoch, last_input_epoch);
        assert_eq!(context.last_finished_epoch, last_finished_epoch);
    }

    #[test]
    #[should_panic]
    fn new_should_panic_because_epoch_length_is_zero() {
        Context::new(
            0,
            0,
            DAppMetadata::default(),
            DispatcherMetrics::default(),
            0,
            None,
            None,
        );
    }

    // --------------------------------------------------------------------------------------------
    // inputs_sent_count
    // --------------------------------------------------------------------------------------------

    #[test]
    fn inputs_sent_count() {
        let number_of_inputs_sent = 42;
        let mut context = Context::default();
        context.inputs_sent = number_of_inputs_sent;
        assert_eq!(context.inputs_sent(), number_of_inputs_sent);
    }

    // --------------------------------------------------------------------------------------------
    // finish_epoch_if_needed
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn finish_epoch_if_needed_true() {
        let mut context = Context::default();
        context.inputs_sent = 9;
        context.last_input_epoch = Some(0);
        context.last_finished_epoch = None;

        let broker = mock::Broker::new(vec![], vec![]);
        let result = context.finish_epoch_if_needed(12, &broker).await;
        assert!(result.is_ok());
        broker.assert_state(vec![
            Event::Finish, //
        ]);
    }

    #[tokio::test]
    async fn finish_epoch_if_needed_false() {
        let mut context = Context::default();
        context.inputs_sent = 9;
        context.last_input_epoch = Some(0);
        context.last_finished_epoch = None;

        let broker = mock::Broker::new(vec![], vec![]);
        let result = context.finish_epoch_if_needed(9, &broker).await;
        assert!(result.is_ok());
        broker.assert_state(vec![]);
    }

    #[tokio::test]
    async fn finish_epoch_if_needed_broker_error() {
        let mut context = Context::default();
        context.inputs_sent = 9;
        context.last_input_epoch = Some(0);
        context.last_finished_epoch = None;
        let broker = mock::Broker::with_finish_epoch_error();
        let result = context.finish_epoch_if_needed(28, &broker).await;
        assert!(result.is_err());
    }

    // --------------------------------------------------------------------------------------------
    // enqueue_input
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn enqueue_input_ok() {
        let number_of_inputs_sent = 42;
        let last_input_epoch = Some(1);
        let last_finished_epoch = None;

        let mut context = Context::default();
        context.inputs_sent = number_of_inputs_sent;
        context.last_input_epoch = last_input_epoch;
        context.last_finished_epoch = last_finished_epoch;

        let input = mock::new_input(22);
        let broker = mock::Broker::new(vec![], vec![]);
        let result = context.enqueue_input(&input, &broker).await;
        assert!(result.is_ok());

        assert_eq!(context.inputs_sent, number_of_inputs_sent + 1);
        assert_eq!(context.last_input_epoch, Some(2));
        assert_eq!(context.last_finished_epoch, Some(1));

        broker.assert_state(vec![
            Event::Finish,
            Event::Input(number_of_inputs_sent),
        ]);
    }

    #[tokio::test]
    async fn enqueue_input_broker_error() {
        let mut context = Context::default();
        let broker = mock::Broker::with_enqueue_input_error();
        let result = context.enqueue_input(&mock::new_input(82), &broker).await;
        assert!(result.is_err());
    }

    // --------------------------------------------------------------------------------------------
    // deterministic behavior
    // --------------------------------------------------------------------------------------------

    #[derive(Clone)]
    struct Case {
        input_blocks: Vec<u64>,
        epoch_length: u64,
        last_block: u64,
        expected: Vec<Event>,
    }

    #[tokio::test]
    #[serial]
    async fn deterministic_behavior() {
        let cases: Vec<Case> = vec![
            Case {
                input_blocks: vec![],
                epoch_length: 2,
                last_block: 100,
                expected: vec![],
            },
            Case {
                input_blocks: vec![0, 1, 4, 5],
                epoch_length: 2,
                last_block: 10,
                expected: vec![
                    Event::Input(0),
                    Event::Input(1),
                    Event::Finish,
                    Event::Input(2),
                    Event::Input(3),
                    Event::Finish,
                ],
            },
            Case {
                input_blocks: vec![0, 0, 0, 7, 7],
                epoch_length: 2,
                last_block: 10,
                expected: vec![
                    Event::Input(0),
                    Event::Input(1),
                    Event::Input(2),
                    Event::Finish,
                    Event::Input(3),
                    Event::Input(4),
                    Event::Finish,
                ],
            },
            Case {
                input_blocks: vec![0, 2],
                epoch_length: 2,
                last_block: 4,
                expected: vec![
                    Event::Input(0),
                    Event::Finish,
                    Event::Input(1),
                    Event::Finish,
                ],
            },
            Case {
                input_blocks: vec![1, 2, 4],
                epoch_length: 2,
                last_block: 6,
                expected: vec![
                    Event::Input(0),
                    Event::Finish,
                    Event::Input(1),
                    Event::Finish,
                    Event::Input(2),
                    Event::Finish,
                ],
            },
            Case {
                input_blocks: vec![0, 1, 1, 2, 3, 4, 5, 5, 5, 6, 7],
                epoch_length: 2,
                last_block: 7,
                expected: vec![
                    Event::Input(0),
                    Event::Input(1),
                    Event::Input(2),
                    Event::Finish,
                    Event::Input(3),
                    Event::Input(4),
                    Event::Finish,
                    Event::Input(5),
                    Event::Input(6),
                    Event::Input(7),
                    Event::Input(8),
                    Event::Finish,
                    Event::Input(9),
                    Event::Input(10),
                ],
            },
            Case {
                input_blocks: vec![0, 5, 9],
                epoch_length: 2,
                last_block: 10,
                expected: vec![
                    Event::Input(0),
                    Event::Finish,
                    Event::Input(1),
                    Event::Finish,
                    Event::Input(2),
                    Event::Finish,
                ],
            },
        ];
        for (i, case) in cases.iter().enumerate() {
            println!("Testing case {}.", i);
            test_deterministic_case(case.clone()).await;
        }
    }

    // --------------------------------------------------------------------------------------------
    // auxiliary
    // --------------------------------------------------------------------------------------------

    async fn test_deterministic_case(case: Case) {
        let broker1 = create_state_as_inputs_are_being_received(
            case.epoch_length,
            case.input_blocks.clone(),
            case.last_block,
        )
        .await;
        let broker2 = create_state_by_receiving_all_inputs_at_once(
            case.epoch_length,
            case.input_blocks.clone(),
            case.last_block,
        )
        .await;
        broker1.assert_state(case.expected.clone());
        broker2.assert_state(case.expected.clone());
    }

    async fn create_state_as_inputs_are_being_received(
        epoch_length: u64,
        input_blocks: Vec<u64>,
        last_block: u64,
    ) -> mock::Broker {
        println!("================================================");
        println!("one_block_at_a_time:");

        let mut input_blocks: VecDeque<_> = input_blocks.into();
        let mut current_input_block = input_blocks.pop_front();

        let mut context = Context::default();
        context.epoch_length = epoch_length;
        let broker = mock::Broker::new(vec![], vec![]);

        for block in 0..=last_block {
            while let Some(input_block) = current_input_block {
                if block == input_block {
                    println!("\tenqueue_input(input_block: {})", block);
                    let input = mock::new_input(block);
                    let result = context.enqueue_input(&input, &broker).await;
                    assert!(result.is_ok());

                    current_input_block = input_blocks.pop_front();
                } else {
                    break;
                }
            }

            println!("\tfinish_epoch_if_needed(block: {})\n", block);
            let result = context.finish_epoch_if_needed(block, &broker).await;
            assert!(result.is_ok());
        }

        broker
    }

    async fn create_state_by_receiving_all_inputs_at_once(
        epoch_length: u64,
        input_blocks: Vec<u64>,
        last_block: u64,
    ) -> mock::Broker {
        println!("all_inputs_at_once:");

        let mut context = Context::default();
        context.epoch_length = epoch_length;
        let broker = mock::Broker::new(vec![], vec![]);

        for block in input_blocks {
            println!("\tenqueue_input(input_block: {})\n", block);
            let input = mock::new_input(block);
            let result = context.enqueue_input(&input, &broker).await;
            assert!(result.is_ok());
        }

        println!("\tfinish_epoch_if_needed(last_block: {})", last_block);
        let result = context.finish_epoch_if_needed(last_block, &broker).await;
        assert!(result.is_ok());

        println!("================================================");

        broker
    }
}
