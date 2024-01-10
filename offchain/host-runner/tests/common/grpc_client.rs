// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

pub use grpc_interfaces::cartesi_machine::*;
pub use grpc_interfaces::cartesi_server_manager::*;

use super::config;

pub type ServerManagerClient =
    server_manager_client::ServerManagerClient<tonic::transport::Channel>;

pub async fn connect() -> ServerManagerClient {
    ServerManagerClient::connect(config::get_grpc_server_manager_address())
        .await
        .expect("failed to connect to grpc server")
}

pub fn create_timestamp() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_secs()
}

pub fn create_start_session_request(session_id: &str) -> StartSessionRequest {
    StartSessionRequest {
        session_id: session_id.into(),
        machine_directory: "".into(),
        active_epoch_index: 0,
        processed_input_count: 0,
        server_cycles: None,
        server_deadline: None,
        runtime: None,
    }
}

pub fn create_advance_state_request(
    session_id: &str,
    epoch_index: u64,
    input_index: u64,
) -> AdvanceStateRequest {
    AdvanceStateRequest {
        session_id: session_id.into(),
        active_epoch_index: epoch_index,
        current_input_index: input_index,
        input_metadata: Some(InputMetadata {
            msg_sender: Some(Address {
                data: super::create_address(),
            }),
            block_number: 0,
            timestamp: create_timestamp(),
            epoch_index: 0, //this field is deprecated and should always be 0
            input_index,
        }),
        input_payload: super::create_payload(),
    }
}
