// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use snafu::ResultExt;
use std::{collections::HashMap, fmt::Debug};
use tracing::{debug, info};

use rollups_events::Address;

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
    #[snafu(display("invalid app address {:?}", app_address))]
    InvalidAppAddress { app_address: Address },

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
    broker_listener: B,
    duplicate_checker: D,
    transaction_sender: T,
}

impl<B: BrokerListener, D: DuplicateChecker, T: TransactionSender>
    DefaultClaimer<B, D, T>
{
    pub fn new(
        broker_listener: B,
        duplicate_checker: D,
        transaction_sender: T,
    ) -> Self {
        Self {
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
        debug!("Starting the authority claimer loop");
        loop {
            let rollups_claim = self
                .broker_listener
                .listen()
                .await
                .context(BrokerListenerSnafu)?;
            debug!("Got a claim from the broker: {:?}", rollups_claim);

            let is_duplicated_rollups_claim = self
                .duplicate_checker
                .is_duplicated_rollups_claim(&rollups_claim)
                .await
                .context(DuplicatedClaimSnafu)?;
            if is_duplicated_rollups_claim {
                debug!("It was a duplicated claim");
                continue;
            }

            info!("Sending a new rollups claim");
            self.transaction_sender = self
                .transaction_sender
                .send_rollups_claim_transaction(rollups_claim)
                .await
                .context(TransactionSenderSnafu)?
        }
    }
}

// ------------------------------------------------------------------------------------------------
// MultidappClaimer
// ------------------------------------------------------------------------------------------------

/// The `MultidappClaimer` must be injected with a `BrokerListener`, a map of `Address` to
/// `DuplicateChecker`, and a `TransactionSender`.
#[derive(Debug)]
pub struct MultidappClaimer<
    B: BrokerListener,
    D: DuplicateChecker,
    T: TransactionSender,
> {
    broker_listener: B,
    duplicate_checkers: HashMap<Address, D>,
    transaction_sender: T,
}

impl<B: BrokerListener, D: DuplicateChecker, T: TransactionSender>
    MultidappClaimer<B, D, T>
{
    pub fn new(
        broker_listener: B,
        duplicate_checkers: HashMap<Address, D>,
        transaction_sender: T,
    ) -> Self {
        Self {
            broker_listener,
            duplicate_checkers,
            transaction_sender,
        }
    }
}

#[async_trait]
impl<B, D, T> Claimer for MultidappClaimer<B, D, T>
where
    B: BrokerListener + Send + Sync + 'static,
    D: DuplicateChecker + Send + Sync + 'static,
    T: TransactionSender + Send + 'static,
{
    type Error = ClaimerError<B, D, T>;

    async fn start(mut self) -> Result<(), Self::Error> {
        debug!("Starting the multidapp authority claimer loop");
        loop {
            // Listens for claims from multiple dapps.
            let rollups_claim = self
                .broker_listener
                .listen()
                .await
                .context(BrokerListenerSnafu)?;
            let dapp_address = rollups_claim.dapp_address.clone();
            debug!(
                "Got a claim from the broker for {:?}: {:?}",
                dapp_address, rollups_claim
            );

            // Gets the duplicate checker for the dapp.
            let duplicate_checker = self
                .duplicate_checkers
                .get_mut(&dapp_address)
                .ok_or(ClaimerError::InvalidAppAddress {
                    app_address: dapp_address.clone(),
                })?;

            // Checks for duplicates.
            let is_duplicated_rollups_claim = duplicate_checker
                .is_duplicated_rollups_claim(&rollups_claim)
                .await
                .context(DuplicatedClaimSnafu)?;

            // If it is a duplicate, the loop continues.
            if is_duplicated_rollups_claim {
                debug!("It was a duplicated claim");
                continue;
            }

            // Sends the claim.
            info!("Sending a new rollups claim");
            self.transaction_sender = self
                .transaction_sender
                .send_rollups_claim_transaction(rollups_claim)
                .await
                .context(TransactionSenderSnafu)?
        }
    }
}
