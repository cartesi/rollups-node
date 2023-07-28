// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use serde_json;
use snafu::Snafu;

use grpc_interfaces::conversions::{
    MessageConversionError, StateConversionError,
};

#[derive(Debug, Snafu)]
#[snafu(visibility(pub))]
pub enum StateServerError {
    #[snafu(display("Tonic error in {}: {}", context, source))]
    TonicError {
        context: String,
        source: tonic::Status,
    },

    #[snafu(display("Transport error: {}", source))]
    TransportError { source: tonic::transport::Error },

    #[snafu(display("Serialize error: {}", source))]
    SerializeError { source: serde_json::Error },

    #[snafu(display("Message conversion error in {}: {}", context, source))]
    MessageConversion {
        context: String,
        source: MessageConversionError,
    },

    #[snafu(display("State conversion error in {}: {}", context, source))]
    StateConversion {
        context: String,
        source: StateConversionError,
    },
}

pub type Result<T> = std::result::Result<T, StateServerError>;
