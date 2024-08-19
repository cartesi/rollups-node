// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use tokio::sync::{
    mpsc,
    oneshot::{self},
};
use tonic::{transport::Channel, Code, Request, Response, Status};
use uuid::Uuid;

use crate::config::InspectServerConfig;
use crate::error::InspectError;

use grpc_interfaces::cartesi_server_manager::{
    server_manager_client::ServerManagerClient, GetSessionStatusRequest,
    GetSessionStatusResponse, InspectStateRequest,
};
pub use grpc_interfaces::cartesi_server_manager::{
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
            tracing::debug!("inspect request added to the queue");
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
        tracing::warn!("failed to respond inspect request (client dropped)");
    }
}

/// Loop that answers requests coming from inspect_rx.
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

                tracing::debug!(
                    "calling grpc inspect_state request={:?} request_id={}",
                    grpc_request,
                    request_id
                );
                let mut grpc_request = Request::new(grpc_request);
                grpc_request
                    .metadata_mut()
                    .insert("request-id", request_id.parse().unwrap());
                let inspect_response = client.inspect_state(grpc_request).await;

                tracing::debug!("got grpc response from inspect_state response={:?} request_id={}",
                                inspect_response, request_id);

                let response = if inspect_response.is_ok() {
                    Ok(inspect_response.unwrap().into_inner())
                } else {
                    // The server-manager does not inform the session tainted reason.
                    // Trying to get it from the session's status.
                    let message = handle_inspect_error(
                        inspect_response.unwrap_err(),
                        session_id.clone(),
                        &mut client,
                    )
                    .await;
                    Err(InspectError::InspectFailed { message })
                };
                respond(request.response_tx, response);
            }
        }
    }
}

async fn get_session_status(
    session_id: String,
    client: &mut ServerManagerClient<Channel>,
) -> Result<Response<GetSessionStatusResponse>, Status> {
    let session_id = session_id.clone();
    let mut status_request =
        Request::new(GetSessionStatusRequest { session_id });
    let request_id = Uuid::new_v4().to_string();
    status_request
        .metadata_mut()
        .insert("request-id", request_id.parse().unwrap());
    client.get_session_status(status_request).await
}

async fn handle_inspect_error(
    status: Status,
    session_id: String,
    client: &mut ServerManagerClient<Channel>,
) -> String {
    let mut message = status.message().to_string();

    // If the session was previously tainted, the server-manager replies it with code DataLoss.
    // Trying to recover the reason for the session tainted from the session's status.
    // If not available, we return the original status error message.
    if status.code() == Code::DataLoss {
        let status_response = get_session_status(session_id, client).await;
        if status_response.is_err() {
            let err = status_response.unwrap_err().message().to_string();
            tracing::error!("get-session-status error: {:?}", err);
        } else {
            let status_response = status_response.unwrap();
            let status_response = status_response.get_ref();
            let taint_status = status_response.taint_status.clone();
            if let Some(taint_status) = taint_status {
                message = format!(
                    "Server manager session was tainted: {} ({})",
                    taint_status.error_code, taint_status.error_message
                );
                tracing::error!(message);
            }
        }
    }

    message
}
