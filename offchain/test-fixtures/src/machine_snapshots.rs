// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use std::path::PathBuf;
use tempfile::TempDir;

use crate::docker_cli;

const TAG: &str = "cartesi/rollups-node-snapshot:devel";
const CONTAINER_SNAPSHOT_DIR: &str = "/usr/share/cartesi/snapshot";

pub struct MachineSnapshotsFixture {
    dir: TempDir,
}

impl MachineSnapshotsFixture {
    #[tracing::instrument(level = "trace", skip_all)]
    pub fn setup() -> Self {
        tracing::info!("setting up machine snapshots fixture");

        let dir = tempfile::tempdir().expect("failed to create temp dir");
        let id = docker_cli::create(TAG);
        let from_container = format!("{}:{}", id, CONTAINER_SNAPSHOT_DIR);
        docker_cli::cp(&from_container, dir.path().to_str().unwrap());
        docker_cli::rm(&id);
        Self { dir }
    }

    /// Return the path of directory that contains the snapshot
    pub fn path(&self) -> PathBuf {
        let snapshot_dir = PathBuf::from(CONTAINER_SNAPSHOT_DIR);
        self.dir
            .path()
            .join(snapshot_dir.file_name().expect("impossible"))
    }
}
