// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::controller::Controller;
use crate::hash::{Hash, HASH_SIZE};
use crate::merkle_tree::{
    complete::Tree, proof::Proof, Error as MerkleTreeError,
};
use crate::model::{
    AdvanceMetadata, AdvanceResult, AdvanceStateRequest, CompletionStatus,
    InspectStateRequest, InspectStatus, Notice, Report, Voucher,
};
use crate::proofs::compute_proofs;
use ethabi::ethereum_types::U256;
use ethabi::Token;
use grpc_interfaces::cartesi_machine::{
    Hash as GrpcHash, MerkleTreeProof as GrpcMerkleTreeProof, Void,
};
use grpc_interfaces::cartesi_server_manager::{
    processed_input::ProcessedInputOneOf, server_manager_server::ServerManager,
    AcceptedData, Address, AdvanceStateRequest as GrpcAdvanceStateRequest,
    CompletionStatus as GrpcCompletionStatus,
    DeleteEpochRequest as GrpcDeleteEpochRequest, EndSessionRequest,
    EpochState, FinishEpochRequest, FinishEpochResponse, GetEpochStatusRequest,
    GetEpochStatusResponse, GetSessionStatusRequest, GetSessionStatusResponse,
    GetStatusResponse, InspectStateRequest as GrpcInspectStateRequest,
    InspectStateResponse, Notice as GrpcNotice, OutputEnum,
    OutputValidityProof, ProcessedInput, Proof as GrpcProof,
    Report as GrpcReport, StartSessionRequest, StartSessionResponse,
    TaintStatus, Voucher as GrpcVoucher,
};
use grpc_interfaces::versioning::{GetVersionResponse, SemanticVersion};
use std::{collections::HashMap, sync::Arc};
use tokio::sync::Mutex;
use tonic::{Request, Response, Status};

pub struct ServerManagerService {
    controller: Controller,
    sessions: SessionManager,
}

impl ServerManagerService {
    pub fn new(controller: Controller) -> Self {
        Self {
            controller,
            sessions: SessionManager::new(),
        }
    }
}

#[tonic::async_trait]
impl ServerManager for ServerManagerService {
    async fn get_version(
        &self,
        _: Request<Void>,
    ) -> Result<Response<GetVersionResponse>, Status> {
        tracing::info!("received get_version");
        let response = GetVersionResponse {
            version: Some(SemanticVersion {
                major: 0,
                minor: 2,
                patch: 0,
                pre_release: String::from(""),
                build: String::from("host-runner"),
            }),
        };
        Ok(Response::new(response))
    }

    async fn start_session(
        &self,
        request: Request<StartSessionRequest>,
    ) -> Result<Response<StartSessionResponse>, Status> {
        let request = request.into_inner();
        tracing::info!("received start_session with id={}", request.session_id);
        self.sessions
            .try_set_session(
                request.session_id,
                request.active_epoch_index,
                request.processed_input_count,
                self.controller.clone(),
            )
            .await?;
        let response = StartSessionResponse { config: None };
        Ok(Response::new(response))
    }

    async fn end_session(
        &self,
        request: Request<EndSessionRequest>,
    ) -> Result<Response<Void>, Status> {
        let request = request.into_inner();
        tracing::info!("received end_session with id={}", request.session_id);
        self.sessions.try_del_session(&request.session_id).await?;
        Ok(Response::new(Void {}))
    }

