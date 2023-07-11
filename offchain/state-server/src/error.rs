// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use eth_block_history::BlockArchiveError;
use eth_state_fold_types::ethers::providers::{Http, RetryClient};
use snafu::Snafu;
use tonic::transport::Error as TonicError;
use url::ParseError;

#[derive(Debug, Snafu)]
#[snafu(visibility(pub(crate)))]
#[allow(clippy::enum_variant_names)]
pub enum StateServerError {
    #[snafu(display("tonic error"))]
    TonicError { source: TonicError },

    #[snafu(display("parser error"))]
    ParserError { source: ParseError },

    #[snafu(display("block archive error"))]
    BlockArchiveError {
        source: BlockArchiveError<
            eth_state_fold_types::ethers::providers::Provider<
                RetryClient<Http>,
            >,
        >,
    },
}
