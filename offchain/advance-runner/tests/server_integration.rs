// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use fixtures::AdvanceRunnerFixture;
use rand::Rng;
use rollups_events::{
    Hash, InputMetadata, Payload, RollupsAdvanceStateInput, RollupsClaim,
    RollupsData, RollupsInput, INITIAL_ID,
};
use test_fixtures::{
    BrokerFixture, MachineSnapshotsFixture, ServerManagerFixture,
};
use testcontainers::clients::Cli;

mod fixtures;

struct TestState<'d> {
    snapshots: MachineSnapshotsFixture,
    broker: BrokerFixture<'d>,
    server_manager: ServerManagerFixture<'d>,
    advance_runner: AdvanceRunnerFixture,
}

impl TestState<'_> {
    async fn setup(docker: &Cli) -> TestState<'_> {
        let snapshots = MachineSnapshotsFixture::setup();
        let broker = BrokerFixture::setup(docker).await;
        let server_manager =
            ServerManagerFixture::setup(docker, snapshots.path()).await;
        let advance_runner = AdvanceRunnerFixture::setup(
            server_manager.endpoint().to_owned(),
            server_manager.session_id().to_owned(),
            broker.redis_endpoint().to_owned(),
            broker.chain_id(),
            broker.dapp_address().to_owned(),
            Some(snapshots.path()),
        )
        .await;
        TestState {
            snapshots,
            broker,
            server_manager,
            advance_runner,
        }
    }
}

fn generate_payload() -> Payload {
    let len = rand::thread_rng().gen_range(100..200);
    let data: Vec<u8> = (0..len).map(|_| rand::thread_rng().gen()).collect();
    Payload::new(data)
}

#[test_log::test(tokio::test)]
async fn test_advance_runner_starts_server_manager_session() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    tracing::info!("checking whether advance_runner created session");
    state.server_manager.assert_session_ready().await;
}

#[test_log::test(tokio::test)]
async fn test_advance_runner_sends_inputs_to_server_manager() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    const N: usize = 3;
    tracing::info!("producing {} inputs", N);
    let payloads: Vec<_> = (0..N).map(|_| generate_payload()).collect();
    for (i, payload) in payloads.iter().enumerate() {
        let data = RollupsData::AdvanceStateInput(RollupsAdvanceStateInput {
            metadata: InputMetadata {
                input_index: i as u64,
                ..Default::default()
            },
            payload: payload.clone().into(),
            tx_hash: Hash::default(),
        });
        state.broker.produce_input_event(data).await;
    }

    tracing::info!("waiting until the inputs are processed");
    state.server_manager.assert_session_ready().await;
    state
        .server_manager
        .assert_epoch_status_payloads(0, &payloads)
        .await;
}

#[test_log::test(tokio::test)]
async fn test_advance_runner_fails_when_inputs_has_wrong_epoch() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    tracing::info!("producing input with wrong epoch index");
    let data = RollupsData::AdvanceStateInput(RollupsAdvanceStateInput {
        metadata: InputMetadata {
            input_index: 0,
            ..Default::default()
        },
        payload: Default::default(),
        tx_hash: Hash::default(),
    });
    let input = RollupsInput {
        parent_id: INITIAL_ID.to_owned(),
        epoch_index: 1,
        inputs_sent_count: 1,
        data,
    };
    state.broker.produce_raw_input_event(input).await;

    tracing::info!("waiting for the advance_runner to exit with error");
    let err = state.advance_runner.wait_err().await;
    assert!(format!("{:?}", err).contains("incorrect active epoch index"));
}

#[test_log::test(tokio::test)]
async fn test_advance_runner_fails_when_inputs_has_wrong_parent_id() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    tracing::info!("producing input with wrong parent id");
    let data = RollupsData::AdvanceStateInput(RollupsAdvanceStateInput {
        metadata: InputMetadata {
            input_index: 0,
            ..Default::default()
        },
        payload: Default::default(),
        tx_hash: Hash::default(),
    });
    let input = RollupsInput {
        parent_id: "invalid".to_owned(),
        epoch_index: 0,
        inputs_sent_count: 1,
        data,
    };
    state.broker.produce_raw_input_event(input).await;

    tracing::info!("waiting for the advance_runner to exit with error");
    let _err = state.advance_runner.wait_err().await;
    assert!(matches!(
        advance_runner::AdvanceRunnerError::RunnerSnapshotDisabledError {
            source:
                advance_runner::runner::RunnerError::ParentIdMismatchError {
                    expected: "0".to_owned(),
                    got: "invalid".to_owned()
                }
        },
        _err
    ));
}

