// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use backoff::{future::retry, Error, ExponentialBackoff};
use rollups_events::{
    InputMetadata as RollupsInputMetadata, Payload, RollupsClaim,
    RollupsNotice, RollupsOutput, RollupsReport, RollupsVoucher,
};
use snafu::{OptionExt, ResultExt};
use std::path::Path;
use tonic::{transport::Channel, Request};
use uuid::Uuid;

use grpc_interfaces::cartesi_machine::Void;
use grpc_interfaces::cartesi_server_manager::server_manager_client::ServerManagerClient;
use grpc_interfaces::cartesi_server_manager::{
    processed_input::ProcessedInputOneOf, Address, AdvanceStateRequest,
    EndSessionRequest, FinishEpochRequest, GetEpochStatusRequest,
    GetSessionStatusRequest, InputMetadata, ProcessedInput,
    StartSessionRequest,
};

use super::claim::compute_epoch_hash;
use super::config::ServerManagerConfig;
use super::conversions::{
    convert_address, convert_hash, convert_proof, get_field,
};
use super::error::{
    ConnectionSnafu, EmptyEpochSnafu, InvalidProcessedInputSnafu,
    ServerManagerError,
};

/// Call the grpc method passing an unique request-id and with retry
macro_rules! grpc_call {
    ($self: ident, $method: ident, $request: expr) => {
        retry($self.backoff.clone(), || async {
            let request_id = Uuid::new_v4().to_string();
            let request = $request;

            tracing::trace!(
                request_id,
                method = stringify!($method),
                ?request,
                "calling grpc"
            );

            let mut grpc_request = Request::new(request);
            grpc_request
                .metadata_mut()
                .insert("request-id", request_id.parse().unwrap());

            let response = $self.client.clone().$method(grpc_request).await;

            tracing::trace!(
                request_id,
                method = stringify!($method),
                ?response,
                "got grpc response",
            );

            response.map(|v| v.into_inner()).map_err(|status| {
                let err_type = match status.code() {
                    tonic::Code::InvalidArgument => Error::Permanent,
                    tonic::Code::NotFound => Error::Permanent,
                    tonic::Code::AlreadyExists => Error::Permanent,
                    tonic::Code::FailedPrecondition => Error::Permanent,
                    tonic::Code::OutOfRange => Error::Permanent,
                    tonic::Code::Unimplemented => Error::Permanent,
                    tonic::Code::DataLoss => Error::Permanent,
                    _ => Error::transient,
                };
                err_type(ServerManagerError::MethodCallError {
                    source: status,
                    method: stringify!($method).to_owned(),
                    request_id,
                })
            })
        })
        .await
    };
}

pub type Result<T> = std::result::Result<T, ServerManagerError>;

pub struct ServerManagerFacade {
    client: ServerManagerClient<Channel>,
    config: ServerManagerConfig,
    backoff: ExponentialBackoff,
}

impl ServerManagerFacade {
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn new(
        config: ServerManagerConfig,
        backoff: ExponentialBackoff,
    ) -> Result<Self> {
        tracing::trace!(?config, "connecting to server manager");

        let client = retry(backoff.clone(), || async {
            ServerManagerClient::connect(config.server_manager_endpoint.clone())
                .await
                .map_err(Error::transient)
        })
        .await
        .context(ConnectionSnafu)?;

        Ok(Self {
            client,
            config,
            backoff,
        })
    }

    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn start_session(
        &mut self,
        machine_directory: &Path,
        active_epoch_index: u64,
        processed_input_count: u64,
    ) -> Result<()> {
        tracing::trace!(
            ?machine_directory,
            active_epoch_index,
            "starting server-manager session"
        );

        // If session exists, delete it before creating new one
        let response = grpc_call!(self, get_status, Void {})?;
        if response.session_id.contains(&self.config.session_id) {
            tracing::warn!("deleting previous server-manager session");
            let session_status = grpc_call!(
                self,
                get_session_status,
                GetSessionStatusRequest {
                    session_id: self.config.session_id.clone(),
                }
            )?;
            let active_epoch_index = session_status.active_epoch_index;
            let processed_input_count_within_epoch =
                self.wait_for_pending_inputs(active_epoch_index)
                    .await?
                    .len() as u64;
            grpc_call!(
                self,
                finish_epoch,
                FinishEpochRequest {
                    session_id: self.config.session_id.clone(),
                    active_epoch_index,
                    processed_input_count_within_epoch,
                    storage_directory: "".to_string(),
                }
            )?;
            grpc_call!(
                self,
                end_session,
                EndSessionRequest {
                    session_id: self.config.session_id.clone(),
                }
            )?;
        }

        grpc_call!(self, start_session, {
            StartSessionRequest {
                session_id: self.config.session_id.clone(),
                machine_directory: machine_directory.to_string_lossy().into(),
                runtime: Some(self.config.runtime_config.clone()),
                active_epoch_index,
                processed_input_count,
                server_cycles: Some(self.config.cycles_config.clone()),
                server_deadline: Some(self.config.deadline_config.clone()),
            }
        })?;

        Ok(())
    }

    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn advance_state(
        &mut self,
        active_epoch_index: u64,
        current_input_index: u64,
        input_metadata: RollupsInputMetadata,
        input_payload: Vec<u8>,
    ) -> Result<Vec<RollupsOutput>> {
        tracing::trace!("sending advance-state input to server-manager");

        grpc_call!(self, advance_state, {
            let input_metadata = InputMetadata {
                msg_sender: Some(Address {
                    data: (*input_metadata.msg_sender.inner()).into(),
                }),
                block_number: input_metadata.block_number,
                timestamp: input_metadata.timestamp,
                epoch_index: input_metadata.epoch_index,
                input_index: input_metadata.input_index,
            };
            AdvanceStateRequest {
                session_id: self.config.session_id.to_owned(),
                active_epoch_index,
                current_input_index,
                input_metadata: Some(input_metadata),
                input_payload: input_payload.clone(),
            }
        })?;

        tracing::trace!("waiting until the input is processed");

        let processed_input = self
            .wait_for_pending_inputs(active_epoch_index)
            .await?
            .pop()
            .ok_or(ServerManagerError::MissingProcessedInputError {})?;
        snafu::ensure!(
            processed_input.input_index == current_input_index,
            InvalidProcessedInputSnafu {
                expected: current_input_index,
                got: processed_input.input_index,
            }
        );

        tracing::trace!("getting outputs");

        let mut outputs = vec![];

        for (index, report) in processed_input.reports.into_iter().enumerate() {
            let report = RollupsReport {
                index: index as u64,
                input_index: current_input_index,
                payload: Payload::new(report.payload),
            };
            outputs.push(RollupsOutput::Report(report));
        }

        if let Some(one_of) = processed_input.processed_input_one_of {
            match one_of {
                ProcessedInputOneOf::AcceptedData(data) => {
                    for (index, voucher) in
                        data.vouchers.into_iter().enumerate()
                    {
                        let destination =
                            convert_address(get_field!(voucher.destination))?;
                        let voucher = RollupsVoucher {
                            index: index as u64,
                            input_index: current_input_index,
                            payload: Payload::new(voucher.payload),
                            destination,
                        };
                        outputs.push(RollupsOutput::Voucher(voucher));
                    }
                    for (index, notice) in data.notices.into_iter().enumerate()
                    {
                        let notice = RollupsNotice {
                            index: index as u64,
                            input_index: current_input_index,
                            payload: Payload::new(notice.payload),
                        };
                        outputs.push(RollupsOutput::Notice(notice));
                    }
                }
                _ => {
                    tracing::trace!("ignoring input not accepted");
                }
            }
        }

        tracing::trace!(?outputs, "got outputs from epoch status");

        Ok(outputs)
    }