    async fn advance_state(
        &self,
        request: Request<GrpcAdvanceStateRequest>,
    ) -> Result<Response<Void>, Status> {
        let request = request.into_inner();
        tracing::info!("received advance_state with id={}", request.session_id);
        let metadata = request
            .input_metadata
            .ok_or(Status::invalid_argument("missing metadata from request"))?;
        let msg_sender = metadata
            .msg_sender
            .ok_or(Status::invalid_argument(
                "missing msg_sender from metadata",
            ))?
            .data
            .try_into()
            .or(Err(Status::invalid_argument("invalid address")))?;
        if metadata.epoch_index != 0 {
            return Err(Status::invalid_argument(
                "metadata epoch index is deprecated and should always be 0",
            ));
        }
        if metadata.input_index != request.current_input_index {
            return Err(Status::invalid_argument(
                "metadata input index mismatch",
            ));
        }
        let advance_request = AdvanceStateRequest {
            metadata: AdvanceMetadata {
                msg_sender,
                epoch_index: metadata.epoch_index,
                input_index: metadata.input_index,
                block_number: metadata.block_number,
                timestamp: metadata.timestamp,
            },
            payload: request.input_payload,
        };
        self.sessions
            .try_get_session(&request.session_id)
            .await?
            .try_lock()
            .or(Err(Status::aborted("concurrent call in session")))?
            .try_advance(
                request.active_epoch_index,
                request.current_input_index,
                advance_request,
            )
            .await?;
        Ok(Response::new(Void {}))
    }

    async fn finish_epoch(
        &self,
        request: Request<FinishEpochRequest>,
    ) -> Result<Response<FinishEpochResponse>, Status> {
        let request = request.into_inner();
        tracing::info!("received finish_epoch with id={}", request.session_id);
        if !request.storage_directory.is_empty() {
            tracing::warn!("ignoring storage_directory parameter");
        }
        let response = self
            .sessions
            .try_get_session(&request.session_id)
            .await?
            .try_lock()
            .or(Err(Status::aborted("concurrent call in session")))?
            .try_finish_epoch(
                request.active_epoch_index,
                request.processed_input_count_within_epoch,
            )
            .await?;
        Ok(Response::new(response))
    }

    async fn inspect_state(
        &self,
        request: Request<GrpcInspectStateRequest>,
    ) -> Result<tonic::Response<InspectStateResponse>, Status> {
        let request = request.into_inner();
        tracing::info!("received inspect_state with id={}", request.session_id);
        self.sessions
            .try_get_session(&request.session_id)
            .await?
            .try_lock()
            .or(Err(Status::aborted("concurrent call in session")))?
            .try_inspect(request.session_id, request.query_payload)
            .await
            .map(Response::new)
    }

    async fn get_status(
        &self,
        _: Request<Void>,
    ) -> Result<Response<GetStatusResponse>, Status> {
        tracing::info!("received get_status");
        let session_id = self.sessions.get_sessions().await;
        Ok(Response::new(GetStatusResponse { session_id }))
    }

    async fn get_session_status(
        &self,
        request: Request<GetSessionStatusRequest>,
    ) -> Result<Response<GetSessionStatusResponse>, Status> {
        let request = request.into_inner();
        tracing::info!(
            "received get_session_status with id={}",
            request.session_id
        );
        let response = self
            .sessions
            .try_get_session(&request.session_id)
            .await?
            .try_lock()
            .or(Err(Status::aborted("concurrent call in session")))?
            .get_status(request.session_id)
            .await;
        Ok(Response::new(response))
    }

    async fn get_epoch_status(
        &self,
        request: Request<GetEpochStatusRequest>,
    ) -> Result<Response<GetEpochStatusResponse>, Status> {
        let request = request.into_inner();
        tracing::info!(
            "received get_epoch_status with id={} and epoch_index={}",
            request.session_id,
            request.epoch_index
        );
        let response = self
            .sessions
            .try_get_session(&request.session_id)
            .await?
            .try_lock()
            .or(Err(Status::aborted("concurrent call in session")))?
            .try_get_epoch_status(request.session_id, request.epoch_index)
            .await?;
        Ok(Response::new(response))
    }

    async fn delete_epoch(
        &self,
        request: Request<GrpcDeleteEpochRequest>,
    ) -> Result<Response<Void>, Status> {
        let request = request.into_inner();
        self.sessions
            .try_get_session(&request.session_id)
            .await?
            .try_lock()
            .or(Err(Status::aborted("concurrent call in session")))?
            .try_delete_epoch(request.epoch_index)
            .await?;
        Ok(Response::new(Void {}))
    }
}

