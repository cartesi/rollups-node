// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use backoff::ExponentialBackoffBuilder;
use diesel::pg::Pg;
use diesel::{
    sql_query, Connection, PgConnection, QueryableByName, RunQueryDsl,
};
use redacted::Redacted;
use rollups_data::Connection as PaginationConnection;
use rollups_data::{
    Cursor, Edge, Error, Input, InputQueryFilter, Notice, PageInfo, Proof,
    Report, Repository, RepositoryConfig, Voucher,
};
use serial_test::serial;
use std::time::{Duration, UNIX_EPOCH};
use test_fixtures::DataFixture;
use testcontainers::clients::Cli;

const BACKOFF_DURATION: u64 = 120000;

struct TestState<'d> {
    data: DataFixture<'d>,
}

impl TestState<'_> {
    fn setup(docker: &Cli) -> TestState<'_> {
        let data = DataFixture::setup(docker);
        TestState { data }
    }

    pub fn get_repository(&self) -> Repository {
        let backoff = ExponentialBackoffBuilder::new()
            .with_max_elapsed_time(Some(Duration::from_millis(
                BACKOFF_DURATION,
            )))
            .build();

        Repository::new(RepositoryConfig {
            user: self.data.user.clone(),
            password: Redacted::new(self.data.password.clone()),
            hostname: self.data.hostname.clone(),
            port: self.data.port.clone(),
            db: self.data.db.clone(),
            connection_pool_size: 3,
            backoff,
        })
        .expect("Repository should have connected successfully")
    }

    pub fn get_from_sql<T: QueryableByName<Pg> + 'static>(
        &self,
        query: &str,
    ) -> T {
        let mut conn = PgConnection::establish(&self.data.endpoint)
            .expect("failed to connect to db");
        sql_query(query)
            .load::<T>(&mut conn)
            .expect("failed to run query")
            .pop()
            .expect("query returned no results")
    }
}

pub fn insert_test_input(repo: &Repository) {
    let input = Input {
        index: 0,
        msg_sender: "msg-sender".as_bytes().to_vec(),
        tx_hash: "tx-hash".as_bytes().to_vec(),
        block_number: 0,
        timestamp: UNIX_EPOCH + Duration::from_secs(1676489717),
        payload: "input-0".as_bytes().to_vec(),
    };

    repo.insert_input(input)
        .expect("The input should've been inserted")
}

pub fn create_input() -> Input {
    Input {
        index: 0,
        msg_sender: "msg-sender".as_bytes().to_vec(),
        tx_hash: "tx-hash".as_bytes().to_vec(),
        block_number: 0,
        timestamp: UNIX_EPOCH + Duration::from_secs(1676489717),
        payload: "input-0".as_bytes().to_vec(),
    }
}

#[test]
#[serial]
fn test_create_repository() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);

    // Since we create the repository for every test, we created an auxiliary function
    // to do so, and to avoid code duplication, we are calling this function here
    test.get_repository();
}

#[test]
#[serial]
fn test_fail_to_create_repository() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);

    let backoff = ExponentialBackoffBuilder::new()
        .with_max_elapsed_time(Some(Duration::from_millis(2000)))
        .build();

    let err = Repository::new(RepositoryConfig {
        user: "Err".to_string(),
        password: Redacted::new(test.data.password.clone()),
        hostname: test.data.hostname.clone(),
        port: test.data.port.clone(),
        db: test.data.db.clone(),
        connection_pool_size: 3,
        backoff,
    })
    .expect_err("Repository::new should fail");

    assert!(matches!(err, Error::DatabaseConnectionError { source: _ }));
}

#[test]
#[serial]
fn test_insert_input() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    let input = create_input();

    repo.insert_input(input.clone())
        .expect("Failed to insert input");

    let result: Input = test.get_from_sql("Select * from inputs");

    assert_eq!(result, input);
}

#[test]
#[serial]
fn test_get_input() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    let input = create_input();

    repo.insert_input(input.clone())
        .expect("Failed to insert input");

    let get_input = repo.get_input(0).expect("Failed to get input");

    assert_eq!(input, get_input);
}

#[test]
#[serial]
fn test_get_input_error() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    let input = create_input();

    repo.insert_input(input.clone())
        .expect("Failed to insert input");

    let input_error = repo.get_input(1).expect_err("Get input should fail");

    assert!(matches!(
        input_error,
        Error::ItemNotFound { item_type } if item_type == "input"
    ));
}

