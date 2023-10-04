// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

mod config;
mod controller;
mod conversions;
mod driver;
mod grpc;
mod hash;
mod http;
mod merkle_tree;
mod model;
mod proofs;

use futures_util::FutureExt;
use std::sync::{atomic::AtomicBool, atomic::Ordering, Arc};
use std::time::Duration;
use tokio::sync::oneshot;

use clap::Parser;
use config::{CLIConfig, Config};
use controller::Controller;

fn log_result<T, E: std::error::Error>(name: &str, result: Result<T, E>) {
    let prefix = format!("http {} terminated ", name);
    match result {
        Ok(_) => tracing::info!("{} successfully", prefix),
        Err(e) => tracing::warn!("{} with error: {}", prefix, e),
    };
}

#[actix_web::main]
async fn main() {
    let config: Config = CLIConfig::parse().into();

    log::configure(&config.log_config);

    log::log_service_start(&config, "Host Runner");

    let controller =
        Controller::new(Duration::from_millis(config.finish_timeout));
    let http_service_running = Arc::new(AtomicBool::new(true));
    let (grpc_shutdown_tx, grpc_shutdown_rx) = oneshot::channel::<()>();
    let grpc_service = {
        let controller = controller.clone();
        let config = config.clone();
        let shutdown = grpc_shutdown_rx.map(|_| ());
        let http_service_running = http_service_running.clone();
        tokio::spawn(async move {
            log_result(
                "gRPC service",
                grpc::start_service(&config, controller.clone(), shutdown)
                    .await,
            );
            if http_service_running.load(Ordering::Relaxed) {
                panic!("gRPC service terminated before shutdown signal");
            }
        })
    };

    // We run the actix-web in the main thread because it handles the SIGINT
    let host_runner_handle = http::start_services(&config, controller.clone());
    let health_handle = http_health_check::start(config.healthcheck_port);
    tokio::select! {
        result = health_handle => {
            log_result("http health check", result);
        }
        result = host_runner_handle => {
            log_result("http service", result);
        }
    }
    http_service_running.store(false, Ordering::Relaxed);

    // Shutdown the other services
    if let Err(e) = controller.shutdown().await.await {
        tracing::error!("failed to shutdown controller ({})", e);
    }
    if grpc_shutdown_tx.send(()).is_err() {
        tracing::error!("failed to send the shutdown signal to grpc");
    }
    if let Err(e) = grpc_service.await {
        tracing::error!("failed to shutdown the grpc service ({})", e);
    }
}
