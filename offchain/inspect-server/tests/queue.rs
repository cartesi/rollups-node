// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

mod common;
use crate::common::*;

use futures::stream::FuturesUnordered;
use futures::StreamExt;
use tokio::sync::{mpsc, Mutex};

struct SyncInspect {
    response_rx: Mutex<mpsc::Receiver<MockInspectResponse>>,
}

#[tonic::async_trait]
impl MockInspect for SyncInspect {
    async fn inspect_state(&self, _: Vec<u8>) -> MockInspectResponse {
        self.response_rx.lock().await.recv().await.unwrap()
    }
}

impl SyncInspect {
    fn setup() -> (Self, mpsc::Sender<MockInspectResponse>) {
        let (response_tx, response_rx) = mpsc::channel(1000);
        let mock = SyncInspect {
            response_rx: Mutex::new(response_rx),
        };
        (mock, response_tx)
    }
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_error_when_server_manager_is_down() {
    let inspect_server = InspectServerWrapper::start().await;
    let (status, message) = send_get_request("hello")
        .await
        .expect_err("failed to obtain response");
    assert_eq!(status, StatusCode::BAD_GATEWAY);
    assert_eq!(
        &message,
        "Failed to connect to server manager: transport error"
    );
    inspect_server.stop().await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_succeeds_after_server_manager_starts() {
    let inspect_server = InspectServerWrapper::start().await;
    let (status, _) = send_get_request("hello")
        .await
        .expect_err("failed to obtain response");
    assert_eq!(status, StatusCode::BAD_GATEWAY);
    let (mock, response_tx) = SyncInspect::setup();
    let server_manager = MockServerManagerWrapper::start(mock).await;
    // Add response to queue before sending request
    response_tx
        .send(MockInspectResponse::default())
        .await
        .expect("failed to send response");
    send_get_request("hello")
        .await
        .expect("failed to obtain response");
    server_manager.stop().await;
    inspect_server.stop().await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_it_handle_concurrent_inspect_requests() {
    let (mock, response_tx) = SyncInspect::setup();
    let state = TestState::setup(mock).await;
    // Send multiple concurrent requests
    let handlers: Vec<_> = (0..QUEUE_SIZE)
        .map(|_| {
            tokio::spawn(async {
                send_get_request("hello")
                    .await
                    .expect("failed to obtain response");
            })
        })
        .collect();
    // Wait until the requests arrive in the inspect-server
    tokio::time::sleep(std::time::Duration::from_millis(100)).await;
    // Add the responses to the queue
    for _ in 0..QUEUE_SIZE {
        response_tx
            .send(MockInspectResponse::default())
            .await
            .expect("failed to send response");
    }
    // Check the responses
    for handler in handlers {
        handler.await.expect("failed to wait handler");
    }
    state.teardown().await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_it_returns_error_when_queue_is_full() {
    let (mock, response_tx) = SyncInspect::setup();
    let state = TestState::setup(mock).await;
    // Send concurrent requests to overflow the queue.
    // We need to 2 extra requests to overflow the queue because the first message will be
    // imediatelly consumed and removed from the queue.
    let mut handlers = FuturesUnordered::new();
    for _ in 0..(QUEUE_SIZE + 2) {
        handlers.push(tokio::spawn(send_get_request("hello")));
    }
    // Poll the handlers to find the overflow error
    let (status, message) = handlers
        .next()
        .await
        .expect("failed to poll")
        .expect("failed to join handler")
        .expect_err("failed to receive error");
    assert_eq!(status, StatusCode::BAD_REQUEST);
    assert_eq!(
        message,
        String::from("Failed to inspect state: no available capacity")
    );
    // Add the responses to the queue
    for _ in 0..(QUEUE_SIZE + 1) {
        response_tx
            .send(MockInspectResponse::default())
            .await
            .expect("failed to send response");
    }
    // Wait for responses so we don't have zombie threads
    while let Some(handler) = handlers.next().await {
        let _ = handler.expect("failed to join handler");
    }
    state.teardown().await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_post_error_when_server_manager_is_down() {
    let inspect_server = InspectServerWrapper::start().await;
    let (status, message) = send_get_request("hello")
        .await
        .expect_err("failed to obtain response");
    assert_eq!(status, StatusCode::BAD_GATEWAY);
    assert_eq!(
        &message,
        "Failed to connect to server manager: transport error"
    );
    inspect_server.stop().await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_post_succeeds_after_server_manager_starts() {
    let inspect_server = InspectServerWrapper::start().await;
    let (status, _) = send_post_request("hello")
        .await
        .expect_err("failed to obtain response");
    assert_eq!(status, StatusCode::BAD_GATEWAY);
    let (mock, response_tx) = SyncInspect::setup();
    let server_manager = MockServerManagerWrapper::start(mock).await;
    // Add response to queue before sending request
    response_tx
        .send(MockInspectResponse::default())
        .await
        .expect("failed to send response");
    send_post_request("hello")
        .await
        .expect("failed to obtain response");
    server_manager.stop().await;
    inspect_server.stop().await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_post_it_handle_concurrent_inspect_requests() {
    let (mock, response_tx) = SyncInspect::setup();
    let state = TestState::setup(mock).await;
    // Send multiple concurrent requests
    let handlers: Vec<_> = (0..QUEUE_SIZE)
        .map(|_| {
            tokio::spawn(async {
                send_post_request("hello")
                    .await
                    .expect("failed to obtain response");
            })
        })
        .collect();
    // Wait until the requests arrive in the inspect-server
    tokio::time::sleep(std::time::Duration::from_millis(100)).await;
    // Add the responses to the queue
    for _ in 0..QUEUE_SIZE {
        response_tx
            .send(MockInspectResponse::default())
            .await
            .expect("failed to send response");
    }
    // Check the responses
    for handler in handlers {
        handler.await.expect("failed to wait handler");
    }
    state.teardown().await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_post_it_returns_error_when_queue_is_full() {
    let (mock, response_tx) = SyncInspect::setup();
    let state = TestState::setup(mock).await;
    // Send concurrent requests to overflow the queue.
    // We need to 2 extra requests to overflow the queue because the first message will be
    // imediatelly consumed and removed from the queue.
    let mut handlers = FuturesUnordered::new();
    for _ in 0..(QUEUE_SIZE + 2) {
        handlers.push(tokio::spawn(send_post_request("hello")));
    }
    // Poll the handlers to find the overflow error
    let (status, message) = handlers
        .next()
        .await
        .expect("failed to poll")
        .expect("failed to join handler")
        .expect_err("failed to receive error");
    assert_eq!(status, StatusCode::BAD_REQUEST);
    assert_eq!(
        message,
        String::from("Failed to inspect state: no available capacity")
    );
    // Add the responses to the queue
    for _ in 0..(QUEUE_SIZE + 1) {
        response_tx
            .send(MockInspectResponse::default())
            .await
            .expect("failed to send response");
    }
    // Wait for responses so we don't have zombie threads
    while let Some(handler) = handlers.next().await {
        let _ = handler.expect("failed to join handler");
    }
    state.teardown().await;
}
