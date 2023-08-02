// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

mod common;
use crate::common::*;

struct FixedResponseInspect {
    response: MockInspectResponse,
}

#[tonic::async_trait]
impl MockInspect for FixedResponseInspect {
    async fn inspect_state(&self, _: Vec<u8>) -> MockInspectResponse {
        self.response.clone()
    }
}

async fn test_get_response(sent: MockInspectResponse, expected_status: &str) {
    let mock = FixedResponseInspect {
        response: sent.clone(),
    };
    let state = TestState::setup(mock).await;
    let response = send_get_request("")
        .await
        .expect("failed to obtain response");
    assert_eq!(&response.status, expected_status);
    assert_eq!(
        response.exception_payload,
        sent.exception.as_ref().map(hex_to_bin)
    );
    assert_eq!(response.reports.len(), sent.reports.len());
    for (received, sent) in response.reports.iter().zip(sent.reports) {
        assert_eq!(received.payload, hex_to_bin(&sent.payload));
    }
    assert_eq!(response.processed_input_count, PROCESSED_INPUT_COUNT);
    state.teardown().await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_response_with_no_reports() {
    let response = MockInspectResponse {
        reports: vec![],
        exception: None,
        completion_status: CompletionStatus::Accepted,
    };
    test_get_response(response, "Accepted").await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_response_with_single_report() {
    let response = MockInspectResponse {
        reports: vec![Report {
            payload: vec![1, 2, 3],
        }],
        exception: None,
        completion_status: CompletionStatus::Accepted,
    };
    test_get_response(response, "Accepted").await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_response_with_multiple_reports() {
    let reports = vec![
        Report {
            payload: vec![1, 2, 3],
        },
        Report {
            payload: vec![4, 5, 6],
        },
        Report {
            payload: vec![7, 8, 9],
        },
    ];
    let response = MockInspectResponse {
        reports,
        exception: None,
        completion_status: CompletionStatus::Accepted,
    };
    test_get_response(response, "Accepted").await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_get_response_with_reports_and_exception() {
    let response = MockInspectResponse {
        reports: vec![Report {
            payload: vec![1, 2, 3],
        }],
        exception: Some(vec![4, 5, 6]),
        completion_status: CompletionStatus::Exception,
    };
    test_get_response(response, "Exception").await;
}

async fn test_post_response(sent: MockInspectResponse, expected_status: &str) {
    let mock = FixedResponseInspect {
        response: sent.clone(),
    };
    let state = TestState::setup(mock).await;
    let response = send_post_request("")
        .await
        .expect("failed to obtain response");
    assert_eq!(&response.status, expected_status);
    assert_eq!(
        response.exception_payload,
        sent.exception.as_ref().map(hex_to_bin)
    );
    assert_eq!(response.reports.len(), sent.reports.len());
    for (received, sent) in response.reports.iter().zip(sent.reports) {
        assert_eq!(received.payload, hex_to_bin(&sent.payload));
    }
    assert_eq!(response.processed_input_count, PROCESSED_INPUT_COUNT);
    state.teardown().await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_post_response_with_no_reports() {
    let response = MockInspectResponse {
        reports: vec![],
        exception: None,
        completion_status: CompletionStatus::Accepted,
    };
    test_post_response(response, "Accepted").await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_post_response_with_single_report() {
    let response = MockInspectResponse {
        reports: vec![Report {
            payload: vec![1, 2, 3],
        }],
        exception: None,
        completion_status: CompletionStatus::Accepted,
    };
    test_post_response(response, "Accepted").await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_post_response_with_multiple_reports() {
    let reports = vec![
        Report {
            payload: vec![1, 2, 3],
        },
        Report {
            payload: vec![4, 5, 6],
        },
        Report {
            payload: vec![7, 8, 9],
        },
    ];
    let response = MockInspectResponse {
        reports,
        exception: None,
        completion_status: CompletionStatus::Accepted,
    };
    test_post_response(response, "Accepted").await;
}

#[tokio::test]
#[serial_test::serial]
async fn test_post_response_with_reports_and_exception() {
    let response = MockInspectResponse {
        reports: vec![Report {
            payload: vec![1, 2, 3],
        }],
        exception: Some(vec![4, 5, 6]),
        completion_status: CompletionStatus::Exception,
    };
    test_post_response(response, "Exception").await;
}
