// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use indexer::IndexerError;
use rand::Rng;
use rollups_data::{
    Input, Notice, OutputEnum, Proof, Report, RepositoryConfig, Voucher,
};
use rollups_events::{
    BrokerConfig, BrokerEndpoint, DAppMetadata, InputMetadata,
    RollupsAdvanceStateInput, RollupsData, RollupsNotice, RollupsOutput,
    RollupsOutputEnum, RollupsOutputValidityProof, RollupsProof, RollupsReport,
    RollupsVoucher,
};
use serial_test::serial;
use std::time::UNIX_EPOCH;
use test_fixtures::{BrokerFixture, RepositoryFixture};
use testcontainers::clients::Cli;
use tokio::task::JoinHandle;

const BROKER_CONSUME_TIMEOUT: usize = 100;

/// Starts one container with the broker, one container with the database,
/// and the indexer in a background thread.
struct TestState<'d> {
    broker: BrokerFixture<'d>,
    repository: RepositoryFixture<'d>,
    indexer: JoinHandle<Result<(), IndexerError>>,
}

#[test_log::test(tokio::test)]
#[serial]
async fn indexer_inserts_inputs() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    const N: u64 = 3;
    let mut inputs = vec![];
    for i in 0..N {
        let input = state.produce_input_in_broker(i).await;
        inputs.push(input);
    }

    for input_sent in inputs.into_iter() {
        let input_read = state.get_input_from_database(&input_sent).await;
        assert_input_eq(&input_sent, &input_read);
    }
}

#[test_log::test(tokio::test)]
#[serial]
async fn indexer_inserts_vouchers() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    const N: u64 = 3;
    let mut vouchers = vec![];
    for i in 0..N {
        state.produce_input_in_broker(i).await;
        for j in 0..N {
            let voucher = state.produce_voucher_in_broker(i, j).await;
            vouchers.push(voucher)
        }
    }

    for voucher_sent in vouchers.into_iter() {
        let voucher_read = state.get_voucher_from_database(&voucher_sent).await;
        assert_voucher_eq(&voucher_sent, &voucher_read);
    }
}

#[test_log::test(tokio::test)]
#[serial]
async fn indexer_inserts_notices() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    const N: u64 = 3;
    let mut notices = vec![];
    for i in 0..N {
        state.produce_input_in_broker(i).await;
        for j in 0..N {
            let notice = state.produce_notice_in_broker(i, j).await;
            notices.push(notice);
        }
    }

    for notice_sent in notices.into_iter() {
        let notice_read = state.get_notice_from_database(&notice_sent).await;
        assert_notice_eq(&notice_sent, &notice_read);
    }
}

#[test_log::test(tokio::test)]
#[serial]
async fn indexer_inserts_reports() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    const N: u64 = 3;
    let mut reports = vec![];
    for i in 0..N {
        state.produce_input_in_broker(i).await;
        for j in 0..N {
            let report = state.produce_report_in_broker(i, j).await;
            reports.push(report);
        }
    }

    for report_sent in reports.into_iter() {
        let report_read = state.get_report_from_database(&report_sent).await;
        assert_report_eq(&report_sent, &report_read);
    }
}

#[test_log::test(tokio::test)]
#[serial]
async fn indexer_inserts_proofs() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    const N: u64 = 3;
    let mut proofs = vec![];
    for i in 0..N {
        state.produce_input_in_broker(i).await;
        for j in 0..N {
            state.produce_voucher_in_broker(i, j).await;
            state.produce_notice_in_broker(i, j).await;
            proofs.push(
                state
                    .produce_proof_in_broker(i, j, RollupsOutputEnum::Voucher)
                    .await,
            );
            proofs.push(
                state
                    .produce_proof_in_broker(i, j, RollupsOutputEnum::Notice)
                    .await,
            );
        }
    }

    for proof_sent in proofs.into_iter() {
        let proof_read = state.get_proof_from_database(&proof_sent).await;
        assert_proof_eq(&proof_sent, &proof_read);
    }
}

