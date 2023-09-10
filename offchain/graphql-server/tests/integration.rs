// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use actix_web::dev::ServerHandle;
use actix_web::rt::spawn;
use awc::{Client, ClientRequest};
use graphql_server::{http, schema::Context};
use rollups_data::{Input, Notice, Proof, Report, Repository, Voucher};
use std::fs::read_to_string;
use std::str::from_utf8;
use std::time::{Duration, UNIX_EPOCH};
use test_fixtures::RepositoryFixture;
use testcontainers::clients::Cli;
use tokio::sync::oneshot;
use tokio::task::JoinHandle;

const QUERY_PATH: &str = "tests/queries/";
const RESPONSE_PATH: &str = "tests/responses/";
const HOST: &str = "127.0.0.1";
const PORT: u16 = 4003;

struct TestState<'d> {
    repository: RepositoryFixture<'d>,
    server: GraphQLServerWrapper,
}

impl TestState<'_> {
    async fn setup(docker: &Cli) -> TestState<'_> {
        let repository = RepositoryFixture::setup(docker);
        let server =
            GraphQLServerWrapper::spawn_server(repository.repository().clone())
                .await;
        TestState { repository, server }
    }

    async fn populate_database(&self) {
        let input = Input {
            index: 0,
            msg_sender: "msg-sender".as_bytes().to_vec(),
            tx_hash: "tx-hash".as_bytes().to_vec(),
            block_number: 0,
            timestamp: UNIX_EPOCH + Duration::from_secs(1676489717),
            payload: "input-0".as_bytes().to_vec(),
        };

        let notice = Notice {
            input_index: 0,
            index: 0,
            payload: "notice-0-0".as_bytes().to_vec(),
        };

        let voucher = Voucher {
            input_index: 0,
            index: 0,
            destination: "destination".as_bytes().to_vec(),
            payload: "voucher-0-0".as_bytes().to_vec(),
        };

        let report = Report {
            input_index: 0,
            index: 0,
            payload: "report-0-0".as_bytes().to_vec(),
        };

        let proof_voucher = Proof {
            input_index: 0,
            output_index: 0,
            output_enum: rollups_data::OutputEnum::Voucher,
            validity_input_index_within_epoch: 0,
            validity_output_index_within_input: 0,
            validity_output_hashes_root_hash: "<hash>".as_bytes().to_vec(),
            validity_vouchers_epoch_root_hash: "<hash>".as_bytes().to_vec(),
            validity_notices_epoch_root_hash: "<hash>".as_bytes().to_vec(),
            validity_machine_state_hash: "<hash>".as_bytes().to_vec(),
            validity_output_hash_in_output_hashes_siblings: vec![Some(
                "<array>".as_bytes().to_vec(),
            )],
            validity_output_hashes_in_epoch_siblings: vec![Some(
                "<array>".as_bytes().to_vec(),
            )],
            context: "<context>".as_bytes().to_vec(),
        };

        let proof_notice = Proof {
            input_index: 0,
            output_index: 0,
            output_enum: rollups_data::OutputEnum::Notice,
            validity_input_index_within_epoch: 0,
            validity_output_index_within_input: 0,
            validity_output_hashes_root_hash: "<otherhash>".as_bytes().to_vec(),
            validity_vouchers_epoch_root_hash: "<otherhash>"
                .as_bytes()
                .to_vec(),
            validity_notices_epoch_root_hash: "<otherhash>".as_bytes().to_vec(),
            validity_machine_state_hash: "<otherhash>".as_bytes().to_vec(),
            validity_output_hash_in_output_hashes_siblings: vec![Some(
                "<otherarray>".as_bytes().to_vec(),
            )],
            validity_output_hashes_in_epoch_siblings: vec![Some(
                "<otherarray>".as_bytes().to_vec(),
            )],
            context: "<context>".as_bytes().to_vec(),
        };

        let repo = self.repository.repository();

        repo.insert_input(input.clone())
            .expect("Failed to insert input");

        repo.insert_notice(notice.clone())
            .expect("Failed to insert notice");

        repo.insert_voucher(voucher.clone())
            .expect("Failed to insert voucher");

        repo.insert_report(report.clone())
            .expect("Failed to insert report");

        repo.insert_proof(proof_notice.clone())
            .expect("Failed to insert notice type proof");

        repo.insert_proof(proof_voucher.clone())
            .expect("Failed to insert voucher type proof");
    }

    async fn populate_for_pagination(&self) {
        let input = Input {
            index: 0,
            msg_sender: "msg-sender".as_bytes().to_vec(),
            tx_hash: "tx-hash".as_bytes().to_vec(),
            block_number: 0,
            timestamp: UNIX_EPOCH + Duration::from_secs(1676489717),
            payload: "input-0".as_bytes().to_vec(),
        };

        let notice0 = Notice {
            input_index: 0,
            index: 0,
            payload: "notice-0-0".as_bytes().to_vec(),
        };

        let notice1 = Notice {
            input_index: 0,
            index: 1,
            payload: "notice-0-1".as_bytes().to_vec(),
        };

        let notice2 = Notice {
            input_index: 0,
            index: 2,
            payload: "notice-0-2".as_bytes().to_vec(),
        };

        let notice3 = Notice {
            input_index: 0,
            index: 3,
            payload: "notice-0-3".as_bytes().to_vec(),
        };

        let notice4 = Notice {
            input_index: 0,
            index: 4,
            payload: "notice-0-4".as_bytes().to_vec(),
        };

        let repo = self.repository.repository();

        repo.insert_input(input.clone())
            .expect("Failed to insert input");

        repo.insert_notice(notice0.clone())
            .expect("Failed to insert notice");

        repo.insert_notice(notice1.clone())
            .expect("Failed to insert notice");

        repo.insert_notice(notice2.clone())
            .expect("Failed to insert notice");

        repo.insert_notice(notice3.clone())
            .expect("Failed to insert notice");

        repo.insert_notice(notice4.clone())
            .expect("Failed to insert notice");
    }
}

