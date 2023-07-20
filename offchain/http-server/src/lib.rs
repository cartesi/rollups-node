// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

mod config;
pub use config::HttpServerConfig;

// Re-exporting prometheus' Registry.
pub use prometheus_client::registry::Registry;

// Re-exporting prometheus metrics.
// Add any other metrics to re-export here.
pub use prometheus_client::metrics::counter::Counter as CounterRef;
pub use prometheus_client::metrics::family::Family as FamilyRef;
// End of metrics to re-export.

// Re-exporting hyper error.
pub use hyper::Error as HttpServerError;

use axum::{routing::get, Router};
use prometheus_client::encoding::text::encode;
use std::{
    net::SocketAddr,
    sync::{Arc, Mutex},
};

/// Starts a HTTP server with two endpoints: /healthz and /metrics.
///
/// The `Registry` parameter is a `prometheus` type used for metric tracking.
pub async fn start(
    config: HttpServerConfig,
    registry: Registry,
) -> Result<(), hyper::Error> {
    let ip = "0.0.0.0".parse().expect("could not parse host address");
    let addr = SocketAddr::new(ip, config.port);
    tracing::info!("Starting HTTP server at {}", addr);

    let registry = Arc::new(Mutex::new(registry));
    let router = Router::new()
        .route("/healthz", get(|| async { "" }))
        .route("/metrics", get(|| get_metrics(registry)));

    axum::Server::bind(&addr)
        .serve(router.into_make_service())
        .await
}

/// Returns the metrics as a specially encoded string.
async fn get_metrics(registry: Arc<Mutex<Registry>>) -> String {
    let registry = registry.lock().unwrap();
    let mut buffer = String::new();
    encode(&mut buffer, &registry).unwrap();
    buffer
}
