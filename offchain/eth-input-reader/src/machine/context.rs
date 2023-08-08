// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::{
    machine::{rollups_broker::BrokerFacadeError, BrokerSend, BrokerStatus},
    metrics::EthInputReaderMetrics,
};

use rollups_events::DAppMetadata;
use types::foldables::input_box::Input;

#[derive(Debug)]
pub struct Context {
    inputs_sent_count: u64,
    last_event_is_finish_epoch: bool,
    last_timestamp: u64,

    // constants
    genesis_timestamp: u64,
    epoch_length: u64,

    dapp_metadata: DAppMetadata,
    metrics: EthInputReaderMetrics,
}

impl Context {
    pub async fn new(
        genesis_timestamp: u64,
        epoch_length: u64,
        broker: &impl BrokerStatus,
        dapp_metadata: DAppMetadata,
        metrics: EthInputReaderMetrics,
    ) -> Result<Self, BrokerFacadeError> {
        let status = broker.status().await?;

        Ok(Self {
            inputs_sent_count: status.inputs_sent_count,
            last_event_is_finish_epoch: status.last_event_is_finish_epoch,
            last_timestamp: genesis_timestamp,
            genesis_timestamp,
            epoch_length,
            dapp_metadata,
            metrics,
        })
    }

    pub fn inputs_sent_count(&self) -> u64 {
        self.inputs_sent_count
    }

    pub async fn finish_epoch_if_needed(
        &mut self,
        event_timestamp: u64,
        broker: &impl BrokerSend,
    ) -> Result<(), BrokerFacadeError> {
        if self.should_finish_epoch(event_timestamp) {
            self.finish_epoch(event_timestamp, broker).await?;
        }
        Ok(())
    }

    pub async fn enqueue_input(
        &mut self,
        input: &Input,
        broker: &impl BrokerSend,
    ) -> Result<(), BrokerFacadeError> {
        broker.enqueue_input(self.inputs_sent_count, input).await?;
        self.metrics
            .advance_inputs_sent
            .get_or_create(&self.dapp_metadata)
            .inc();
        self.inputs_sent_count += 1;
        self.last_event_is_finish_epoch = false;
        Ok(())
    }
}

impl Context {
    fn calculate_epoch(&self, timestamp: u64) -> u64 {
        assert!(timestamp >= self.genesis_timestamp);
        (timestamp - self.genesis_timestamp) / self.epoch_length
    }

    // This logic works because we call this function with `event_timestamp` being equal to the
    // timestamp of each individual input, rather than just the latest from the blockchain.
    fn should_finish_epoch(&self, event_timestamp: u64) -> bool {
        if self.inputs_sent_count == 0 || self.last_event_is_finish_epoch {
            false
        } else {
            let current_epoch = self.calculate_epoch(self.last_timestamp);
            let event_epoch = self.calculate_epoch(event_timestamp);
            event_epoch > current_epoch
        }
    }

    async fn finish_epoch(
        &mut self,
        event_timestamp: u64,
        broker: &impl BrokerSend,
    ) -> Result<(), BrokerFacadeError> {
        assert!(event_timestamp >= self.genesis_timestamp);
        broker.finish_epoch(self.inputs_sent_count).await?;
        self.metrics
            .finish_epochs_sent
            .get_or_create(&self.dapp_metadata)
            .inc();
        self.last_timestamp = event_timestamp;
        self.last_event_is_finish_epoch = true;
        Ok(())
    }
}

#[cfg(test)]
mod private_tests {
    use crate::machine::mock;
    use crate::metrics::EthInputReaderMetrics;

    use super::{Context, DAppMetadata};

    // --------------------------------------------------------------------------------------------
    // calculate_epoch_for
    // --------------------------------------------------------------------------------------------

    fn new_context_for_calculate_epoch_test(
        genesis_timestamp: u64,
        epoch_length: u64,
    ) -> Context {
        Context {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: false,
            last_timestamp: 0,
            genesis_timestamp,
            epoch_length,
            dapp_metadata: DAppMetadata::default(),
            metrics: EthInputReaderMetrics::default(),
        }
    }

    #[test]
    fn calculate_epoch_with_zero_genesis() {
        let epoch_length = 3;
        let context = new_context_for_calculate_epoch_test(0, epoch_length);
        let n = 10;
        let mut tested = 0;
        for epoch in 0..n {
            let x = epoch * epoch_length;
            let y = (epoch + 1) * epoch_length;
            for i in x..y {
                assert_eq!(context.calculate_epoch(i), epoch);
                tested += 1;
            }
        }
        assert_eq!(tested, n * epoch_length);
        assert_eq!(context.calculate_epoch(9), 3);
    }

