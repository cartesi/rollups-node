// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::{
    checker::DuplicateChecker, repository::Repository,
    sender::TransactionSender,
};
use async_trait::async_trait;
use snafu::ResultExt;
use std::fmt::Debug;
use tracing::{info, trace};

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
pub enum ClaimerError<R: Repository, D: DuplicateChecker, T: TransactionSender>
{
    #[snafu(display("repository error"))]
    Repository { source: R::Error },

    #[snafu(display("duplicated claim error"))]
    DuplicatedClaim { source: D::Error },

    #[snafu(display("transaction sender error"))]
    TransactionSender { source: T::Error },
}

// ------------------------------------------------------------------------------------------------
// DefaultClaimer
// ------------------------------------------------------------------------------------------------

/// The `DefaultClaimer` must be injected with a
/// `BrokerListener`, a `DuplicateChecker` and a `TransactionSender`.
#[derive(Debug)]
pub struct DefaultClaimer<
    R: Repository,
    D: DuplicateChecker,
    T: TransactionSender,
> {
    repository: R,
    duplicate_checker: D,
    transaction_sender: T,
}

impl<R: Repository, D: DuplicateChecker, T: TransactionSender>
    DefaultClaimer<R, D, T>
{
    pub fn new(
        repository: R,
        duplicate_checker: D,
        transaction_sender: T,
    ) -> Self {
        Self {
            repository,
            duplicate_checker,
            transaction_sender,
        }
    }
}

#[async_trait]
impl<R, D, T> Claimer for DefaultClaimer<R, D, T>
where
    R: Repository + Send + Sync + 'static,
    D: DuplicateChecker + Send + Sync + 'static,
    T: TransactionSender + Send + 'static,
{
    type Error = ClaimerError<R, D, T>;

    async fn start(mut self) -> Result<(), Self::Error> {
        trace!("Starting the authority claimer loop");
        loop {
            let rollups_claim =
                self.repository.get_claim().await.context(RepositorySnafu)?;
            trace!("Got a claim from the repository: {:?}", rollups_claim);

            let is_duplicated_rollups_claim = self
                .duplicate_checker
                .is_duplicated_rollups_claim(&rollups_claim)
                .await
                .context(DuplicatedClaimSnafu)?;
            if is_duplicated_rollups_claim {
                trace!("It was a duplicated claim");
                continue;
            }

            info!("Sending a new rollups claim");
            self.transaction_sender = self
                .transaction_sender
                .send_rollups_claim_transaction(rollups_claim.clone())
                .await
                .context(TransactionSenderSnafu)?;

            info!("Updating claim data in repository");
            self.repository
                .update_claim(rollups_claim.dapp_address)
                .await
                .context(RepositorySnafu)?;
        }
    }
}
