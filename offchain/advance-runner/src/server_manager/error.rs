// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use snafu::Snafu;

#[derive(Debug, Snafu)]
#[snafu(visibility(pub(super)))]
#[allow(clippy::enum_variant_names)]
pub enum ServerManagerError {
    #[snafu(display("failed to connect to server-manager"))]
    ConnectionError { source: tonic::transport::Error },

    #[snafu(display(
        "failed to call {} with request-id {}",
        method,
        request_id
    ))]
    MethodCallError {
        method: String,
        request_id: String,
        source: tonic::Status,
    },

    #[snafu(display("maximum number of retries exceeded"))]
    PendingInputsExceededError {},

    #[snafu(display("missing field {}", name))]
    MissingFieldError { name: String },

    #[snafu(display(
        "array of wrong size for {} type, expected {} but got {}",
        name,
        expected,
        got
    ))]
    WrongArraySizeError {
        name: String,
        expected: usize,
        got: usize,
    },

    #[snafu(display("missing processed input in get epoch status"))]
    MissingProcessedInputError {},

    #[snafu(display(
        "invalid last processed input index, expected {} but got {}",
        expected,
        got
    ))]
    InvalidProcessedInputError { expected: u64, got: u64 },

    #[snafu(display(
        "can't generate claim for epoch {} because it has no inputs",
        epoch_index
    ))]
    EmptyEpochError { epoch_index: u64 },
}