struct SessionManager {
    entry: Mutex<Option<SessionEntry>>,
}

impl SessionManager {
    fn new() -> Self {
        Self {
            entry: Mutex::new(None),
        }
    }

    async fn try_set_session(
        &self,
        session_id: String,
        active_epoch_index: u64,
        processed_input_count: u64,
        controller: Controller,
    ) -> Result<(), Status> {
        if session_id.is_empty() {
            return Err(Status::invalid_argument("session id is empty"));
        }
        let mut entry = self.entry.lock().await;
        match *entry {
            Some(_) => {
                tracing::warn!(
                    "the host-runner only supports a single session"
                );
                Err(Status::already_exists("session id is taken"))
            }
            None => {
                *entry = Some(SessionEntry::new(
                    session_id,
                    active_epoch_index,
                    processed_input_count,
                    controller,
                ));
                Ok(())
            }
        }
    }

    async fn try_get_session(
        &self,
        request_id: &String,
    ) -> Result<Arc<Mutex<Session>>, Status> {
        self.entry
            .lock()
            .await
            .as_ref()
            .and_then(|entry| entry.get_session(request_id))
            .ok_or(Status::invalid_argument("session id not found"))
    }

    async fn try_del_session(&self, request_id: &String) -> Result<(), Status> {
        self.try_get_session(request_id)
            .await?
            .try_lock()
            .or(Err(Status::aborted("concurrent call in session")))?
            .check_endable()
            .await?;
        let mut entry = self.entry.lock().await;
        *entry = None;
        Ok(())
    }

    async fn get_sessions(&self) -> Vec<String> {
        let mut sessions = Vec::new();
        if let Some(entry) = self.entry.lock().await.as_ref() {
            sessions.push(entry.get_id());
        }
        sessions
    }
}

struct SessionEntry {
    id: String,
    session: Arc<Mutex<Session>>,
}

impl SessionEntry {
    fn new(
        id: String,
        active_epoch_index: u64,
        processed_input_count: u64,
        controller: Controller,
    ) -> Self {
        Self {
            id,
            session: Arc::new(Mutex::new(Session::new(
                active_epoch_index,
                processed_input_count,
                controller,
            ))),
        }
    }

    fn get_session(&self, request_id: &String) -> Option<Arc<Mutex<Session>>> {
        if &self.id == request_id {
            Some(self.session.clone())
        } else {
            None
        }
    }

    fn get_id(&self) -> String {
        self.id.clone()
    }
}

struct Session {
    active_epoch_index: u64,
    controller: Controller,
    epochs: HashMap<u64, Arc<Mutex<Epoch>>>,
    tainted: Arc<Mutex<Option<Status>>>,
}

impl Session {
    fn new(
        active_epoch_index: u64,
        processed_input_count: u64,
        controller: Controller,
    ) -> Self {
        let epoch = Arc::new(Mutex::new(Epoch::new(processed_input_count)));
        let mut epochs = HashMap::new();
        epochs.insert(active_epoch_index, epoch);
        Self {
            active_epoch_index,
            controller,
            epochs,
            tainted: Arc::new(Mutex::new(None)),
        }
    }

    async fn try_advance(
        &mut self,
        active_epoch_index: u64,
        current_input_index: u64,
        advance_request: AdvanceStateRequest,
    ) -> Result<(), Status> {
        self.check_epoch_index_overflow()?;
        self.check_tainted().await?;
        self.check_active_epoch(active_epoch_index)?;
        let epoch = self.try_get_epoch(active_epoch_index)?;
        epoch
            .lock()
            .await
            .try_add_pending_input(current_input_index)?;
        let rx = self.controller.advance(advance_request).await;
        let epoch = epoch.clone();
        let tainted = self.tainted.clone();
        // Handle the advance response in another thread
        tokio::spawn(async move {
            match rx.await {
                Ok(result) => {
                    if let Err(e) =
                        epoch.lock().await.add_processed_input(result)
                    {
                        tracing::error!(
                            "failed to add processed input; tainting session"
                        );
                        *tainted.lock().await = Some(e);
                    }
                }
                Err(_) => {
                    tracing::error!("sender dropped the channel");
                }
            }
        });
        Ok(())
    }

