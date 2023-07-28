// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::state_fold::delegate_access::{FoldMiddleware, SyncMiddleware};
use crate::state_fold::error::*;
use crate::state_fold::Foldable;

use super::global_archive::GlobalArchive;

use crate::block_history::{
    current_block_number, fetch_block, fetch_block_at_depth, BlockArchive,
    BlockArchiveError,
};
use state_fold_types::{Block, BlockState, QueryBlock};

use ethers::core::types::{BlockId, BlockNumber, H256, U64};
use ethers::providers::Middleware;

use snafu::ResultExt;
use std::sync::Arc;

pub struct StateFoldEnvironment<M: Middleware, UD> {
    inner_middleware: Arc<M>,
    pub block_archive: Option<Arc<BlockArchive<M>>>,

    genesis_block: U64,
    pub safety_margin: usize,

    // If the Ethereum node has a limit on the number of events returned by the
    // method `eth_getLogs` (such as Infura, with a 10k events limit and <10s
    // query limit), `query_limit_error_codes` contains the error codes of when
    // the request fails. In case of a match, Access will attempt a partition.
    // In case of Infura, the error code is `-32005`. An empty array means a
    // partition will never be attempted.
    query_limit_error_codes: Vec<i32>,

    // When attempting a partition, the number of concurrent fetches is bounded
    // by `concurrent_events_fetch + 1`. We recommend something like `16`.
    concurrent_events_fetch: usize,

    /// Maximum events allowed to be in a single provider response. If response
    /// event number reaches over this number, the request must be split into
    /// sub-ranges retried on each of them separately.
    ///
    /// Motivation for this configuration parameter mainly comes from Alchemy
    /// as past a certain limit it responds with invalid data.
    maximum_events_per_response: usize,

    global_archive: GlobalArchive,

    user_data: UD,
}

impl<M: Middleware + 'static, UD> StateFoldEnvironment<M, UD> {
    pub fn new(
        inner_middleware: Arc<M>,
        block_archive: Option<Arc<BlockArchive<M>>>,
        safety_margin: usize,
        genesis_block: U64,
        query_limit_error_codes: Vec<i32>,
        concurrent_events_fetch: usize,
        maximum_events_per_response: usize,
        user_data: UD,
    ) -> Self {
        let global_archive = GlobalArchive::new(safety_margin);

        Self {
            inner_middleware,
            block_archive,
            safety_margin,
            genesis_block,
            query_limit_error_codes,
            concurrent_events_fetch,
            maximum_events_per_response,
            global_archive,
            user_data,
        }
    }

    pub fn user_data(&self) -> &UD {
        &self.user_data
    }

    pub fn inner_middleware(&self) -> Arc<M> {
        self.inner_middleware.clone()
    }

    pub async fn get_state_for_block<
        F: Foldable<UserData = UD> + Send + Sync + 'static,
    >(
        &self,
        initial_state: &F::InitialState,
        fold_block: QueryBlock,
    ) -> Result<BlockState<F>, FoldableError<M, F>> {
        let archive = self.global_archive.get_archive::<F>().await;
        let train = archive.get_train(initial_state).await;

        // First check if block exists in archive, returning it if so. This is
        // an optimization and can be removed. The following code will be able
        // to get the requested block regardless. By doing this, we won't need
        // to instantiate an unnecessary provider and we avoid running on mutex
        // locks. This match also reduces unecessary `get_block` queries.
        let block = match fold_block {
            QueryBlock::Latest => {
                self.current_block().await.context(BlockArchiveSnafu)?
            }

            QueryBlock::BlockHash(hash) => self
                .block_with_hash(&hash)
                .await
                .context(BlockArchiveSnafu)?,

            QueryBlock::BlockNumber(n) => {
                self.block_with_number(n).await.context(BlockArchiveSnafu)?
            }

            QueryBlock::BlockDepth(depth) => self
                .block_at_depth(depth)
                .await
                .context(BlockArchiveSnafu)?,

            QueryBlock::Block(b) => b,
        };

        // Check if exists in archive.
        if let Some(block_state) =
            train.get_block_state(Arc::clone(&block)).await
        {
            return Ok(block_state);
        }

        // If it's not on archive, do the actual work. This method has an
        // internal lock, which makes concurrent calls mutually exclusive, to
        // avoid replicated work.
        train.fetch_block_state(self, block).await
    }
}

///
/// Internals

impl<M: Middleware + 'static, UD> StateFoldEnvironment<M, UD> {
    pub(crate) fn sync_access(&self, block: &Block) -> Arc<SyncMiddleware<M>> {
        let middleware = SyncMiddleware::new(
            Arc::clone(&self.inner_middleware),
            self.genesis_block,
            block.number,
            self.query_limit_error_codes.clone(),
            self.concurrent_events_fetch,
            self.maximum_events_per_response,
        );

        Arc::new(middleware)
    }

    pub(crate) fn fold_access(&self, block: &Block) -> Arc<FoldMiddleware<M>> {
        let middleware =
            FoldMiddleware::new(Arc::clone(&self.inner_middleware), block.hash);
        Arc::new(middleware)
    }

    pub(crate) async fn current_block_number(
        &self,
    ) -> Result<U64, BlockArchiveError<M>> {
        if let Some(a) = &self.block_archive {
            Ok(a.latest_block().await.number)
        } else {
            current_block_number(self.inner_middleware.as_ref()).await
        }
    }

    pub(crate) async fn current_block(
        &self,
    ) -> Result<Arc<Block>, BlockArchiveError<M>> {
        if let Some(a) = &self.block_archive {
            Ok(a.latest_block().await)
        } else {
            self.block(BlockNumber::Latest).await
        }
    }

    pub async fn block_with_hash(
        &self,
        hash: &H256,
    ) -> Result<Arc<Block>, BlockArchiveError<M>> {
        if let Some(a) = &self.block_archive {
            a.block_with_hash(hash).await
        } else {
            self.block(*hash).await
        }
    }

    pub async fn block_with_number(
        &self,
        number: U64,
    ) -> Result<Arc<Block>, BlockArchiveError<M>> {
        if let Some(a) = &self.block_archive {
            a.block_with_number(number).await
        } else {
            self.block(number).await
        }
    }

    pub(crate) async fn block_at_depth(
        &self,
        depth: usize,
    ) -> Result<Arc<Block>, BlockArchiveError<M>> {
        if let Some(a) = &self.block_archive {
            a.block_at_depth(depth).await
        } else {
            let current = self.current_block_number().await?;
            self.fetch_block_at_depth(current, depth).await
        }
    }

    async fn block<T: Into<BlockId> + Send + Sync>(
        &self,
        block: T,
    ) -> Result<Arc<Block>, BlockArchiveError<M>> {
        Ok(Arc::new(
            fetch_block(self.inner_middleware.as_ref(), block).await?,
        ))
    }

    async fn fetch_block_at_depth(
        &self,
        current: U64,
        depth: usize,
    ) -> Result<Arc<Block>, BlockArchiveError<M>> {
        Ok(Arc::new(
            fetch_block_at_depth(
                self.inner_middleware.as_ref(),
                current,
                depth,
            )
            .await?,
        ))
    }
}