#[test]
#[serial]
fn test_insert_notice() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    insert_test_input(&repo);

    let notice = Notice {
        input_index: 0,
        index: 0,
        payload: "notice-0-0".as_bytes().to_vec(),
    };

    repo.insert_notice(notice.clone())
        .expect("Failed to insert notice");

    let result: Notice = test.get_from_sql("Select * from notices");

    assert_eq!(result, notice);
}

#[test]
#[serial]
fn test_get_notice() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    insert_test_input(&repo);

    let notice = Notice {
        input_index: 0,
        index: 0,
        payload: "notice-0-0".as_bytes().to_vec(),
    };

    repo.insert_notice(notice.clone())
        .expect("Failed to insert notice");

    let get_notice = repo
        .get_notice(0, 0)
        .expect("Get notice should have returned a value");

    assert_eq!(notice, get_notice);
}

#[test]
#[serial]
fn test_insert_notice_error() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    insert_test_input(&repo);

    let notice = Notice {
        input_index: 1,
        index: 0,
        payload: "notice-0-0".as_bytes().to_vec(),
    };
    let notice_error = repo
        .insert_notice(notice.clone())
        .expect_err("Insert notice should fail");

    assert!(matches!(notice_error, Error::DatabaseError { source: _ }));
}

#[test]
#[serial]
fn test_get_notice_error() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    insert_test_input(&repo);

    let notice = Notice {
        input_index: 0,
        index: 0,
        payload: "notice-0-0".as_bytes().to_vec(),
    };
    repo.insert_notice(notice.clone())
        .expect("Insert notice should succeed");

    let notice_error =
        repo.get_notice(1, 1).expect_err("Get notice should fail");

    assert!(matches!(
        notice_error,
        Error::ItemNotFound { item_type } if item_type == "notice"
    ));
}

#[test]
#[serial]
fn test_insert_voucher() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    insert_test_input(&repo);

    let voucher = Voucher {
        input_index: 0,
        index: 0,
        destination: "destination".as_bytes().to_vec(),
        payload: "voucher-0-0".as_bytes().to_vec(),
    };

    repo.insert_voucher(voucher.clone())
        .expect("Insert voucher should succeed");

    let result: Voucher = test.get_from_sql("Select * from vouchers");

    assert_eq!(result, voucher);
}

#[test]
#[serial]
fn test_get_voucher() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    insert_test_input(&repo);

    let voucher = Voucher {
        input_index: 0,
        index: 0,
        destination: "destination".as_bytes().to_vec(),
        payload: "voucher-0-0".as_bytes().to_vec(),
    };
    repo.insert_voucher(voucher.clone())
        .expect("Insert voucher should succeed");

    let get_voucher =
        repo.get_voucher(0, 0).expect("Get voucher should succeed");

    assert_eq!(voucher, get_voucher);
}

#[test]
#[serial]
fn test_insert_voucher_error() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    insert_test_input(&repo);

    let voucher = Voucher {
        input_index: 1,
        index: 0,
        destination: "destination".as_bytes().to_vec(),
        payload: "voucher-1-0".as_bytes().to_vec(),
    };
    let voucher_error = repo
        .insert_voucher(voucher.clone())
        .expect_err("Insert voucher should fail");

    assert!(matches!(voucher_error, Error::DatabaseError { source: _ }));
}

#[test]
#[serial]
fn test_get_voucher_error() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    insert_test_input(&repo);

    let voucher = Voucher {
        input_index: 0,
        index: 0,
        destination: "destination".as_bytes().to_vec(),
        payload: "voucher-0-0".as_bytes().to_vec(),
    };
    repo.insert_voucher(voucher.clone())
        .expect("Insert voucher should succeed");

    let voucher_error =
        repo.get_voucher(1, 1).expect_err("Get voucher should fail");

    assert!(matches!(
        voucher_error,
        Error::ItemNotFound { item_type } if item_type == "voucher"
    ));
}

#[test]
#[serial]
fn test_insert_report() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    insert_test_input(&repo);

    let report = Report {
        input_index: 0,
        index: 0,
        payload: "report-0-0".as_bytes().to_vec(),
    };

    repo.insert_report(report.clone())
        .expect("Insert report should succeed");

    let result: Report = test.get_from_sql("Select * from reports");

    assert_eq!(result, report);
}