    async fn try_inspect(
        &mut self,
        session_id: String,
        payload: Vec<u8>,
    ) -> Result<InspectStateResponse, Status> {
        self.check_tainted().await?;
        let rx = self
            .controller
            .inspect(InspectStateRequest { payload })
            .await;
        let result = rx.await.map_err(|e| {
            tracing::error!("sender dropped the channel ({})", e);
            Status::internal("unexpected error during inspect")
        })?;
        let active_epoch_index = self.active_epoch_index;
        let epoch = self.try_get_epoch(active_epoch_index)?;
        let processed_input_count =
            epoch.lock().await.get_num_processed_inputs_since_genesis();
        Ok(InspectStateResponse {
            session_id,
            active_epoch_index,
            processed_input_count,
            status: (&result.status).into(),
            exception_data: match result.status {
                InspectStatus::Exception { exception } => {
                    Some(exception.payload)
                }
                _ => None,
            },
            reports: result.reports.into_iter().map(GrpcReport::from).collect(),
        })
    }

    async fn try_finish_epoch(
        &mut self,
        active_epoch_index: u64,
        processed_input_count_within_epoch: u64,
    ) -> Result<FinishEpochResponse, Status> {
        self.check_epoch_index_overflow()?;
        self.check_tainted().await?;
        self.check_active_epoch(active_epoch_index)?;
        let (response, processed_input_count_since_genesis) = {
            let mut last_epoch =
                self.try_get_epoch(active_epoch_index)?.lock().await;
            (
                last_epoch.try_finish(
                    processed_input_count_within_epoch,
                    active_epoch_index,
                )?,
                last_epoch.processed_input_count_since_genesis,
            )
        };
        self.active_epoch_index += 1;
        let epoch = Arc::new(Mutex::new(Epoch::new(
            processed_input_count_since_genesis
                + processed_input_count_within_epoch,
        )));
        self.epochs.insert(self.active_epoch_index, epoch);
        Ok(response)
    }

    async fn try_delete_epoch(
        &mut self,
        epoch_index: u64,
    ) -> Result<(), Status> {
        self.check_tainted().await?;
        self.try_get_epoch(epoch_index)?
            .lock()
            .await
            .check_finished()?;
        self.epochs.remove(&epoch_index);
        Ok(())
    }

    async fn get_status(&self, session_id: String) -> GetSessionStatusResponse {
        let mut epoch_index: Vec<u64> = self.epochs.keys().cloned().collect();
        epoch_index.sort();
        GetSessionStatusResponse {
            session_id,
            active_epoch_index: self.active_epoch_index,
            epoch_index,
            taint_status: self.get_taint_status().await,
        }
    }

    async fn get_taint_status(&self) -> Option<TaintStatus> {
        self.tainted
            .lock()
            .await
            .as_ref()
            .map(|status| TaintStatus {
                error_code: status.code() as i32,
                error_message: String::from(status.message()),
            })
    }

    async fn try_get_epoch_status(
        &self,
        session_id: String,
        epoch_index: u64,
    ) -> Result<GetEpochStatusResponse, Status> {
        let taint_status = self.get_taint_status().await;
        let response = self
            .try_get_epoch(epoch_index)?
            .lock()
            .await
            .get_status(session_id, epoch_index, taint_status);
        Ok(response)
    }

