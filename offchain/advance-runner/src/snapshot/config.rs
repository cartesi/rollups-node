// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::Parser;
use rollups_events::Address;
use snafu::{ensure, Snafu};
use std::path::PathBuf;
use url::Url;

#[derive(Debug, Clone)]
pub struct FSManagerConfig {
    pub snapshot_dir: PathBuf,
    pub snapshot_latest: PathBuf,
    pub validation_enabled: bool,
    pub provider_http_endpoint: Option<Url>,
    pub dapp_address: Address,
}

#[derive(Debug, Clone)]
pub enum SnapshotConfig {
    FileSystem(FSManagerConfig),
    Disabled,
}

impl SnapshotConfig {
    pub fn new(
        cli_config: SnapshotCLIConfig,
        dapp_address: Address,
    ) -> Result<Self, SnapshotConfigError> {
        if cli_config.snapshot_enabled {
            let snapshot_dir = PathBuf::from(cli_config.snapshot_dir);
            ensure!(snapshot_dir.is_dir(), DirSnafu);

            let snapshot_latest = PathBuf::from(cli_config.snapshot_latest);
            ensure!(snapshot_latest.is_symlink(), SymlinkSnafu);

            let validation_enabled = cli_config.snapshot_validation_enabled;
            if validation_enabled {
                ensure!(
                    cli_config.provider_http_endpoint.is_some(),
                    NoProviderEndpointSnafu,
                );
            }

            let provider_http_endpoint = cli_config.provider_http_endpoint;

            Ok(SnapshotConfig::FileSystem(FSManagerConfig {
                snapshot_dir,
                snapshot_latest,
                validation_enabled,
                provider_http_endpoint,
                dapp_address,
            }))
        } else {
            Ok(SnapshotConfig::Disabled)
        }
    }
}

#[derive(Debug, Snafu)]
#[allow(clippy::enum_variant_names)]
pub enum SnapshotConfigError {
    #[snafu(display("Snapshot dir isn't a directory"))]
    DirError {},

    #[snafu(display("Snapshot latest isn't a symlink"))]
    SymlinkError {},

    #[snafu(display("A provider http endpoint is required"))]
    NoProviderEndpointError {},
}

#[derive(Parser, Debug)]
#[command(name = "snapshot")]
pub struct SnapshotCLIConfig {
    /// If set to false, disables snapshots. Enabled by default
    #[arg(long, env, default_value_t = true)]
    snapshot_enabled: bool,

    /// Path to the directory with the snapshots
    #[arg(long, env)]
    snapshot_dir: String,

    /// Path to the symlink of the latest snapshot
    #[arg(long, env)]
    snapshot_latest: String,

    /// If set to false, disables snapshot validation. Enabled by default
    #[arg(long, env, default_value_t = true)]
    snapshot_validation_enabled: bool,

    /// The endpoint for a JSON-RPC provider.
    /// Required if SNAPSHOT_VALIDATION_ENABLED is `true`
    #[arg(long, env, value_parser = Url::parse)]
    provider_http_endpoint: Option<Url>,
}
