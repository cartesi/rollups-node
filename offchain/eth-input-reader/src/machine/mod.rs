// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

pub mod context;
pub mod driver;
pub mod rollups_broker;

#[cfg(test)]
mod mock;

pub use context::Context;
pub use driver::MachineDriver;
use rollups_events::RollupsClaim;
use types::foldables::input_box::Input;

use async_trait::async_trait;

use self::rollups_broker::BrokerFacadeError;

#[derive(Debug, Default)]
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

#[async_trait]
pub trait BrokerReceive: std::fmt::Debug {
    async fn next_claim(
        &self,
    ) -> Result<Option<RollupsClaim>, BrokerFacadeError>;
}