    fn try_get_epoch(
        &self,
        epoch_index: u64,
    ) -> Result<&Arc<Mutex<Epoch>>, Status> {
        self.epochs
            .get(&epoch_index)
            .ok_or(Status::invalid_argument("unknown epoch index"))
    }

    async fn check_endable(&self) -> Result<(), Status> {
        if self.tainted.lock().await.is_none() {
            self.try_get_epoch(self.active_epoch_index)?
                .lock()
                .await
                .check_endable()?;
        }
        Ok(())
    }

    async fn check_tainted(&self) -> Result<(), Status> {
        if self.tainted.lock().await.is_some() {
            Err(Status::data_loss("session is tainted"))
        } else {
            Ok(())
        }
    }

    fn check_epoch_index_overflow(&self) -> Result<(), Status> {
        if self.active_epoch_index == std::u64::MAX {
            Err(Status::out_of_range("active epoch index will overflow"))
        } else {
            Ok(())
        }
    }

    fn check_active_epoch(
        &self,
        active_epoch_index: u64,
    ) -> Result<(), Status> {
        if self.active_epoch_index != active_epoch_index {
            Err(Status::invalid_argument(format!(
                "incorrect active epoch index (expected {}, got {})",
                self.active_epoch_index, active_epoch_index
            )))
        } else {
            Ok(())
        }
    }
}

/// The keccak output has 32 bytes
const LOG2_KECCAK_SIZE: usize = 5;

/// The epoch tree has 2^32 leafs
const LOG2_ROOT_SIZE: usize = 32 + LOG2_KECCAK_SIZE;

/// The max number of inputs in an epoch is limited by the size of the merkle tree
const MAX_INPUTS_IN_EPOCH: usize = 1 << (LOG2_ROOT_SIZE - LOG2_KECCAK_SIZE);

#[derive(Debug)]
struct Epoch {
    state: EpochState,
    pending_inputs: u64,
    processed_inputs: Vec<AdvanceResult>,
    vouchers_tree: Tree,
    notices_tree: Tree,
    processed_input_count_since_genesis: u64,
}

impl Epoch {
    fn new(processed_input_count_since_genesis: u64) -> Self {
        Self {
            state: EpochState::Active,
            pending_inputs: 0,
            processed_inputs: vec![],
            vouchers_tree: Tree::new(
                LOG2_ROOT_SIZE,
                LOG2_KECCAK_SIZE,
                LOG2_KECCAK_SIZE,
            )
            .expect("cannot fail"),
            notices_tree: Tree::new(
                LOG2_ROOT_SIZE,
                LOG2_KECCAK_SIZE,
                LOG2_KECCAK_SIZE,
            )
            .expect("cannot fail"),
            processed_input_count_since_genesis,
        }
    }

    fn try_add_pending_input(
        &mut self,
        current_input_index: u64,
    ) -> Result<(), Status> {
        self.check_active()?;
        self.check_current_input_index(current_input_index)?;
        self.check_input_limit()?;
        self.pending_inputs += 1;
        Ok(())
    }

    fn add_processed_input(
        &mut self,
        mut result: AdvanceResult,
    ) -> Result<(), Status> {
        // Compute proofs and update vouchers and notices trees
        if let CompletionStatus::Accepted { vouchers, notices } =
            &mut result.status
        {
            let voucher_root = compute_proofs(vouchers)?;
            result.voucher_root = Some(voucher_root.clone());
            self.vouchers_tree.push(voucher_root)?;
            let notice_root = compute_proofs(notices)?;
            result.notice_root = Some(notice_root.clone());
            self.notices_tree.push(notice_root)?;
        } else {
            self.vouchers_tree.push(Hash::default())?;
            self.notices_tree.push(Hash::default())?;
        }
        // Setup proofs for the current result
        let address = (self.vouchers_tree.len() - 1) << LOG2_KECCAK_SIZE;
        result.voucher_hashes_in_epoch =
            Some(self.vouchers_tree.get_proof(address, LOG2_KECCAK_SIZE)?);
        result.notice_hashes_in_epoch =
            Some(self.notices_tree.get_proof(address, LOG2_KECCAK_SIZE)?);
        // Add result to processed inputs
        self.pending_inputs -= 1;
        self.processed_inputs.push(result);
        Ok(())
    }

