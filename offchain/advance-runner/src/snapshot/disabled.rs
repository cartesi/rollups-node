// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::{Snapshot, SnapshotManager};

#[derive(Debug)]
pub struct SnapshotDisabled {}

#[derive(snafu::Snafu, Debug)]
#[snafu(display("shapshot disabled"))]
pub struct SnapshotDisabledError {}

#[async_trait::async_trait]
impl SnapshotManager for SnapshotDisabled {
    type Error = SnapshotDisabledError;

    /// Get the most recent snapshot
    #[tracing::instrument(level = "trace", skip_all)]
    async fn get_latest(&self) -> Result<Snapshot, Self::Error> {
        tracing::trace!("snapshots disabled; returning default");
        Ok(Default::default())
    }

    /// Get the target storage directory for the snapshot
    #[tracing::instrument(level = "trace", skip_all)]
    async fn get_storage_directory(
        &self,
        _: u64,
        _: u64,
    ) -> Result<Snapshot, Self::Error> {
        tracing::trace!("snapshots disabled; returning default");
        Ok(Default::default())
    }

    /// Set the most recent snapshot
    #[tracing::instrument(level = "trace", skip_all)]
    async fn set_latest(&self, _: Snapshot) -> Result<(), Self::Error> {
        tracing::trace!("snapshots disabled; ignoring");
        Ok(())
    }

    #[tracing::instrument(level = "trace", skip_all)]
    async fn validate(&self, _: &Snapshot) -> Result<(), Self::Error> {
        tracing::trace!("snapshots disabled; ignoring");
        Ok(())
    }
}
