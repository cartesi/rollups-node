// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use snafu::ResultExt;
use tracing::{info, trace};

use crate::{
    checker::DuplicateChecker, listener::BrokerListener,
    sender::TransactionSender,
};

/// The `AuthorityClaimer` starts an event loop that waits for claim messages
/// from the broker, and then sends the claims to the blockchain. It checks to
/// see if the claim is duplicated before sending.
///
/// It uses three injected traits, `BrokerListener`, `DuplicateChecker`, and
/// `TransactionSender`, to, respectivelly, listen for messages, check for
/// duplicated claims, and send claims to the blockchain.
#[async_trait]
pub trait AuthorityClaimer {
    async fn start<L, C, S>(
        &self,
        broker_listener: L,
        duplicate_checker: C,
        transaction_sender: S,
    ) -> Result<(), AuthorityClaimerError<L, C, S>>
    where
        L: BrokerListener + Send + Sync,
        C: DuplicateChecker + Send + Sync,
        S: TransactionSender + Send,
    {
        trace!("Starting the authority claimer loop");
        let mut transaction_sender = transaction_sender;
        loop {
            let rollups_claim = broker_listener
                .listen()
                .await
                .context(BrokerListenerSnafu)?;
            trace!("Got a claim from the broker: {:?}", rollups_claim);

            let is_duplicated_rollups_claim = duplicate_checker
                .is_duplicated_rollups_claim(&rollups_claim)
                .await
                .context(DuplicateCheckerSnafu)?;
            if is_duplicated_rollups_claim {
                trace!("It was a duplicated claim");
                continue;
            }

            info!("Sending a new rollups claim");
            transaction_sender = transaction_sender
                .send_rollups_claim(rollups_claim)
                .await
                .context(TransactionSenderSnafu)?
        }
    }
}

#[derive(Debug, snafu::Snafu)]
pub enum AuthorityClaimerError<
    L: BrokerListener + 'static,
    C: DuplicateChecker + 'static,
    S: TransactionSender + 'static,
> {
    #[snafu(display("broker listener error"))]
    BrokerListenerError { source: L::Error },

    #[snafu(display("duplicate checker error"))]
    DuplicateCheckerError { source: C::Error },

    #[snafu(display("transaction sender error"))]
    TransactionSenderError { source: S::Error },
}

// ------------------------------------------------------------------------------------------------
// DefaultAuthorityClaimer
// ------------------------------------------------------------------------------------------------

#[derive(Default)]
pub struct DefaultAuthorityClaimer;

impl DefaultAuthorityClaimer {
    pub fn new() -> Self {
        Self
    }
}

impl AuthorityClaimer for DefaultAuthorityClaimer {}