pub struct GraphQLServerWrapper {
    server_handle: ServerHandle,
    join_handle: JoinHandle<Result<(), std::io::Error>>,
}

impl GraphQLServerWrapper {
    async fn spawn_server(repository: Repository) -> Self {
        let context = Context::new(repository);
        let (tx, rx) = oneshot::channel();

        let join_handle = spawn(
            async {
                let service_handler = http::start_service(HOST, PORT, context)
                    .expect("failed to create server");
                tx.send(service_handler.handle())
                    .expect("failed to send server handle");
                service_handler
            }
            .await,
        );
        let server_handle = rx.await.expect("failed to received server handle");
        Self {
            server_handle,
            join_handle,
        }
    }

    pub async fn stop(self) {
        self.server_handle.stop(true).await;
        self.join_handle
            .await
            .expect("failed to stop graphql server")
            .expect("failed to stop graphql server");
    }
}

#[actix_web::test]
#[serial_test::serial]
async fn get_graphql() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;

    let req = create_get_request("graphql");
    let res = req.send().await.expect("Should get from graphql");
    test.server.stop().await;

    assert_eq!(res.status(), awc::http::StatusCode::from_u16(200).unwrap());
    assert_eq!(res.headers().get("content-length").unwrap(), "19050");
}

#[actix_web::test]
#[serial_test::serial]
async fn query_notice() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("notice.json").await;
    assert_from_body(body, "notice.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_notice_with_input() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("notice_with_input.json").await;
    assert_from_body(body, "notice_with_input.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_notice_with_proof() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("notice_with_proof.json").await;
    assert_from_body(body, "notice_with_proof.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_proof_from_notice() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("proof_from_notice.json").await;
    assert_from_body(body, "proof_from_notice.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_notices() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("notices.json").await;
    assert_from_body(body, "notices.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_voucher() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("voucher.json").await;
    assert_from_body(body, "voucher.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_voucher_with_input() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("voucher_with_input.json").await;
    assert_from_body(body, "voucher_with_input.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_voucher_with_proof() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("voucher_with_proof.json").await;
    assert_from_body(body, "voucher_with_proof.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_proof_from_voucher() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("proof_from_voucher.json").await;
    assert_from_body(body, "proof_from_voucher.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_vouchers() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("vouchers.json").await;
    assert_from_body(body, "vouchers.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_report() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("report.json").await;
    assert_from_body(body, "report.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_report_with_input() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("report_with_input.json").await;
    assert_from_body(body, "report_with_input.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_reports() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("reports.json").await;
    assert_from_body(body, "reports.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_with_variables() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("variables.json").await;
    assert_from_body(body, "variables.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_input() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("input.json").await;
    assert_from_body(body, "input.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_input_with_voucher() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("input_with_voucher.json").await;
    assert_from_body(body, "input_with_voucher.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_input_with_vouchers() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("input_with_vouchers.json").await;
    assert_from_body(body, "input_with_vouchers.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_input_with_notice() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("input_with_notice.json").await;
    assert_from_body(body, "input_with_notice.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_input_with_notices() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("input_with_notices.json").await;
    assert_from_body(body, "input_with_notices.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_input_with_report() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("input_with_report.json").await;
    assert_from_body(body, "input_with_report.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_input_with_reports() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("input_with_reports.json").await;
    assert_from_body(body, "input_with_reports.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_inputs() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("inputs.json").await;
    assert_from_body(body, "inputs.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_next_page() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_for_pagination().await;

    let body = post_query_request("next_page.json").await;
    assert_from_body(body, "next_page.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_previous_page() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_for_pagination().await;

    let body = post_query_request("previous_page.json").await;
    assert_from_body(body, "previous_page.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_error_missing_argument() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("error_missing_argument.json").await;
    assert_from_body(body, "error_missing_argument.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_error_not_found() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("error_not_found.json").await;
    assert_from_body(body, "error_not_found.json");
    test.server.stop().await;
}

#[actix_web::test]
#[serial_test::serial]
async fn query_error_unknown_field() {
    let docker = Cli::default();
    let test = TestState::setup(&docker).await;
    test.populate_database().await;

    let body = post_query_request("error_unknown_field.json").await;
    assert_from_body(body, "error_unknown_field.json");
    test.server.stop().await;
}

fn create_get_request(endpoint: &str) -> ClientRequest {
    let client = Client::default();

    client
        .get(format!("http://localhost:{}/{}", PORT, endpoint))
        .insert_header(("Content-type", "text/html; charset=utf-8"))
}

async fn post_query_request(query_file: &str) -> actix_web::web::Bytes {
    let query = String::from(QUERY_PATH) + query_file;
    let client = Client::builder().timeout(Duration::from_secs(5)).finish();
    let mut response = client
        .post(format!("http://localhost:{}/graphql", PORT))
        .insert_header(("Content-type", "application/json"))
        .send_body(read_to_string(query).expect("Should read request file"))
        .await
        .expect("Should query server");

    let body = response.body().await.expect("Should be body");

    body
}

fn assert_from_body(body: actix_web::web::Bytes, res_file: &str) {
    let response = String::from(RESPONSE_PATH) + res_file;
    assert_eq!(
        from_utf8(&body).expect("Should contain response body"),
        read_to_string(response).expect("Should read response file")
    );
}
