// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

pub mod rollups_broker;

use types::foldables::Input;

use async_trait::async_trait;

use self::rollups_broker::BrokerFacadeError;

#[derive(Debug, Clone, Copy, Default)]
pub struct RollupStatus {
    pub inputs_sent_count: u64,
    pub last_event_is_finish_epoch: bool,
}

#[async_trait]
pub trait BrokerStatus: std::fmt::Debug {
    async fn status(&self) -> Result<RollupStatus, BrokerFacadeError>;
}

#[async_trait]
pub trait BrokerSend: std::fmt::Debug {
    async fn enqueue_input(
        &self,
        input_index: u64,
        input: &Input,
    ) -> Result<(), BrokerFacadeError>;
    async fn finish_epoch(
        &self,
        inputs_sent_count: u64,
    ) -> Result<(), BrokerFacadeError>;
}