#[test_log::test(tokio::test)]
#[serial]
async fn indexer_ignores_finish_epoch_and_insert_input_after() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    tracing::info!("producing finish epoch");
    let data = RollupsData::FinishEpoch {};
    state.broker.produce_input_event(data).await;

    let input_sent = state.produce_input_in_broker(0).await;
    let input_read = state.get_input_from_database(&input_sent).await;
    assert_input_eq(&input_sent, &input_read);
}

#[test_log::test(tokio::test)]
#[serial]
async fn indexer_does_not_override_existing_input() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    let original_input = state.produce_input_in_broker(0).await;
    let _second_input = state.produce_input_in_broker(0).await;
    let input_read = state.get_input_from_database(&original_input).await;
    assert_input_eq(&original_input, &input_read);
}

#[test_log::test(tokio::test)]
#[serial]
async fn indexer_inserts_input_after_broker_timeout() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    tracing::info!("sleeping so the broker consume times out in indexer");
    tokio::time::sleep(std::time::Duration::from_millis(
        2 * BROKER_CONSUME_TIMEOUT as u64,
    ))
    .await;

    let input_sent = state.produce_input_in_broker(0).await;
    let input_read = state.get_input_from_database(&input_sent).await;
    assert_input_eq(&input_sent, &input_read);
}

#[test_log::test(tokio::test)]
#[serial]
async fn indexer_fails_to_insert_output_when_input_does_not_exist() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    state.produce_voucher_in_broker(0, 0).await;
    let error = state.get_indexer_error().await;
    assert!(matches!(error, IndexerError::RepositoryError { .. }));
}