#[test_log::test(tokio::test)]
async fn test_advance_runner_generates_claim_after_finishing_epoch() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    const N: usize = 3;
    tracing::info!("producing {} finish epoch events", N);
    for i in 0..N {
        let advance =
            RollupsData::AdvanceStateInput(RollupsAdvanceStateInput {
                metadata: InputMetadata {
                    input_index: i as u64,
                    ..Default::default()
                },
                payload: generate_payload(),
                tx_hash: Hash::default(),
            });
        let finish = RollupsData::FinishEpoch {};
        state.broker.produce_input_event(advance).await;
        state.broker.produce_input_event(finish).await;
    }

    tracing::info!("waiting until the expected claims are generated");
    state.server_manager.assert_session_ready().await;
    let claims = state.broker.consume_n_claims(N).await;
    // We don't verify the claim hash because it is not the resposability of the
    // advance_runner and because it changes every time we update the Cartesi Machine.
    assert_eq!(claims.len(), N);
}

#[test_log::test(tokio::test)]
async fn test_advance_runner_finishes_epoch_when_it_has_no_inputs() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    tracing::info!("finishing epochs with no inputs");
    state
        .broker
        .produce_input_event(RollupsData::FinishEpoch {})
        .await;
    state
        .broker
        .produce_input_event(RollupsData::FinishEpoch {})
        .await;

    tracing::info!("waiting until second epoch is finished");
    state.server_manager.assert_epoch_finished(1).await;

    tracing::info!("checking it didn't produce claims");
    let claims = state.broker.consume_all_claims().await;
    assert_eq!(claims.len(), 0);
}

/// Send an input, an finish epoch, and another input.
/// After the second input is processed by the server-manager we know
/// for sure that the advance_runner finished processing the finish epoch.
/// We can't simply wait for the epoch to be finished because the advance_runner
/// still does tasks after that.
async fn finish_epoch_and_wait_for_next_input(state: &TestState<'_>) {
    tracing::info!("producing input, finish, and another input");
    let payload = generate_payload();
    let inputs = vec![
        RollupsData::AdvanceStateInput(RollupsAdvanceStateInput {
            metadata: InputMetadata {
                input_index: 0,
                ..Default::default()
            },
            payload: Default::default(),
            tx_hash: Hash::default(),
        }),
        RollupsData::FinishEpoch {},
        RollupsData::AdvanceStateInput(RollupsAdvanceStateInput {
            metadata: InputMetadata {
                input_index: 1,
                ..Default::default()
            },
            payload: payload.clone(),
            tx_hash: Hash::default(),
        }),
    ];
    for input in inputs {
        state.broker.produce_input_event(input).await;
    }

    tracing::info!("waiting until second input is processed");
    state.server_manager.assert_session_ready().await;
    state.server_manager.assert_epoch_status(0, 1).await;
    state
        .server_manager
        .assert_epoch_status_payloads(1, &vec![payload])
        .await;
}

#[test_log::test(tokio::test)]
async fn test_advance_runner_sends_inputs_after_finishing_epoch() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;
    finish_epoch_and_wait_for_next_input(&state).await;
}

#[test_log::test(tokio::test)]
async fn test_advance_runner_does_not_generate_duplicate_claim() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    tracing::info!("producing claim");
    let rollups_claim = RollupsClaim::default();
    state
        .broker
        .produce_rollups_claim(rollups_claim.clone())
        .await;

    finish_epoch_and_wait_for_next_input(&state).await;

    tracing::info!("getting all claims");
    let produced_claims = state.broker.consume_all_claims().await;
    assert_eq!(produced_claims.len(), 1);
    assert_eq!(produced_claims[0].epoch_hash, rollups_claim.epoch_hash);
}

#[test_log::test(tokio::test)]
async fn test_advance_runner_stores_snapshot_after_finishing_epoch() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    finish_epoch_and_wait_for_next_input(&state).await;

    tracing::info!("checking the snapshots dir");
    state.snapshots.assert_latest_snapshot(1, 1);
}

#[test_log::test(tokio::test)]
async fn test_advance_runner_restore_session_after_restart() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;

    finish_epoch_and_wait_for_next_input(&state).await;

    tracing::info!("restarting advance_runner");
    state.advance_runner.restart().await;

    tracing::info!("producing another input and checking");
    let input = RollupsData::AdvanceStateInput(RollupsAdvanceStateInput {
        metadata: InputMetadata {
            input_index: 2,
            ..Default::default()
        },
        payload: generate_payload(),
        tx_hash: Hash::default(),
    });
    state.broker.produce_input_event(input).await;
    state.server_manager.assert_epoch_status(1, 2).await;
}
