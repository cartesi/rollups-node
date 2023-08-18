// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use axum::http::uri::InvalidUri;
use eth_state_fold_types::ethers::providers::{
    Http, Provider, ProviderError, RetryClient,
};
use snafu::Snafu;
use std::net::AddrParseError;
use tonic::transport::Error as TonicError;
use url::ParseError;

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

    #[snafu(display("provider error"))]
    ProviderError { source: ProviderError },

    #[snafu(display("parser error"))]
    ParseError { source: ParseError },

    #[snafu(display("Provider didn't return chain id"))]
    MissingChainId,

    #[snafu(display("parser error"))]
    BlockArchiveError {
        source:
            eth_block_history::BlockArchiveError<Provider<RetryClient<Http>>>,
    },

    #[snafu(whatever, display("{message}"))]
    Whatever {
        message: String,
        #[snafu(source(from(Box<dyn std::error::Error>, Some)))]
        source: Option<Box<dyn std::error::Error>>,
    },
}