    #[test]
    fn calculate_epoch_with_offset_genesis() {
        let context = new_context_for_calculate_epoch_test(2, 2);
        assert_eq!(context.calculate_epoch(2), 0);
        assert_eq!(context.calculate_epoch(3), 0);
        assert_eq!(context.calculate_epoch(4), 1);
        assert_eq!(context.calculate_epoch(5), 1);
        assert_eq!(context.calculate_epoch(6), 2);
    }

    #[test]
    #[should_panic]
    fn calculate_epoch_invalid() {
        new_context_for_calculate_epoch_test(4, 3).calculate_epoch(2);
    }

    // --------------------------------------------------------------------------------------------
    // should_finish_epoch
    // --------------------------------------------------------------------------------------------

    #[test]
    fn should_not_finish_epoch_because_of_time() {
        let context = Context {
            inputs_sent_count: 1,
            last_event_is_finish_epoch: false,
            last_timestamp: 3,
            genesis_timestamp: 0,
            epoch_length: 5,
            dapp_metadata: DAppMetadata::default(),
            metrics: EthInputReaderMetrics::default(),
        };
        assert!(!context.should_finish_epoch(4));
    }

    #[test]
    fn should_not_finish_epoch_because_of_zero_inputs() {
        let context = Context {
            inputs_sent_count: 1,
            last_event_is_finish_epoch: false,
            last_timestamp: 3,
            genesis_timestamp: 0,
            epoch_length: 5,
            dapp_metadata: DAppMetadata::default(),
            metrics: EthInputReaderMetrics::default(),
        };
        assert!(!context.should_finish_epoch(4));
    }

    #[test]
    fn should_finish_epoch_because_of_time() {
        let context = Context {
            inputs_sent_count: 1,
            last_event_is_finish_epoch: false,
            last_timestamp: 3,
            genesis_timestamp: 0,
            epoch_length: 5,
            dapp_metadata: DAppMetadata::default(),
            metrics: EthInputReaderMetrics::default(),
        };
        assert!(context.should_finish_epoch(5));
    }

    #[test]
    fn should_finish_epoch_because_last_event_is_finish_epoch() {
        let context = Context {
            inputs_sent_count: 1,
            last_event_is_finish_epoch: true,
            last_timestamp: 3,
            genesis_timestamp: 0,
            epoch_length: 5,
            dapp_metadata: DAppMetadata::default(),
            metrics: EthInputReaderMetrics::default(),
        };
        assert!(!context.should_finish_epoch(5));
    }

    // --------------------------------------------------------------------------------------------
    // finish_epoch
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn finish_epoch_ok() {
        let mut context = Context {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: false,
            last_timestamp: 3,
            genesis_timestamp: 0,
            epoch_length: 5,
            dapp_metadata: DAppMetadata::default(),
            metrics: EthInputReaderMetrics::default(),
        };
        let broker = mock::Broker::new(vec![], vec![]);
        let timestamp = 6;
        let result = context.finish_epoch(timestamp, &broker).await;
        assert!(result.is_ok());
        assert_eq!(context.last_timestamp, timestamp);
        assert!(context.last_event_is_finish_epoch);
    }

    #[tokio::test]
    #[should_panic]
    async fn finish_epoch_invalid() {
        let mut context = Context {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: false,
            last_timestamp: 6,
            genesis_timestamp: 5,
            epoch_length: 5,
            dapp_metadata: DAppMetadata::default(),
            metrics: EthInputReaderMetrics::default(),
        };
        let broker = mock::Broker::new(vec![], vec![]);
        let _ = context.finish_epoch(0, &broker).await;
    }

    #[tokio::test]
    async fn finish_epoch_broker_error() {
        let last_timestamp = 3;
        let last_event_is_finish_epoch = false;
        let mut context = Context {
            inputs_sent_count: 0,
            last_event_is_finish_epoch,
            last_timestamp,
            genesis_timestamp: 0,
            epoch_length: 5,
            dapp_metadata: DAppMetadata::default(),
            metrics: EthInputReaderMetrics::default(),
        };
        let broker = mock::Broker::with_finish_epoch_error();
        let result = context.finish_epoch(6, &broker).await;
        assert!(result.is_err());
        assert_eq!(context.last_timestamp, last_timestamp);
        assert_eq!(
            context.last_event_is_finish_epoch,
            last_event_is_finish_epoch
        );
    }
}

#[cfg(test)]
mod public_tests {
    use crate::{
        machine::{
            mock::{self, Broker, SendInteraction},
            RollupStatus,
        },
        metrics::EthInputReaderMetrics,
    };

    use super::{Context, DAppMetadata};

