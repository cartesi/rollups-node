// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use rollups_events::{BrokerConfig, DAppMetadata, RollupsClaim};
use snafu::Snafu;
use std::fmt::Debug;

use crate::metrics::AuthorityClaimerMetrics;

/// The `BrokerListener` listens for new claims from the broker.
///
/// The `listen` function should preferably yield to other processes while
/// waiting for new messages (instead of busy-waiting).
#[async_trait]
pub trait BrokerListener: Debug {
    type Error: snafu::Error;

    async fn listen(&self) -> Result<RollupsClaim, Self::Error>;
}

// ------------------------------------------------------------------------------------------------
// DefaultBrokerListener
// ------------------------------------------------------------------------------------------------

#[derive(Debug)]
pub struct DefaultBrokerListener;

#[derive(Debug, Snafu)]
pub enum DefaultBrokerListenerError {
    Todo,
}

impl DefaultBrokerListener {
    pub fn new(
        _broker_config: BrokerConfig,
        _dapp_metadata: DAppMetadata,
        _metrics: AuthorityClaimerMetrics,
    ) -> Result<Self, DefaultBrokerListenerError> {
        todo!()
    }
}

#[async_trait]
impl BrokerListener for DefaultBrokerListener {
    type Error = DefaultBrokerListenerError;

    async fn listen(&self) -> Result<RollupsClaim, Self::Error> {
        todo!()
    }
}

// impl Stream for BrokerListener {
//     type Item = u32;
//
//     fn poll_next(
//         self: Pin<&mut Self>,
//         cx: &mut Context<'_>,
//     ) -> Poll<Option<Self::Item>> {
//         todo!()
//     }
// }
