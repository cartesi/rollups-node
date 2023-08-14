// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use rollups_events::RollupsClaim;
use snafu::Snafu;
use std::fmt::Debug;

/// The `DuplicateChecker` checks if a given claim was already submitted to the blockchain.
#[async_trait]
pub trait DuplicateChecker: Debug {
    type Error: snafu::Error + 'static;

    async fn is_duplicated_rollups_claim(
        &self,
        rollups_claim: &RollupsClaim,
    ) -> Result<bool, Self::Error>;
}

// ------------------------------------------------------------------------------------------------
// DefaultDuplicateChecker
// ------------------------------------------------------------------------------------------------

#[derive(Debug)]
pub struct DefaultDuplicateChecker;

#[derive(Debug, Snafu)]
pub enum DuplicateCheckerError {
    Todo,
}

impl DefaultDuplicateChecker {
    pub fn new() -> Result<Self, DuplicateCheckerError> {
        todo!()
    }
}

#[async_trait]
impl DuplicateChecker for DefaultDuplicateChecker {
    type Error = DuplicateCheckerError;

    async fn is_duplicated_rollups_claim(
        &self,
        _rollups_claim: &RollupsClaim,
    ) -> Result<bool, Self::Error> {
        Err(DuplicateCheckerError::Todo)
    }
}