    // --------------------------------------------------------------------------------------------
    // new
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn new_ok() {
        let genesis_timestamp = 42;
        let epoch_length = 24;
        let inputs_sent_count = 150;
        let last_event_is_finish_epoch = true;
        let rollup_status = RollupStatus {
            inputs_sent_count,
            last_event_is_finish_epoch,
        };
        let broker = Broker::new(vec![rollup_status], vec![]);
        let result = Context::new(
            genesis_timestamp,
            epoch_length,
            &broker,
            DAppMetadata::default(),
            EthInputReaderMetrics::default(),
        )
        .await;
        assert!(result.is_ok());
        let context = result.unwrap();
        assert_eq!(context.genesis_timestamp, genesis_timestamp);
        assert_eq!(context.inputs_sent_count, inputs_sent_count);
        assert_eq!(
            context.last_event_is_finish_epoch,
            last_event_is_finish_epoch
        );
    }

    #[tokio::test]
    async fn new_broker_error() {
        let broker = Broker::with_status_error();
        let result = Context::new(
            1337,
            7331,
            &broker,
            DAppMetadata::default(),
            EthInputReaderMetrics::default(),
        )
        .await;
        assert!(result.is_err());
    }

    // --------------------------------------------------------------------------------------------
    // inputs_sent_count
    // --------------------------------------------------------------------------------------------

    #[test]
    fn inputs_sent_count() {
        let inputs_sent_count = 42;
        let context = Context {
            inputs_sent_count,
            last_event_is_finish_epoch: false, // ignored
            last_timestamp: 0,                 // ignored
            genesis_timestamp: 0,              // ignored
            epoch_length: 0,                   // ignored
            dapp_metadata: DAppMetadata::default(),
            metrics: EthInputReaderMetrics::default(),
        };
        assert_eq!(context.inputs_sent_count(), inputs_sent_count);
    }

    // --------------------------------------------------------------------------------------------
    // finish_epoch_if_needed
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn finish_epoch_if_needed_true() {
        let mut context = Context {
            inputs_sent_count: 1,
            last_event_is_finish_epoch: false,
            last_timestamp: 2,
            genesis_timestamp: 0,
            epoch_length: 4,
            dapp_metadata: DAppMetadata::default(),
            metrics: EthInputReaderMetrics::default(),
        };
        let broker = mock::Broker::new(vec![], vec![]);
        let result = context.finish_epoch_if_needed(4, &broker).await;
        assert!(result.is_ok());
        broker
            .assert_send_interactions(vec![SendInteraction::FinishedEpoch(1)]);
    }

    #[tokio::test]
    async fn finish_epoch_if_needed_false() {
        let mut context = Context {
            inputs_sent_count: 0,
            last_event_is_finish_epoch: false,
            last_timestamp: 2,
            genesis_timestamp: 0,
            epoch_length: 2,
            dapp_metadata: DAppMetadata::default(),
            metrics: EthInputReaderMetrics::default(),
        };
        let broker = mock::Broker::new(vec![], vec![]);
        let result = context.finish_epoch_if_needed(3, &broker).await;
        assert!(result.is_ok());
        broker.assert_send_interactions(vec![]);
    }

    #[tokio::test]
    async fn finish_epoch_if_needed_broker_error() {
        let mut context = Context {
            inputs_sent_count: 1,
            last_event_is_finish_epoch: false,
            last_timestamp: 2,
            genesis_timestamp: 0,
            epoch_length: 4,
            dapp_metadata: DAppMetadata::default(),
            metrics: EthInputReaderMetrics::default(),
        };
        let broker = mock::Broker::with_finish_epoch_error();
        let result = context.finish_epoch_if_needed(4, &broker).await;
        assert!(result.is_err());
    }

    // --------------------------------------------------------------------------------------------
    // enqueue_input
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn enqueue_input_ok() {
        let inputs_sent_count = 42;
        let mut context = Context {
            inputs_sent_count,
            last_event_is_finish_epoch: true,
            last_timestamp: 0,    // ignored
            genesis_timestamp: 0, // ignored
            epoch_length: 0,      // ignored
            dapp_metadata: DAppMetadata::default(),
            metrics: EthInputReaderMetrics::default(),
        };
        let input = mock::new_input(2);
        let broker = mock::Broker::new(vec![], vec![]);
        let result = context.enqueue_input(&input, &broker).await;
        assert!(result.is_ok());
        assert_eq!(context.inputs_sent_count, inputs_sent_count + 1);
        assert!(!context.last_event_is_finish_epoch);
        broker.assert_send_interactions(vec![SendInteraction::EnqueuedInput(
            inputs_sent_count,
        )]);
    }

    #[tokio::test]
    async fn enqueue_input_broker_error() {
        let mut context = Context {
            inputs_sent_count: 42,
            last_event_is_finish_epoch: true,
            last_timestamp: 0,    // ignored
            genesis_timestamp: 0, // ignored
            epoch_length: 0,      // ignored
            dapp_metadata: DAppMetadata::default(),
            metrics: EthInputReaderMetrics::default(),
        };
        let broker = mock::Broker::with_enqueue_input_error();
        let result = context.enqueue_input(&mock::new_input(2), &broker).await;
        assert!(result.is_err());
    }
}
