// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use rollups_events::{DAppMetadata, RollupsClaim};
use snafu::Snafu;
use std::fmt::Debug;

use crate::metrics::AuthorityClaimerMetrics;

/// The `TransactionSender` sends claims to the blockchain.
///
/// It should wait for N blockchain confirmations.
#[async_trait]
pub trait TransactionSender: Sized + Debug {
    type Error: snafu::Error + 'static;

    /// The `send_rollups_claim_transaction` function consumes the
    /// `TransactionSender` object and then returns it to avoid
    /// that processes use the transaction sender concurrently.
    async fn send_rollups_claim_transaction(
        self,
        rollups_claim: RollupsClaim,
    ) -> Result<Self, Self::Error>;
}

// ------------------------------------------------------------------------------------------------
// DefaultTransactionSender
// ------------------------------------------------------------------------------------------------

#[derive(Debug, Clone)]
pub struct DefaultTransactionSender;

#[derive(Debug, Snafu)]
pub enum TransactionSenderError {
    Todo,
}

impl DefaultTransactionSender {
    pub fn new(
        _dapp_metadata: DAppMetadata,
        _metrics: AuthorityClaimerMetrics,
    ) -> Result<Self, TransactionSenderError> {
        todo!()
    }
}

#[async_trait]
impl TransactionSender for DefaultTransactionSender {
    type Error = TransactionSenderError;

    async fn send_rollups_claim_transaction(
        self,
        _rollups_claim: RollupsClaim,
    ) -> Result<Self, Self::Error> {
        Err(TransactionSenderError::Todo)
    }
}
