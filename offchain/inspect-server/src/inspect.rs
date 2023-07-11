// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use tokio::sync::{mpsc, oneshot};
use tonic::Request;
use uuid::Uuid;

use crate::config::InspectServerConfig;
use crate::error::InspectError;
use crate::grpc::server_manager::{
    server_manager_client::ServerManagerClient, InspectStateRequest,
};

pub use crate::grpc::server_manager::{
    CompletionStatus, InspectStateResponse, Report,
};

#[derive(Clone)]
pub struct InspectClient {
    inspect_tx: mpsc::Sender<InspectRequest>,
}

/// The inspect client is a wrapper that just sends the inspect requests to another thread and
/// waits for the result. The actual request to the server manager is done by the handle_inspect
/// function.
impl InspectClient {
    pub fn new(config: &InspectServerConfig) -> Self {
        let (inspect_tx, inspect_rx) = mpsc::channel(config.queue_size);
        let address = config.server_manager_address.clone();
        let session_id = config.session_id.clone();
        tokio::spawn(handle_inspect(address, session_id, inspect_rx));
        Self { inspect_tx }
    }

    pub async fn inspect(
        &self,
        payload: Vec<u8>,
    ) -> Result<InspectStateResponse, InspectError> {
        let (response_tx, response_rx) = oneshot::channel();
        let request = InspectRequest {
            payload,
            response_tx,
        };
        if let Err(e) = self.inspect_tx.try_send(request) {
            return Err(InspectError::InspectFailed {
                message: e.to_string(),
            });
        } else {
            log::debug!("inspect request added to the queue");
        }
        response_rx.await.expect("handle_inspect never fails")
    }
}

struct InspectRequest {
    payload: Vec<u8>,
    response_tx: oneshot::Sender<Result<InspectStateResponse, InspectError>>,
}

fn respond(
    response_tx: oneshot::Sender<Result<InspectStateResponse, InspectError>>,
    response: Result<InspectStateResponse, InspectError>,
) {
    if response_tx.send(response).is_err() {
        log::warn!("failed to respond inspect request (client dropped)");
    }
}

/// Loop that answers requests comming from inspect_rx.
async fn handle_inspect(
    address: String,
    session_id: String,
    mut inspect_rx: mpsc::Receiver<InspectRequest>,
) {
    let endpoint = format!("http://{}", address);
    while let Some(request) = inspect_rx.recv().await {
        match ServerManagerClient::connect(endpoint.clone()).await {
            Err(e) => {
                respond(
                    request.response_tx,
                    Err(InspectError::FailedToConnect {
                        message: e.to_string(),
                    }),
                );
            }
            Ok(mut client) => {
                let request_id = Uuid::new_v4().to_string();
                let grpc_request = InspectStateRequest {
                    session_id: session_id.clone(),
                    query_payload: request.payload,
                };

                log::debug!(
                    "calling grpc inspect_state request={:?} request_id={}",
                    grpc_request,
                    request_id
                );
                let mut grpc_request = Request::new(grpc_request);
                grpc_request
                    .metadata_mut()
                    .insert("request-id", request_id.parse().unwrap());
                let grpc_response = client.inspect_state(grpc_request).await;

                log::debug!("got grpc response from inspect_state response={:?} request_id={}", grpc_response, request_id);

                let response = grpc_response
                    .map(|result| result.into_inner())
                    .map_err(|e| InspectError::InspectFailed {
                        message: e.message().to_string(),
                    });
                respond(request.response_tx, response);
            }
        }
    }
}
