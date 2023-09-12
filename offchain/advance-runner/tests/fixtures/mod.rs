// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use advance_runner::config::{
    AdvanceRunnerConfig, BrokerConfig, DAppMetadata, FSManagerConfig,
    ServerManagerConfig, SnapshotConfig,
};
use advance_runner::AdvanceRunnerError;
use grpc_interfaces::cartesi_machine::{
    ConcurrencyConfig, MachineRuntimeConfig,
};
use grpc_interfaces::cartesi_server_manager::{CyclesConfig, DeadlineConfig};
use rollups_events::{Address, BrokerEndpoint};
use std::cell::RefCell;
use std::path::Path;
use std::time::Duration;
use tokio::task::JoinHandle;

pub struct AdvanceRunnerFixture {
    config: AdvanceRunnerConfig,
    handler: RefCell<Option<JoinHandle<Result<(), AdvanceRunnerError>>>>,
}

impl AdvanceRunnerFixture {
    pub async fn setup(
        server_manager_endpoint: String,
        session_id: String,
        redis_endpoint: BrokerEndpoint,
        chain_id: u64,
        dapp_address: Address,
        snapshot_dir: Option<&Path>,
    ) -> Self {
        let runtime_config = MachineRuntimeConfig {
            concurrency: Some(ConcurrencyConfig {
                update_merkle_tree: 0,
            }),
        };

        let deadline_config = DeadlineConfig {
            checkin: 1000 * 5,
            advance_state: 1000 * 60 * 3,
            advance_state_increment: 1000 * 10,
            inspect_state: 1000 * 60 * 3,
            inspect_state_increment: 1000 * 10,
            machine: 1000 * 60 * 5,
            store: 1000 * 60 * 3,
            fast: 1000 * 5,
        };

        let cycles_config = CyclesConfig {
            max_advance_state: u64::MAX >> 2,
            advance_state_increment: 1 << 22,
            max_inspect_state: u64::MAX >> 2,
            inspect_state_increment: 1 << 22,
        };

        let server_manager_config = ServerManagerConfig {
            server_manager_endpoint,
            max_decoding_message_size: 100 * 1024 * 1024,
            session_id,
            pending_inputs_sleep_duration: 1000,
            pending_inputs_max_retries: 10,
            runtime_config,
            deadline_config,
            cycles_config,
        };

        let dapp_metadata = DAppMetadata {
            chain_id,
            dapp_address,
        };

        let broker_config = BrokerConfig {
            redis_endpoint,
            consume_timeout: 100,
            backoff: Default::default(),
        };

        let snapshot_config = if snapshot_dir.is_some() {
            SnapshotConfig::FileSystem(FSManagerConfig {
                snapshot_dir: snapshot_dir
                    .expect("Should have a Path")
                    .to_owned(),
                snapshot_latest: snapshot_dir
                    .expect("Should have a Path")
                    .join("latest"),
            })
        } else {
            SnapshotConfig::Disabled
        };

        let backoff_max_elapsed_duration = Duration::from_millis(1);

        let config = AdvanceRunnerConfig {
            server_manager_config,
            broker_config,
            dapp_metadata,
            snapshot_config,
            backoff_max_elapsed_duration,
            healthcheck_port: 0,
        };
        let handler = RefCell::new(Some(start_advance_runner(config.clone())));
        Self { config, handler }
    }

    /// Wait until the advance runner exists with an error
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn wait_err(&self) -> AdvanceRunnerError {
        tracing::trace!("waiting for advance runner error");
        let handler = self.handler.replace(None);
        handler
            .expect("handler not found")
            .await
            .expect("failed to wait for handler")
            .expect_err("advance runner should exit with an error")
    }

    /// Abort the current advance runner, wait it to finish and start another one
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn restart(&self) {
        tracing::trace!("restartin advance runner");
        let handler = self.handler.replace(None).expect("handler not found");
        handler.abort();
        handler
            .await
            .expect_err("advance runner finished before abort");
        let new_handler = start_advance_runner(self.config.clone());
        self.handler.replace(Some(new_handler));
    }
}

fn start_advance_runner(
    config: AdvanceRunnerConfig,
) -> JoinHandle<Result<(), AdvanceRunnerError>> {
    tokio::spawn(async move {
        let output = advance_runner::run(config).await;
        tracing::error!(?output, "advance_runner exited");
        output
    })
}

impl Drop for AdvanceRunnerFixture {
    fn drop(&mut self) {
        if let Some(handler) = self.handler.borrow().as_ref() {
            handler.abort();
        }
    }
}