impl TestState<'_> {
    async fn setup(docker: &Cli) -> TestState<'_> {
        let broker = BrokerFixture::setup(docker).await;
        let repository = RepositoryFixture::setup(docker);
        let indexer = spawn_indexer(
            repository.config(),
            broker.redis_endpoint().to_owned(),
            broker.dapp_metadata(),
        )
        .await;
        TestState {
            broker,
            repository,
            indexer,
        }
    }

    /// Wait for the indexer to fail and return the error
    async fn get_indexer_error(self) -> IndexerError {
        tracing::info!("waiting for indexer to fail");
        self.indexer
            .await
            .expect("failed to wait for indexer")
            .expect_err("indexer should exit with error")
    }

    async fn produce_input_in_broker(
        &self,
        input_index: u64,
    ) -> RollupsAdvanceStateInput {
        let timestamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;
        self.produce_input_in_broker_with_timestamp(input_index, timestamp)
            .await
    }

    async fn produce_input_in_broker_with_timestamp(
        &self,
        input_index: u64,
        timestamp: u64,
    ) -> RollupsAdvanceStateInput {
        let metadata = InputMetadata {
            epoch_index: 0,
            input_index,
            block_number: rand::thread_rng().gen(),
            msg_sender: random_array().into(),
            timestamp,
        };
        let input = RollupsAdvanceStateInput {
            metadata,
            payload: random_array::<32>().to_vec().into(),
            tx_hash: random_array().into(),
        };
        let data = RollupsData::AdvanceStateInput(input.clone());

        tracing::info!(?input, "producing input");
        self.broker.produce_input_event(data).await;
        input
    }

    async fn get_input_from_database(
        &self,
        input_sent: &RollupsAdvanceStateInput,
    ) -> Input {
        tracing::info!("waiting for input in database");
        let index = input_sent.metadata.input_index as i32;
        self.repository.retry(move |r| r.get_input(index)).await
    }

    async fn produce_voucher_in_broker(
        &self,
        input_index: u64,
        index: u64,
    ) -> RollupsVoucher {
        let voucher = RollupsVoucher {
            index,
            input_index,
            destination: random_array().into(),
            payload: random_array::<32>().to_vec().into(),
        };
        let output = RollupsOutput::Voucher(voucher.clone());

        tracing::info!(?voucher, "producing voucher");
        self.broker.produce_output(output).await;
        voucher
    }

    async fn get_voucher_from_database(
        &self,
        voucher_sent: &RollupsVoucher,
    ) -> Voucher {
        tracing::info!("waiting for voucher in database");
        let input_index = voucher_sent.input_index as i32;
        let index = voucher_sent.index as i32;
        self.repository
            .retry(move |r| r.get_voucher(index, input_index))
            .await
    }

    async fn produce_notice_in_broker(
        &self,
        input_index: u64,
        index: u64,
    ) -> RollupsNotice {
        let notice = RollupsNotice {
            index,
            input_index,
            payload: random_array::<32>().to_vec().into(),
        };
        let output = RollupsOutput::Notice(notice.clone());

        tracing::info!(?notice, "producing notice");
        self.broker.produce_output(output).await;
        notice
    }

    async fn get_notice_from_database(
        &self,
        notice_sent: &RollupsNotice,
    ) -> Notice {
        tracing::info!("waiting for notice in database");
        let input_index = notice_sent.input_index as i32;
        let index = notice_sent.index as i32;
        self.repository
            .retry(move |r| r.get_notice(index, input_index))
            .await
    }

    async fn produce_report_in_broker(
        &self,
        input_index: u64,
        index: u64,
    ) -> RollupsReport {
        let report = RollupsReport {
            index,
            input_index,
            payload: random_array::<32>().to_vec().into(),
        };
        let output = RollupsOutput::Report(report.clone());

        tracing::info!(?report, "producing report");
        self.broker.produce_output(output).await;
        report
    }

    async fn get_report_from_database(
        &self,
        report_sent: &RollupsReport,
    ) -> Report {
        tracing::info!("waiting for report in database");
        let input_index = report_sent.input_index as i32;
        let index = report_sent.index as i32;
        self.repository
            .retry(move |r| r.get_report(index, input_index))
            .await
    }

    async fn produce_proof_in_broker(
        &self,
        input_index: u64,
        output_index: u64,
        output_enum: RollupsOutputEnum,
    ) -> RollupsProof {
        let validity = RollupsOutputValidityProof {
            input_index_within_epoch: input_index,
            output_index_within_input: output_index,
            output_hashes_root_hash: random_array().into(),
            vouchers_epoch_root_hash: random_array().into(),
            notices_epoch_root_hash: random_array().into(),
            machine_state_hash: random_array().into(),
            output_hash_in_output_hashes_siblings: vec![random_array().into()],
            output_hashes_in_epoch_siblings: vec![random_array().into()],
        };
        let proof = RollupsProof {
            input_index,
            output_index,
            output_enum,
            validity,
            context: random_array::<32>().to_vec().into(),
        };
        let output = RollupsOutput::Proof(proof.clone());

        tracing::info!(?proof, "producing proof");
        self.broker.produce_output(output).await;
        proof
    }

    async fn get_proof_from_database(
        &self,
        proof_sent: &RollupsProof,
    ) -> Proof {
        tracing::info!("waiting for proof in database");
        let input_index = proof_sent.input_index as i32;
        let output_index = proof_sent.output_index as i32;
        let output_enum = match proof_sent.output_enum {
            RollupsOutputEnum::Voucher => OutputEnum::Voucher,
            RollupsOutputEnum::Notice => OutputEnum::Notice,
        };
        self.repository
            .retry(move |r| {
                match r.get_proof(input_index, output_index, output_enum) {
                    Ok(option_proof) => {
                        // The retry only works properly if the query
                        // returns item not found
                        option_proof.ok_or(rollups_data::Error::ItemNotFound {
                            item_type: "proof".to_owned(),
                        })
                    }
                    Err(e) => Err(e),
                }
            })
            .await
    }
}

async fn spawn_indexer(
    repository_config: RepositoryConfig,
    redis_endpoint: BrokerEndpoint,
    dapp_metadata: DAppMetadata,
) -> JoinHandle<Result<(), IndexerError>> {
    let broker_config = BrokerConfig {
        redis_endpoint,
        consume_timeout: BROKER_CONSUME_TIMEOUT,
        backoff: Default::default(),
    };
    let indexer_config = indexer::IndexerConfig {
        repository_config,
        dapp_metadata,
        broker_config,
        healthcheck_port: 0,
    };
    tokio::spawn(async move {
        indexer::run(indexer_config).await.map_err(|e| {
            tracing::error!("{:?}", e);
            e
        })
    })
}