    fn try_finish(
        &mut self,
        processed_input_count_within_epoch: u64,
        epoch_index: u64,
    ) -> Result<FinishEpochResponse, Status> {
        self.check_active()?;
        self.check_pending_inputs()?;
        self.check_processed_inputs(processed_input_count_within_epoch)?;
        self.state = EpochState::Finished;

        let machine_state_hash = GrpcHash {
            data: vec![0_u8; HASH_SIZE],
        };
        let mut proofs: Vec<GrpcProof> = vec![];
        let index = Token::Int(U256::from(epoch_index));
        let context = ethabi::encode(&[index]);

        for (local_input_index, result) in
            self.processed_inputs.iter_mut().enumerate()
        {
            let address = local_input_index << LOG2_KECCAK_SIZE;
            let voucher_hashes_in_epoch =
                self.vouchers_tree.get_proof(address, LOG2_KECCAK_SIZE)?;
            let notice_hashes_in_epoch =
                self.notices_tree.get_proof(address, LOG2_KECCAK_SIZE)?;
            let global_input_index = self.processed_input_count_since_genesis
                + local_input_index as u64;

            if let CompletionStatus::Accepted { vouchers, notices } =
                &mut result.status
            {
                // Create GrpcProof for each voucher
                for (output_index, voucher) in vouchers.iter().enumerate() {
                    proofs.push(GrpcProof {
                        input_index: global_input_index,
                        output_index: output_index as u64,
                        output_enum: OutputEnum::Voucher.into(),
                        // Create OutputValidityProof for each voucher
                        validity: Some(OutputValidityProof {
                            input_index_within_epoch: local_input_index as u64,
                            output_index_within_input: output_index as u64,
                            output_hashes_root_hash: Some(GrpcHash::from(
                                result.voucher_root.clone().expect(
                                    "expected voucher's root hash to exist",
                                ),
                            )),
                            vouchers_epoch_root_hash: Some(GrpcHash::from(
                                self.vouchers_tree.get_root_hash().clone(),
                            )),
                            notices_epoch_root_hash: Some(GrpcHash::from(
                                self.notices_tree.get_root_hash().clone(),
                            )),
                            machine_state_hash: Some(
                                machine_state_hash.clone(),
                            ),
                            output_hash_in_output_hashes_siblings: voucher
                                .keccak_in_voucher_hashes
                                .clone()
                                .expect("expected voucher proof to exist")
                                .sibling_hashes
                                .into_iter()
                                .map(GrpcHash::from)
                                .collect(),
                            output_hashes_in_epoch_siblings:
                                voucher_hashes_in_epoch
                                    .clone()
                                    .sibling_hashes
                                    .into_iter()
                                    .map(GrpcHash::from)
                                    .collect(),
                        }),
                        context: context.clone(),
                    })
                }
                // Create GrpcProof for each notice
                for (output_index, notice) in notices.iter().enumerate() {
                    proofs.push(GrpcProof {
                        input_index: global_input_index,
                        output_index: output_index as u64,
                        output_enum: OutputEnum::Notice.into(),
                        // Create OutputValidityProof for each notice
                        validity: Some(OutputValidityProof {
                            input_index_within_epoch: local_input_index as u64,
                            output_index_within_input: output_index as u64,
                            output_hashes_root_hash: Some(GrpcHash::from(
                                result.notice_root.clone().expect(
                                    "expected notice's root hash to exist",
                                ),
                            )),
                            vouchers_epoch_root_hash: Some(GrpcHash::from(
                                self.vouchers_tree.get_root_hash().clone(),
                            )),
                            notices_epoch_root_hash: Some(GrpcHash::from(
                                self.notices_tree.get_root_hash().clone(),
                            )),
                            machine_state_hash: Some(
                                machine_state_hash.clone(),
                            ),
                            output_hash_in_output_hashes_siblings: notice
                                .keccak_in_notice_hashes
                                .clone()
                                .expect("expected notice proof to exist")
                                .sibling_hashes
                                .into_iter()
                                .map(GrpcHash::from)
                                .collect(),
                            output_hashes_in_epoch_siblings:
                                notice_hashes_in_epoch
                                    .clone()
                                    .sibling_hashes
                                    .into_iter()
                                    .map(GrpcHash::from)
                                    .collect(),
                        }),
                        context: context.clone(),
                    })
                }
            }
        }

        Ok(FinishEpochResponse {
            machine_hash: Some(machine_state_hash.clone()),
            vouchers_epoch_root_hash: Some(GrpcHash::from(
                self.vouchers_tree.get_root_hash().clone(),
            )),
            notices_epoch_root_hash: Some(GrpcHash::from(
                self.notices_tree.get_root_hash().clone(),
            )),
            proofs,
        })
    }

