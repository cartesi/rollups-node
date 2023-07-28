// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use ethereum_types::{Bloom, H256, U256, U64};
use serde::{Deserialize, Serialize};
use snafu::Snafu;
use std::sync::Arc;

#[cfg(feature = "ethers")]
pub mod contract;

pub use ethabi;
pub use ethabi::ethereum_types;

#[cfg(feature = "ethers")]
pub use ethers;

pub mod config_utils;

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Block {
    pub hash: H256,
    pub number: U64,
    pub parent_hash: H256,
    pub timestamp: U256,
    pub logs_bloom: Bloom,
}

impl PartialEq for Block {
    fn eq(&self, other: &Self) -> bool {
        self.hash.eq(&other.hash)
    }
}

impl Eq for Block {}

impl std::hash::Hash for Block {
    fn hash<H: std::hash::Hasher>(&self, state: &mut H) {
        self.hash.hash(state)
    }
}

#[derive(Debug)]
pub struct BlockState<State> {
    pub block: Arc<Block>,
    pub state: Arc<State>,
}

impl<State> Clone for BlockState<State> {
    fn clone(&self) -> Self {
        Self {
            block: Arc::clone(&self.block),
            state: Arc::clone(&self.state),
        }
    }
}

#[derive(Clone, Debug)]
pub enum BlocksSince {
    Normal(Vec<Arc<Block>>),
    Reorg(Vec<Arc<Block>>),
}

#[derive(Clone, Debug)]
pub enum BlockStreamItem {
    NewBlock(Arc<Block>),
    Reorg(Vec<Arc<Block>>),
}

#[derive(Clone, Debug)]
pub enum StatesSince<T> {
    Normal(Vec<BlockState<T>>),
    Reorg(Vec<BlockState<T>>),
}

#[derive(Clone, Debug)]
pub enum StateStreamItem<T> {
    NewState(BlockState<T>),
    Reorg(Vec<BlockState<T>>),
}

#[derive(Clone, Debug)]
pub enum QueryBlock {
    Latest,
    BlockHash(H256),
    BlockNumber(U64),
    BlockDepth(usize),
    Block(Arc<Block>),
}

/// Error that might occur when trying to convert [`ethers::Block`] into
/// [`Block`].
///
/// [`Block`]: self::Block
/// [`ethers::Block`]: self::ethers::types::Block
#[derive(Snafu, Clone, Debug)]
pub enum BlockError {
    #[snafu(display("Block has no hash"))]
    MissingHash,
    #[snafu(display("Block has no number"))]
    MissingNumber,
    #[snafu(display("Block has no logs bloom"))]
    MissingLogsBloom,
}

#[cfg(feature = "ethers")]
impl<T> std::convert::TryFrom<self::ethers::types::Block<T>> for Block {
    type Error = BlockError;

    fn try_from(b: self::ethers::types::Block<T>) -> Result<Self, Self::Error> {
        Ok(Self {
            hash: H256::from(b.hash.ok_or(BlockError::MissingHash)?.0),
            number: U64(b.number.ok_or(BlockError::MissingNumber)?.0),
            parent_hash: H256::from(b.parent_hash.0),
            timestamp: U256(b.timestamp.0),
            logs_bloom: Bloom::from(
                b.logs_bloom.ok_or(BlockError::MissingLogsBloom)?.0,
            ),
        })
    }
}

impl From<H256> for QueryBlock {
    fn from(h: H256) -> Self {
        QueryBlock::BlockHash(h)
    }
}

impl From<&H256> for QueryBlock {
    fn from(h: &H256) -> Self {
        QueryBlock::BlockHash(*h)
    }
}

impl From<U64> for QueryBlock {
    fn from(n: U64) -> Self {
        QueryBlock::BlockNumber(n)
    }
}

impl From<&U64> for QueryBlock {
    fn from(n: &U64) -> Self {
        QueryBlock::BlockNumber(*n)
    }
}

impl From<Block> for QueryBlock {
    fn from(b: Block) -> Self {
        QueryBlock::Block(Arc::new(b))
    }
}

impl From<Arc<Block>> for QueryBlock {
    fn from(b: Arc<Block>) -> Self {
        QueryBlock::Block(b)
    }
}

impl From<&Block> for QueryBlock {
    fn from(b: &Block) -> Self {
        QueryBlock::from(b.clone())
    }
}
