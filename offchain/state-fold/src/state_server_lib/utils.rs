// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::grpc_server::StateServer;

use crate::state_fold::Foldable;
use ethers::providers::Middleware;
use grpc_interfaces::state_fold_server::state_fold_server::StateFoldServer;

use std::sync::Arc;
use tokio::{select, signal, sync::oneshot};
use tonic::transport::Server;

pub async fn start_server<
    M: Middleware + 'static,
    UD: Send + Sync + 'static,
    F: Foldable<UserData = UD> + 'static,
>(
    address: std::net::SocketAddr,
    state_server: StateServer<M, UD, F>,
    kill_switch: oneshot::Receiver<()>,
) -> Result<(), tonic::transport::Error>
where
    F::InitialState: serde::de::DeserializeOwned + 'static,
    F: serde::Serialize,
{
    let (mut health_reporter, health_server) =
        tonic_health::server::health_reporter();

    health_reporter
        .set_serving::<StateFoldServer<StateServer<M, UD, F>>>()
        .await;

    let block_subscriber = Arc::clone(&state_server.block_subscriber);

    tracing::info!("StateFoldServer listening on {}", address);

    Server::builder()
        .trace_fn(|_| tracing::trace_span!("state_fold_server"))
        .add_service(health_server)
        .add_service(StateFoldServer::new(state_server))
        .serve_with_shutdown(address, async {
            select! {
                r = block_subscriber.wait_for_completion() => {
                    tracing::error!("`block_subscriber` has exited: {:?}", r);
                    tracing::error!("Shutting down...");
                    return;
                }

                r = kill_switch => {
                    tracing::info!("Graceful context shutdown: {:?}", r);
                    return;
                }
            }
        })
        .await
}

pub async fn wait_for_signal(tx: oneshot::Sender<()>) {
    let _ = signal::ctrl_c().await;
    tracing::info!("SIGINT received: shutting down");
    let _ = tx.send(());
}
