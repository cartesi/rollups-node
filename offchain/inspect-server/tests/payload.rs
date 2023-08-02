// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

mod common;
use crate::common::*;
use inspect_server::server::CARTESI_MACHINE_RX_BUFFER_LIMIT;

struct EchoInspect {}

#[tonic::async_trait]
impl MockInspect for EchoInspect {
    async fn inspect_state(&self, payload: Vec<u8>) -> MockInspectResponse {
        MockInspectResponse {
            reports: vec![Report { payload }],
            exception: None,
            completion_status: CompletionStatus::Accepted,
        }
    }
}

async fn test_get_payload(sent_payload: &str, expected_payload: &str) {
    let test_state = TestState::setup(EchoInspect {}).await;
    let response = send_get_request(sent_payload)
        .await
        .expect("failed to obtain response");
    assert_eq!(response.status, "Accepted");
    assert_eq!(response.exception_payload, None);
    assert_eq!(response.reports.len(), 1);
    let expected_payload = String::from("0x") + &hex::encode(expected_payload);
    assert_eq!(response.reports[0].payload, expected_payload);
    test_state.teardown().await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_simple_payload() {
    test_get_payload("hello", "hello").await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_payload_with_spaces() {
    test_get_payload("hello world", "hello world").await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_url_encoded_payload() {
    test_get_payload("hello%20world", "hello world").await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_payload_with_slashes() {
    test_get_payload("user/123/name", "user/123/name").await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_payload_with_path_and_query() {
    test_get_payload(
        "user/data?key=value&key2=value2",
        "user/data?key=value&key2=value2",
    )
    .await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_raw_json_payload() {
    test_get_payload(
        r#"{"key": ["value1", "value2"]}"#,
        r#"{"key": ["value1", "value2"]}"#,
    )
    .await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_empty_payload() {
    test_get_payload("", "").await;
}

async fn test_post_payload(sent_payload: &str, expected_payload: &str) {
    let test_state = TestState::setup(EchoInspect {}).await;
    let response = send_post_request(sent_payload)
        .await
        .expect("failed to obtain response");
    assert_eq!(response.status, "Accepted");
    assert_eq!(response.exception_payload, None);
    assert_eq!(response.reports.len(), 1);
    let expected_payload = String::from("0x") + &hex::encode(expected_payload);
    assert_eq!(response.reports[0].payload, expected_payload);
    test_state.teardown().await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_post_empty_payload() {
    test_post_payload("", "").await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_post_simple_payload() {
    test_post_payload("hello", "hello").await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_post_raw_json_payload() {
    test_post_payload(
        r#"{"key": ["value1", "value2"]}"#,
        r#"{"key": ["value1", "value2"]}"#,
    )
    .await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_post_payload_on_limit() {
    let payload = "0".repeat(CARTESI_MACHINE_RX_BUFFER_LIMIT);
    test_post_payload(&payload.clone(), &payload.clone()).await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_post_fails_when_payload_over_limit() {
    let payload = "0".repeat(CARTESI_MACHINE_RX_BUFFER_LIMIT + 1);
    let test_state = TestState::setup(EchoInspect {}).await;
    send_post_request(&payload)
        .await
        .expect_err("Payload reached size limit");
    test_state.teardown().await;
}
