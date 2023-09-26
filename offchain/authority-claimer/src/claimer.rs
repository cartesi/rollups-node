// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use rollups_events::Address;
use snafu::ResultExt;
use std::fmt::Debug;
use tracing::{info, trace};

use crate::{
    checker::DuplicateChecker, listener::BrokerListener,
    sender::TransactionSender,
};

/// The `Claimer` starts an event loop that waits for claim messages
/// from the broker, and then sends the claims to the blockchain. It checks to
/// see if the claim is duplicated before sending.
///
/// It uses three injected traits, `BrokerListener`, `DuplicateChecker`, and
/// `TransactionSender`, to, respectivelly, listen for messages, check for
/// duplicated claims, and send claims to the blockchain.
#[async_trait]
pub trait Claimer: Sized + Debug {
    type Error: snafu::Error + 'static;

    async fn start(mut self) -> Result<(), Self::Error>;
}

#[derive(Debug, snafu::Snafu)]
pub enum ClaimerError<
    B: BrokerListener,
    D: DuplicateChecker,
    T: TransactionSender,
> {
    #[snafu(display("broker listener error"))]
    BrokerListenerError { source: B::Error },

    #[snafu(display("duplicated claim error"))]
    DuplicatedClaimError { source: D::Error },

    #[snafu(display("transaction sender error"))]
    TransactionSenderError { source: T::Error },
}

// ------------------------------------------------------------------------------------------------
// DefaultClaimer
// ------------------------------------------------------------------------------------------------

/// The `DefaultClaimer` must be injected with a
/// `BrokerListener`, a `DuplicateChecker` and a `TransactionSender`.
#[derive(Debug)]
pub struct DefaultClaimer<
    B: BrokerListener,
    D: DuplicateChecker,
    T: TransactionSender,
> {
    dapp_address: Address,
    broker_listener: B,
    duplicate_checker: D,
    transaction_sender: T,
}

impl<B: BrokerListener, D: DuplicateChecker, T: TransactionSender>
    DefaultClaimer<B, D, T>
{
    pub fn new(
        dapp_address: Address,
        broker_listener: B,
        duplicate_checker: D,
        transaction_sender: T,
    ) -> Self {
        Self {
            dapp_address,
            broker_listener,
            duplicate_checker,
            transaction_sender,
        }
    }
}

#[async_trait]
impl<B, D, T> Claimer for DefaultClaimer<B, D, T>
where
    B: BrokerListener + Send + Sync + 'static,
    D: DuplicateChecker + Send + Sync + 'static,
    T: TransactionSender + Send + 'static,
{
    type Error = ClaimerError<B, D, T>;

    async fn start(mut self) -> Result<(), Self::Error> {
        trace!("Starting the authority claimer loop");
        loop {
            let rollups_claim = self
                .broker_listener
                .listen()
                .await
                .context(BrokerListenerSnafu)?;
            trace!("Got a claim from the broker: {:?}", rollups_claim);

            let is_duplicated_rollups_claim = self
                .duplicate_checker
                .is_duplicated_rollups_claim(
                    self.dapp_address.clone(),
                    &rollups_claim,
                )
                .await
                .context(DuplicatedClaimSnafu)?;
            if is_duplicated_rollups_claim {
                trace!("It was a duplicated claim");
                continue;
            }

            info!("Sending a new rollups claim");
            self.transaction_sender = self
                .transaction_sender
                .send_rollups_claim_transaction(
                    self.dapp_address.clone(),
                    rollups_claim,
                )
                .await
                .context(TransactionSenderSnafu)?
        }
    }
}