#[test]
#[serial]
fn test_get_report() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    insert_test_input(&repo);

    let report = Report {
        input_index: 0,
        index: 0,
        payload: "report-0-0".as_bytes().to_vec(),
    };
    repo.insert_report(report.clone())
        .expect("Insert report should succeed");

    let get_report = repo.get_report(0, 0).expect("Get report should succeed");

    assert_eq!(report, get_report);
}

#[test]
#[serial]
fn test_insert_report_error() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    insert_test_input(&repo);

    let report = Report {
        input_index: 1,
        index: 0,
        payload: "report-1-0".as_bytes().to_vec(),
    };
    let report_error = repo
        .insert_report(report.clone())
        .expect_err("Insert report should fail");

    assert!(matches!(report_error, Error::DatabaseError { source: _ }));
}

#[test]
#[serial]
fn test_get_report_error() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    insert_test_input(&repo);

    let report = Report {
        input_index: 0,
        index: 0,
        payload: "report-0-0".as_bytes().to_vec(),
    };
    repo.insert_report(report.clone())
        .expect("Insert report should succeed");

    let report_error =
        repo.get_report(1, 1).expect_err("Get report should fail");

    assert!(matches!(
        report_error,
        Error::ItemNotFound { item_type } if item_type == "report"
    ));
}

#[test]
#[serial]
fn test_insert_proof() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    let proof = Proof {
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

    repo.insert_proof(proof.clone())
        .expect("Insert proof should succeed");

    let result: Proof = test.get_from_sql("Select * from proofs");

    assert_eq!(result, proof);
}

#[test]
#[serial]
fn test_get_proof() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    let proof = Proof {
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
    repo.insert_proof(proof.clone())
        .expect("Insert proof should succeed");

    let get_proof = repo
        .get_proof(0, 0, rollups_data::OutputEnum::Voucher)
        .unwrap()
        .expect("Get proof should succeed");

    assert_eq!(proof, get_proof);
}

#[test]
#[serial]
fn test_get_proof_error() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    let proof = Proof {
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
    repo.insert_proof(proof.clone())
        .expect("Insert proof should succeed");

    let proof_error = repo.get_proof(1, 1, rollups_data::OutputEnum::Voucher);

    match proof_error {
        Ok(None) => assert!(true),
        Ok(Some(_proof)) => assert!(false),
        Err(_) => assert!(false),
    }
}

#[test]
#[serial]
fn test_pagination_macro() {
    let docker = Cli::default();
    let test = TestState::setup(&docker);
    let repo = test.get_repository();

    let input0 = create_input();

    let input1 = Input {
        index: 1,
        msg_sender: "msg-sender".as_bytes().to_vec(),
        tx_hash: "tx-hash".as_bytes().to_vec(),
        block_number: 0,
        timestamp: UNIX_EPOCH + Duration::from_secs(1676489717),
        payload: "input-1".as_bytes().to_vec(),
    };

    repo.insert_input(input0.clone())
        .expect("Insert input should succeed");
    repo.insert_input(input1.clone())
        .expect("Insert input should succeed");

    let query_filter = InputQueryFilter {
        index_greater_than: Some(-1),
        index_lower_than: Some(5),
    };

    let pagination_connection = repo
        .get_inputs(Some(5), None, None, None, query_filter)
        .expect("The macro should work, creating a pagination connection");

    assert_eq!(
        pagination_connection,
        PaginationConnection {
            total_count: 2,
            edges: vec![
                Edge {
                    node: input0,
                    cursor: Cursor::decode("MA==")
                        .expect("Should create cursor with correct offset"),
                },
                Edge {
                    node: input1,
                    cursor: Cursor::decode("MQ==")
                        .expect("Should create cursor with correct offset"),
                },
            ],
            page_info: PageInfo {
                start_cursor: Some(
                    Cursor::decode("MA==")
                        .expect("Should create cursor with correct offset")
                ),
                end_cursor: Some(
                    Cursor::decode("MQ==")
                        .expect("Should create cursor with correct offset")
                ),
                has_next_page: false,
                has_previous_page: false,
            },
        }
    );
}