    /// Send a finish-epoch request to the server-manager
    /// Return the epoch claim and the proofs
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn finish_epoch(
        &mut self,
        epoch_index: u64,
        storage_directory: &Path,
    ) -> Result<(RollupsClaim, Vec<RollupsOutput>)> {
        tracing::trace!(epoch_index, "sending finish epoch");

        // Wait for pending inputs before sending a finish request
        let processed_inputs =
            self.wait_for_pending_inputs(epoch_index).await?;
        let processed_input_count_within_epoch = processed_inputs.len() as u64;

        let response = grpc_call!(self, finish_epoch, {
            FinishEpochRequest {
                session_id: self.config.session_id.to_owned(),
                active_epoch_index: epoch_index,
                processed_input_count_within_epoch,
                storage_directory: storage_directory
                    .to_string_lossy()
                    .to_string(),
            }
        })?;

        // Only try to get first and last after making the finish epoch request
        let (first_input, last_input) = processed_inputs
            .first()
            .zip(processed_inputs.last())
            .context(EmptyEpochSnafu { epoch_index })?;

        let vouchers_metadata_hash =
            convert_hash(get_field!(response.vouchers_epoch_root_hash))?;
        let notices_metadata_hash =
            convert_hash(get_field!(response.notices_epoch_root_hash))?;
        let machine_state_hash =
            convert_hash(get_field!(response.machine_hash))?;
        let epoch_hash = compute_epoch_hash(
            &vouchers_metadata_hash,
            &notices_metadata_hash,
            &machine_state_hash,
        );
        tracing::trace!(?epoch_hash, "computed epoch hash");

        let mut proofs = vec![];
        for proof in response.proofs {
            let proof = convert_proof(proof)?;
            proofs.push(RollupsOutput::Proof(proof));
        }
        tracing::trace!(?proofs, "got proofs");

        let rollups_claim = RollupsClaim {
            epoch_index,
            epoch_hash,
            first_index: first_input.input_index as u128,
            last_index: last_input.input_index as u128,
        };

        Ok((rollups_claim, proofs))
    }

    /// Wait until the server-manager processes all pending inputs
    /// Return the list of processed inputs for the given epoch
    #[tracing::instrument(level = "trace", skip_all)]
    async fn wait_for_pending_inputs(
        &mut self,
        epoch_index: u64,
    ) -> Result<Vec<ProcessedInput>> {
        tracing::trace!(epoch_index, "waiting for pending inputs");

        for _ in 0..self.config.pending_inputs_max_retries {
            let response = grpc_call!(self, get_epoch_status, {
                GetEpochStatusRequest {
                    session_id: self.config.session_id.to_owned(),
                    epoch_index,
                }
            })?;
            if response.pending_input_count > 0 {
                let duration = std::time::Duration::from_millis(
                    self.config.pending_inputs_sleep_duration,
                );
                tracing::debug!(
                    "server-manager has {} pending inputs; sleeping for {} ms",
                    response.pending_input_count,
                    duration.as_millis(),
                );
                tokio::time::sleep(duration).await;
            } else {
                return Ok(response.processed_inputs);
            }
        }

        tracing::warn!(
            "the number of retries while waiting for pending inputs exceeded"
        );

        Err(ServerManagerError::PendingInputsExceededError {})
    }
}
