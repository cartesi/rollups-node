// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

#![allow(dead_code)]

use actix_web::dev::ServerHandle;
use inspect_server::config::InspectServerConfig;
pub use reqwest::StatusCode;
use std::sync::Arc;
use tokio::sync::{oneshot, Notify};
use tokio::task::JoinHandle;
use tonic::{transport::Server, Request, Response, Status};

use inspect_server::grpc::cartesi_machine::Void;
use inspect_server::grpc::server_manager::{
    server_manager_server::{ServerManager, ServerManagerServer},
    AdvanceStateRequest, DeleteEpochRequest, EndSessionRequest,
    FinishEpochRequest, FinishEpochResponse, GetEpochStatusRequest,
    GetEpochStatusResponse, GetSessionStatusRequest, GetSessionStatusResponse,
    GetStatusResponse, InspectStateRequest, InspectStateResponse,
    StartSessionRequest, StartSessionResponse,
};
pub use inspect_server::grpc::server_manager::{CompletionStatus, Report};
use inspect_server::grpc::versioning::GetVersionResponse;
use inspect_server::inspect::InspectClient;
use inspect_server::server::HttpInspectResponse;

pub const SERVER_MANAGER_ADDRESS: &'static str = "127.0.0.1:50001";
pub const INSPECT_SERVER_ADDRESS: &'static str = "127.0.0.1:50002";
pub const SESSION_ID: &'static str = "default session";
pub const ACTIVE_EPOCH_INDEX: u64 = 123;
pub const PROCESSED_INPUT_COUNT: u64 = 456;
pub const QUEUE_SIZE: usize = 3;

pub struct TestState {
    server_manager: MockServerManagerWrapper,
    inspect_server: InspectServerWrapper,
}

impl TestState {
    /// Start the inspect-server and the mock-server-manager
    pub async fn setup(mock: impl MockInspect) -> Self {
        let _ = env_logger::builder()
            .filter_level(log::LevelFilter::Debug)
            .is_test(true)
            .try_init();
        let server_manager = MockServerManagerWrapper::start(mock).await;
        let inspect_server = InspectServerWrapper::start().await;
        Self {
            server_manager,
            inspect_server,
        }
    }

    /// Shutdown both servers.
    /// This function cannot be implemented as the drop trait because it is async.
    pub async fn teardown(self) {
        self.inspect_server.stop().await;
        self.server_manager.stop().await;
    }
}

#[derive(Clone, Debug, Default)]
pub struct MockInspectResponse {
    pub reports: Vec<Report>,
    pub exception: Option<Vec<u8>>,
    pub completion_status: CompletionStatus,
}

#[tonic::async_trait]
pub trait MockInspect: Send + Sync + 'static {
    async fn inspect_state(&self, payload: Vec<u8>) -> MockInspectResponse;
}

pub struct InspectServerWrapper {
    server_handle: ServerHandle,
    join_handle: JoinHandle<()>,
}

impl InspectServerWrapper {
    /// Start the inspect server in another thread.
    /// This function blocks until the server is ready.
    pub async fn start() -> Self {
        let inspect_server_config = InspectServerConfig {
            inspect_server_address: INSPECT_SERVER_ADDRESS.to_string(),
            server_manager_address: SERVER_MANAGER_ADDRESS.to_string(),
            session_id: SESSION_ID.to_string(),
            queue_size: QUEUE_SIZE,
            healthcheck_port: 0,
        };

        let inspect_client = InspectClient::new(&inspect_server_config);
        let (handle_tx, handle_rx) = oneshot::channel();
        let join_handle = tokio::spawn(async move {
            let server = inspect_server::server::create(
                &inspect_server_config,
                inspect_client,
            )
            .expect("failed to start inspect server");
            handle_tx
                .send(server.handle())
                .expect("failed to send server handle");
            server.await.expect("inspect server execution failed");
        });
        let server_handle =
            handle_rx.await.expect("failed to received server handle");
        Self {
            server_handle,
            join_handle,
        }
    }

    /// Stop the inspect server.
    /// This function blocks util the server is shut down.
    pub async fn stop(self) {
        self.server_handle.stop(true).await;
        self.join_handle
            .await
            .expect("failed to stop inspect server");
    }
}

pub struct MockServerManagerWrapper {
    shutdown: Arc<Notify>,
    join_handle: JoinHandle<()>,
}