    fn get_status(
        &self,
        session_id: String,
        epoch_index: u64,
        taint_status: Option<TaintStatus>,
    ) -> GetEpochStatusResponse {
        let processed_inputs = self
            .processed_inputs
            .iter()
            .cloned()
            .enumerate()
            .map(|(local_input_index, input)| {
                let input_index = local_input_index as u64
                    + self.processed_input_count_since_genesis;
                ProcessedInput {
                    input_index,
                    status: (&input.status).into(),
                    processed_input_one_of: input.status.into(),
                    reports: input
                        .reports
                        .into_iter()
                        .map(GrpcReport::from)
                        .collect(),
                }
            })
            .collect();
        GetEpochStatusResponse {
            session_id,
            epoch_index,
            state: self.state as i32,
            processed_inputs,
            pending_input_count: self.pending_inputs,
            taint_status,
        }
    }

    fn get_num_processed_inputs_within_epoch(&self) -> u64 {
        self.processed_inputs.len() as u64
    }

    fn get_num_processed_inputs_since_genesis(&self) -> u64 {
        self.get_num_processed_inputs_within_epoch()
            + self.processed_input_count_since_genesis
    }

    fn get_current_input_index(&self) -> u64 {
        self.pending_inputs + self.get_num_processed_inputs_since_genesis()
    }

    fn check_endable(&self) -> Result<(), Status> {
        self.check_pending_inputs()?;
        self.check_no_processed_inputs()?;
        Ok(())
    }

    fn check_active(&self) -> Result<(), Status> {
        if self.state != EpochState::Active {
            Err(Status::invalid_argument("epoch is finished"))
        } else {
            Ok(())
        }
    }

    fn check_finished(&self) -> Result<(), Status> {
        match self.check_active() {
            Ok(_) => Err(Status::invalid_argument("epoch is not finished")),
            Err(_) => Ok(()),
        }
    }

    fn check_current_input_index(
        &self,
        current_input_index: u64,
    ) -> Result<(), Status> {
        let epoch_current_input_index = self.get_current_input_index();
        if epoch_current_input_index != current_input_index {
            Err(Status::invalid_argument(format!(
                "incorrect current input index (expected {}, got {})",
                epoch_current_input_index, current_input_index
            )))
        } else {
            Ok(())
        }
    }

    fn check_pending_inputs(&self) -> Result<(), Status> {
        if self.pending_inputs != 0 {
            Err(Status::invalid_argument("epoch still has pending inputs"))
        } else {
            Ok(())
        }
    }

    fn check_processed_inputs(
        &self,
        processed_input_count_within_epoch: u64,
    ) -> Result<(), Status> {
        if self.get_num_processed_inputs_within_epoch()
            != processed_input_count_within_epoch
        {
            Err(Status::invalid_argument(format!(
                "incorrect processed input count (expected {}, got {})",
                self.get_num_processed_inputs_within_epoch(),
                processed_input_count_within_epoch
            )))
        } else {
            Ok(())
        }
    }

