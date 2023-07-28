// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use std::path::PathBuf;

pub mod config;
pub mod disabled;
pub mod fs_manager;

/// A path to a Cartesi Machine snapshot and its metadata
#[derive(Debug, Default, Clone, PartialEq, Eq)]
pub struct Snapshot {
    pub path: PathBuf,
    pub epoch: u64,
    pub processed_input_count: u64,
}

impl Snapshot {
    /// Verifies if this is the template snapshot. The template snapshot is a
    /// Cartesi Machine snapshot taken right after its creation and before it
    /// processes any inputs
    pub fn is_template(&self) -> bool {
        self.epoch == 0 && self.processed_input_count == 0
    }
}

#[async_trait::async_trait]
pub trait SnapshotManager {
    type Error: snafu::Error;

    /// Get the most recent snapshot
    async fn get_latest(&self) -> Result<Snapshot, Self::Error>;

    /// Get the target storage directory for the snapshot
    async fn get_storage_directory(
        &self,
        epoch: u64,
        processed_input_count: u64,
    ) -> Result<Snapshot, Self::Error>;

    /// Set the most recent snapshot
    async fn set_latest(&self, snapshot: Snapshot) -> Result<(), Self::Error>;

    /// Compares `Snapshot`'s hash with the template hash stored on-chain,
    /// failing if they don't match
    async fn validate(&self, snapshot: &Snapshot) -> Result<(), Self::Error>;
}