fn random_array<const N: usize>() -> [u8; N] {
    let mut arr = [0; N];
    for i in 0..N {
        arr[i] = rand::thread_rng().gen();
    }
    arr
}

fn assert_input_eq(input_sent: &RollupsAdvanceStateInput, input_read: &Input) {
    assert_eq!(input_read.index as u64, input_sent.metadata.input_index);
    assert_eq!(
        &input_read.msg_sender,
        input_sent.metadata.msg_sender.inner()
    );
    assert_eq!(&input_read.tx_hash, input_sent.tx_hash.inner());
    assert_eq!(
        input_read.block_number as u64,
        input_sent.metadata.block_number
    );
    assert_eq!(
        input_read
            .timestamp
            .duration_since(UNIX_EPOCH)
            .expect("failed to get time")
            .as_secs(),
        input_sent.metadata.timestamp
    );
    assert_eq!(&input_read.payload, input_sent.payload.inner());
}

fn assert_voucher_eq(voucher_sent: &RollupsVoucher, voucher_read: &Voucher) {
    assert_eq!(voucher_read.index as u64, voucher_sent.index);
    assert_eq!(voucher_read.input_index as u64, voucher_sent.input_index);
    assert_eq!(&voucher_read.destination, voucher_sent.destination.inner());
    assert_eq!(&voucher_read.payload, voucher_sent.payload.inner());
}

fn assert_notice_eq(notice_sent: &RollupsNotice, notice_read: &Notice) {
    assert_eq!(notice_read.index as u64, notice_sent.index);
    assert_eq!(notice_read.input_index as u64, notice_sent.input_index);
    assert_eq!(&notice_read.payload, notice_sent.payload.inner());
}

fn assert_report_eq(report_sent: &RollupsReport, report_read: &Report) {
    assert_eq!(report_read.index as u64, report_sent.index);
    assert_eq!(report_read.input_index as u64, report_sent.input_index);
    assert_eq!(&report_read.payload, report_sent.payload.inner());
}

fn assert_proof_eq(proof_sent: &RollupsProof, proof_read: &Proof) {
    let output_enum = match proof_sent.output_enum {
        RollupsOutputEnum::Voucher => OutputEnum::Voucher,
        RollupsOutputEnum::Notice => OutputEnum::Notice,
    };
    assert_eq!(proof_read.input_index as u64, proof_sent.input_index);
    assert_eq!(proof_read.output_index as u64, proof_sent.output_index);
    assert_eq!(proof_read.output_enum, output_enum);
    assert_eq!(
        proof_read.validity_input_index_within_epoch as u64,
        proof_sent.validity.input_index_within_epoch
    );
    assert_eq!(
        proof_read.validity_output_index_within_input as u64,
        proof_sent.validity.output_index_within_input
    );
    assert_eq!(
        &proof_read.validity_output_hashes_root_hash,
        proof_sent.validity.output_hashes_root_hash.inner()
    );
    assert_eq!(
        &proof_read.validity_vouchers_epoch_root_hash,
        proof_sent.validity.vouchers_epoch_root_hash.inner()
    );
    assert_eq!(
        &proof_read.validity_notices_epoch_root_hash,
        proof_sent.validity.notices_epoch_root_hash.inner()
    );
    assert_eq!(
        &proof_read.validity_machine_state_hash,
        proof_sent.validity.machine_state_hash.inner()
    );
    for (siblings_read, siblings_sent) in proof_read
        .validity_output_hash_in_output_hashes_siblings
        .iter()
        .zip(&proof_sent.validity.output_hash_in_output_hashes_siblings)
    {
        assert_eq!(siblings_read.as_ref().unwrap(), siblings_sent.inner());
    }
    for (siblings_read, siblings_sent) in proof_read
        .validity_output_hashes_in_epoch_siblings
        .iter()
        .zip(&proof_sent.validity.output_hashes_in_epoch_siblings)
    {
        assert_eq!(siblings_read.as_ref().unwrap(), siblings_sent.inner());
    }
    assert_eq!(&proof_read.context, proof_sent.context.inner());
}
