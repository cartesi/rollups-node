// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use rollups_events::Hash;
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

    /// Get the snapshot's template hash
    async fn get_template_hash(
        &self,
        snapshot: &Snapshot,
    ) -> Result<Hash, Self::Error>;
}
