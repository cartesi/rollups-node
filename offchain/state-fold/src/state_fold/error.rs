// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::Foldable;

use crate::block_history::BlockArchiveError;

use ethers::providers::{FromErr, Middleware};
use snafu::Snafu;

#[derive(Debug, Snafu)]
#[snafu(visibility(pub))]
pub enum FoldableError<M: Middleware + 'static, F: Foldable + 'static> {
    #[snafu(display("Inner error: {}", source))]
    InnerError { source: F::Error },

    #[snafu(display("Middleware error: {}", source))]
    MiddlewareError { source: M::Error },

    #[snafu(display("Block Archive error: {}", source))]
    BlockArchiveError { source: BlockArchiveError<M> },

    #[snafu(display("Requested block unavailable"))]
    BlockUnavailable {},

    #[snafu(display("Requested log unavailable"))]
    LogUnavailable {},

    #[snafu(display("Partition error: {:?}", sources))]
    PartitionError { sources: Vec<M::Error> },
}

impl<M: Middleware, F: Foldable> FromErr<M::Error> for FoldableError<M, F> {
    fn from(source: M::Error) -> Self {
        FoldableError::MiddlewareError { source }
    }
}