impl MockServerManagerWrapper {
    /// Start the server manager in another thread.
    /// This function blocks until the server is ready.
    pub async fn start(mock: impl MockInspect) -> Self {
        let service = MockServerManager { mock };
        let address = SERVER_MANAGER_ADDRESS.parse().expect("invalid address");
        let ready = Arc::new(Notify::new());
        let shutdown = Arc::new(Notify::new());
        let join_handle = {
            let ready = ready.clone();
            let shutdown = shutdown.clone();
            tokio::spawn(async move {
                let server = Server::builder()
                    .add_service(ServerManagerServer::new(service))
                    .serve_with_shutdown(address, shutdown.notified());
                ready.notify_one();
                server.await.expect("failed to start server manager");
            })
        };
        ready.notified().await;
        Self {
            shutdown,
            join_handle,
        }
    }

    /// Stop the server manager.
    /// This function blocks until the server is shut down.
    pub async fn stop(self) {
        self.shutdown.notify_one();
        self.join_handle
            .await
            .expect("failed to shutdown server manager");
    }
}

struct MockServerManager<T: MockInspect> {
    mock: T,
}

#[tonic::async_trait]
impl<T: MockInspect> ServerManager for MockServerManager<T> {
    async fn inspect_state(
        &self,
        request: Request<InspectStateRequest>,
    ) -> Result<Response<InspectStateResponse>, Status> {
        let mock_response = self
            .mock
            .inspect_state(request.into_inner().query_payload)
            .await;
        let response = InspectStateResponse {
            session_id: SESSION_ID.to_string(),
            active_epoch_index: ACTIVE_EPOCH_INDEX,
            processed_input_count: PROCESSED_INPUT_COUNT,
            exception_data: mock_response.exception,
            status: mock_response.completion_status as i32,
            reports: mock_response.reports,
        };
        Ok(Response::new(response))
    }

    async fn get_version(
        &self,
        _: Request<Void>,
    ) -> Result<Response<GetVersionResponse>, Status> {
        unimplemented!()
    }

    async fn start_session(
        &self,
        _: Request<StartSessionRequest>,
    ) -> Result<Response<StartSessionResponse>, Status> {
        unimplemented!()
    }

    async fn end_session(
        &self,
        _: Request<EndSessionRequest>,
    ) -> Result<Response<Void>, Status> {
        unimplemented!()
    }

    async fn advance_state(
        &self,
        _: Request<AdvanceStateRequest>,
    ) -> Result<Response<Void>, Status> {
        unimplemented!()
    }

    async fn finish_epoch(
        &self,
        _: Request<FinishEpochRequest>,
    ) -> Result<Response<FinishEpochResponse>, Status> {
        unimplemented!()
    }

    async fn get_status(
        &self,
        _: Request<Void>,
    ) -> Result<Response<GetStatusResponse>, Status> {
        unimplemented!()
    }

    async fn get_session_status(
        &self,
        _: Request<GetSessionStatusRequest>,
    ) -> Result<Response<GetSessionStatusResponse>, Status> {
        unimplemented!()
    }

    async fn get_epoch_status(
        &self,
        _: Request<GetEpochStatusRequest>,
    ) -> Result<Response<GetEpochStatusResponse>, Status> {
        unimplemented!()
    }

    async fn delete_epoch(
        &self,
        _: Request<DeleteEpochRequest>,
    ) -> Result<Response<Void>, Status> {
        unimplemented!()
    }
}

/// Send an inspect-state request to the inspect server via GET.
/// If the status code is 200, return the HttpInspectResponse.
/// Else, return the status code and the error message.
pub async fn send_get_request(
    payload: &str,
) -> Result<HttpInspectResponse, (StatusCode, String)> {
    let url = format!("http://{}/inspect/{}", INSPECT_SERVER_ADDRESS, payload);
    let response = reqwest::get(url)
        .await
        .expect("failed to send inspect via GET");
    let status = response.status();
    if status == 200 {
        let response = response
            .json::<HttpInspectResponse>()
            .await
            .expect("failed to decode json response");
        Ok(response)
    } else {
        let message = response
            .text()
            .await
            .expect("failed to obtain response body");
        Err((status, message))
    }
}

/// Send an inspect-state request to the inspect server via POST.
/// If the status code is 200, return the HttpInspectResponse.
/// Else, return the status code and the error message.
pub async fn send_post_request(
    payload: &str,
) -> Result<HttpInspectResponse, (StatusCode, String)> {
    let url = format!("http://{}/inspect", INSPECT_SERVER_ADDRESS);
    let client = reqwest::Client::new();
    let response = client
        .post(url)
        .body(String::from(payload))
        .send()
        .await
        .expect("failed to send inspect via POST");
    let status = response.status();
    if status == 200 {
        let response = response
            .json::<HttpInspectResponse>()
            .await
            .expect("failed to decode json response");
        Ok(response)
    } else {
        let message = response
            .text()
            .await
            .expect("failed to obtain response body");
        Err((status, message))
    }
}

/// Convert binary value to the hex format
pub fn hex_to_bin(payload: &Vec<u8>) -> String {
    String::from("0x") + &hex::encode(payload)
}
