// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::common::*;
use grpc_interfaces::versioning::{GetVersionResponse, SemanticVersion};

#[tokio::test]
#[serial_test::serial]
async fn test_it_gets_version() {
    let _manager = manager::Wrapper::new().await;
    let mut grpc_client = grpc_client::connect().await;
    let response = grpc_client
        .get_version(grpc_client::Void {})
        .await
        .unwrap()
        .into_inner();
    assert_eq!(
        response,
        GetVersionResponse {
            version: Some(SemanticVersion {
                major: 0,
                minor: 2,
                patch: 0,
                pre_release: String::from(""),
                build: String::from("host-runner"),
            })
        }
    );
}
