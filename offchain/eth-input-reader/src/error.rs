// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use axum::http::uri::InvalidUri;
use eth_state_client_lib::error::StateServerError;
use snafu::Snafu;
use std::net::AddrParseError;
use tonic::transport::Error as TonicError;

use crate::machine;

#[derive(Debug, Snafu)]
#[snafu(visibility(pub(crate)))]
pub enum EthInputReaderError {
    #[snafu(display("http server error"))]
    HttpServerError { source: hyper::Error },

    #[snafu(display("metrics address error"))]
    MetricsAddressError { source: AddrParseError },

    #[snafu(display("broker facade error"))]
    BrokerError {
        source: machine::rollups_broker::BrokerFacadeError,
    },

    #[snafu(display("connection error"))]
    ChannelError { source: InvalidUri },

    #[snafu(display("connection error"))]
    ConnectError { source: TonicError },

    #[snafu(display("state server error"))]
    StateServerError { source: StateServerError },

    #[snafu(whatever, display("{message}"))]
    Whatever {
        message: String,
        #[snafu(source(from(Box<dyn std::error::Error>, Some)))]
        source: Option<Box<dyn std::error::Error>>,
    },
}