    fn check_no_processed_inputs(&self) -> Result<(), Status> {
        if self.get_num_processed_inputs_within_epoch() != 0 {
            Err(Status::invalid_argument("epoch still has processed inputs"))
        } else {
            Ok(())
        }
    }

    fn check_input_limit(&self) -> Result<(), Status> {
        if self.pending_inputs
            + self.get_num_processed_inputs_within_epoch()
            + 1
            >= MAX_INPUTS_IN_EPOCH as u64
        {
            Err(Status::invalid_argument(
                "reached max number of inputs per epoch",
            ))
        } else {
            Ok(())
        }
    }
}

impl From<&CompletionStatus> for i32 {
    fn from(status: &CompletionStatus) -> i32 {
        let status = match status {
            CompletionStatus::Accepted { .. } => GrpcCompletionStatus::Accepted,
            CompletionStatus::Rejected => GrpcCompletionStatus::Rejected,
            CompletionStatus::Exception { .. } => {
                GrpcCompletionStatus::Exception
            }
        };
        status as i32
    }
}

impl From<&InspectStatus> for i32 {
    fn from(status: &InspectStatus) -> i32 {
        let status = match status {
            InspectStatus::Accepted => GrpcCompletionStatus::Accepted,
            InspectStatus::Rejected => GrpcCompletionStatus::Rejected,
            InspectStatus::Exception { .. } => GrpcCompletionStatus::Exception,
        };
        status as i32
    }
}

impl From<CompletionStatus> for Option<ProcessedInputOneOf> {
    fn from(status: CompletionStatus) -> Option<ProcessedInputOneOf> {
        match status {
            CompletionStatus::Accepted { vouchers, notices } => {
                Some(ProcessedInputOneOf::AcceptedData(AcceptedData {
                    vouchers: vouchers
                        .into_iter()
                        .map(GrpcVoucher::from)
                        .collect(),
                    notices: notices
                        .into_iter()
                        .map(GrpcNotice::from)
                        .collect(),
                }))
            }
            CompletionStatus::Rejected => None,
            CompletionStatus::Exception { exception } => {
                Some(ProcessedInputOneOf::ExceptionData(exception.payload))
            }
        }
    }
}

impl From<Voucher> for GrpcVoucher {
    fn from(voucher: Voucher) -> GrpcVoucher {
        GrpcVoucher {
            destination: Some(Address {
                data: voucher.destination.into(),
            }),
            payload: voucher.payload,
        }
    }
}

impl From<Notice> for GrpcNotice {
    fn from(notice: Notice) -> GrpcNotice {
        GrpcNotice {
            payload: notice.payload,
        }
    }
}

impl From<Report> for GrpcReport {
    fn from(report: Report) -> GrpcReport {
        GrpcReport {
            payload: report.payload,
        }
    }
}

impl From<Hash> for GrpcHash {
    fn from(hash: Hash) -> GrpcHash {
        GrpcHash { data: hash.into() }
    }
}

impl From<Proof> for GrpcMerkleTreeProof {
    fn from(proof: Proof) -> GrpcMerkleTreeProof {
        GrpcMerkleTreeProof {
            target_address: proof.target_address as u64,
            log2_target_size: proof.log2_target_size as u64,
            target_hash: Some(proof.target_hash.into()),
            log2_root_size: proof.log2_root_size as u64,
            root_hash: Some(proof.root_hash.into()),
            sibling_hashes: proof
                .sibling_hashes
                .into_iter()
                .map(GrpcHash::from)
                .collect(),
        }
    }
}

impl From<MerkleTreeError> for Status {
    fn from(e: MerkleTreeError) -> Status {
        Status::internal(format!(
            "unexpected error when updating merkle tree ({})",
            e
        ))
    }
}
