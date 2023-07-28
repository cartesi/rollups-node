// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::error::Result;

use ethers::core::types::H256;

use state_fold_types::ethers;
use state_fold_types::{
    Block, BlockState, BlockStreamItem, BlocksSince, QueryBlock,
    StateStreamItem, StatesSince,
};

use std::sync::Arc;

use async_trait::async_trait;
use std::pin::Pin;
use tokio_stream::Stream;

#[async_trait]
pub trait BlockServer {
    async fn query_block(
        &self,
        query_block: impl Into<QueryBlock> + Send + 'static,
    ) -> Result<Arc<Block>>;

    async fn query_blocks_since(
        &self,
        previous_block_hash: H256,
        depth: usize,
    ) -> Result<BlocksSince>;

    async fn subscribe_blocks(
        &self,
        confirmations: usize,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<BlockStreamItem>> + Send>>>;
}

#[async_trait]
pub trait StateServer {
    type InitialState: serde::Serialize;
    type State: serde::de::DeserializeOwned;

    async fn query_state(
        &self,
        initial_state: &Self::InitialState,
        query_block: impl Into<QueryBlock> + Send + 'static,
    ) -> Result<BlockState<Self::State>>;

    async fn query_states_since(
        &self,
        initial_state: &Self::InitialState,
        previous_block_hash: H256,
        depth: usize,
    ) -> Result<StatesSince<Self::State>>;

    async fn subscribe_states(
        &self,
        initial_state: &Self::InitialState,
        confirmations: usize,
    ) -> Result<
        Pin<
            Box<dyn Stream<Item = Result<StateStreamItem<Self::State>>> + Send>,
        >,
    >;
}
