use crate::state_fold_server::{
    block_stream_response::Response as GrpcBlockStreamResponse,
    blocks_since_response::Response as GrpcBlocksSince, query_block::Id,
    state_stream_response::Response as GrpcStateStreamResponse,
    states_since_response::Response as GrpcStatesSince, Block as GrpcBlock,
    BlockState as GrpcBlockState, BlockStreamResponse, Blocks as GrpcBlocks,
    BlocksSinceResponse, Bloom as GrpcBloom, Hash,
    QueryBlock as GrpcQueryBlock, State as GrpcState, StateStreamResponse,
    States as GrpcStates, StatesSinceResponse,
};

use state_fold_types::{
    ethereum_types::{Bloom, H256},
    Block,
};

use state_fold_types::{
    BlockState, BlockStreamItem, BlocksSince, QueryBlock, StateStreamItem,
    StatesSince,
};

use serde::{de::DeserializeOwned, Serialize};
use serde_json;

use std::convert::{TryFrom, TryInto};
use std::sync::Arc;

use snafu::{ResultExt, Snafu};

#[derive(Debug, Snafu)]
#[snafu(display("message `{}` field `{}` is nil", message, field))]
pub struct MessageNilError {
    message: String,
    field: String,
}

#[derive(Debug, Snafu)]
#[snafu(display(
    "message `{}` field `{}` malformed: {}",
    message,
    field,
    reason
))]
pub struct MessageMalformedError {
    message: String,
    field: String,
    reason: String,
}

#[derive(Debug, Snafu)]
pub enum MessageConversionError {
    #[snafu(display("NilError error: {}", source,))]
    NilError { source: MessageNilError },

    #[snafu(display("MalformedError error: {}", source,))]
    MalformedError { source: MessageMalformedError },
}

#[derive(Debug, Snafu)]
pub enum StateConversionError {
    #[snafu(display("NilError error: {}", source,))]
    MessageError { source: MessageConversionError },

    #[snafu(display("Deserialize error: {}", source))]
    DeserializeError { source: serde_json::Error },
}

impl<T: Serialize> TryFrom<BlockState<T>> for GrpcBlockState {
    type Error = serde_json::Error;

    fn try_from(bs: BlockState<T>) -> Result<Self, Self::Error> {
        Ok(Self {
            block: Some(bs.block.into()),
            state: Some(GrpcState {
                json_data: serde_json::to_string(bs.state.as_ref())?,
            }),
        })
    }
}

impl<T: DeserializeOwned> TryFrom<GrpcBlockState> for BlockState<T> {
    type Error = StateConversionError;

    fn try_from(sr: GrpcBlockState) -> Result<Self, Self::Error> {
        let block = sr
            .block
            .ok_or(MessageNilError {
                message: "GrpcBlockState".to_owned(),
                field: "block".to_owned(),
            })
            .context(NilSnafu)
            .context(MessageSnafu)?
            .try_into()
            .context(MessageSnafu)?;

        let state = serde_json::from_str(
            &sr.state
                .ok_or(MessageNilError {
                    message: "GrpcBlockState".to_owned(),
                    field: "state".to_owned(),
                })
                .context(NilSnafu)
                .context(MessageSnafu)?
                .json_data,
        )
        .context(DeserializeSnafu)?;

        Ok(Self {
            block,
            state: Arc::new(state),
        })
    }
}

impl<T: DeserializeOwned> TryFrom<StatesSinceResponse> for StatesSince<T> {
    type Error = StateConversionError;

    fn try_from(sd: StatesSinceResponse) -> Result<Self, Self::Error> {
        sd.response
            .ok_or(MessageNilError {
                message: "StatesSinceResponse".to_owned(),
                field: "response".to_owned(),
            })
            .context(NilSnafu)
            .context(MessageSnafu)?
            .try_into()
    }
}

impl<T: DeserializeOwned> TryFrom<GrpcStatesSince> for StatesSince<T> {
    type Error = StateConversionError;

    fn try_from(sd: GrpcStatesSince) -> Result<Self, Self::Error> {
        Ok(match sd {
            GrpcStatesSince::NewStates(ss) => {
                StatesSince::Normal(ss.try_into()?)
            }
            GrpcStatesSince::ReorganizedStates(ss) => {
                StatesSince::Reorg(ss.try_into()?)
            }
        })
    }
}

impl<T: DeserializeOwned> TryFrom<StateStreamResponse> for StateStreamItem<T> {
    type Error = StateConversionError;

    fn try_from(b: StateStreamResponse) -> Result<Self, Self::Error> {
        b.response
            .ok_or(MessageNilError {
                message: "StateStreamResponse".to_owned(),
                field: "response".to_owned(),
            })
            .context(NilSnafu)
            .context(MessageSnafu)?
            .try_into()
    }
}

impl<T: Serialize> TryFrom<StateStreamItem<T>> for StateStreamResponse {
    type Error = serde_json::Error;

    fn try_from(i: StateStreamItem<T>) -> Result<Self, Self::Error> {
        Ok(Self {
            response: Some(i.try_into()?),
        })
    }
}

impl<T: Serialize> TryFrom<StateStreamItem<T>> for GrpcStateStreamResponse {
    type Error = serde_json::Error;

    fn try_from(i: StateStreamItem<T>) -> Result<Self, Self::Error> {
        Ok(match i {
            StateStreamItem::NewState(s) => {
                GrpcStateStreamResponse::NewState(s.try_into()?)
            }
            StateStreamItem::Reorg(ss) => {
                GrpcStateStreamResponse::ReorganizedStates(ss.try_into()?)
            }
        })
    }
}
impl<T: DeserializeOwned> TryFrom<GrpcStateStreamResponse>
    for StateStreamItem<T>
{
    type Error = StateConversionError;

    fn try_from(ssr: GrpcStateStreamResponse) -> Result<Self, Self::Error> {
        Ok(match ssr {
            GrpcStateStreamResponse::NewState(s) => {
                StateStreamItem::NewState(s.try_into()?)
            }
            GrpcStateStreamResponse::ReorganizedStates(ss) => {
                StateStreamItem::Reorg(ss.try_into()?)
            }
        })
    }
}

impl From<BlocksSince> for BlocksSinceResponse {
    fn from(d: BlocksSince) -> Self {
        Self {
            response: Some(d.into()),
        }
    }
}

impl TryFrom<BlocksSinceResponse> for BlocksSince {
    type Error = MessageConversionError;

    fn try_from(d: BlocksSinceResponse) -> Result<Self, Self::Error> {
        d.response
            .ok_or(MessageNilError {
                message: "BlocksSinceResponse".to_owned(),
                field: "response".to_owned(),
            })
            .context(NilSnafu)?
            .try_into()
    }
}

impl From<BlocksSince> for GrpcBlocksSince {
    fn from(d: BlocksSince) -> Self {
        match d {
            BlocksSince::Normal(bs) => GrpcBlocksSince::NewBlocks(bs.into()),
            BlocksSince::Reorg(bs) => {
                GrpcBlocksSince::ReorganizedBlocks(bs.into())
            }
        }
    }
}

impl TryFrom<GrpcBlocksSince> for BlocksSince {
    type Error = MessageConversionError;

    fn try_from(d: GrpcBlocksSince) -> Result<Self, Self::Error> {
        Ok(match d {
            GrpcBlocksSince::NewBlocks(bs) => {
                BlocksSince::Normal(bs.try_into()?)
            }

            GrpcBlocksSince::ReorganizedBlocks(bs) => {
                BlocksSince::Reorg(bs.try_into()?)
            }
        })
    }
}

impl TryFrom<BlockStreamResponse> for BlockStreamItem {
    type Error = MessageConversionError;

    fn try_from(b: BlockStreamResponse) -> Result<Self, Self::Error> {
        b.response
            .ok_or(MessageNilError {
                message: "BlockStreamResponse".to_owned(),
                field: "response".to_owned(),
            })
            .context(NilSnafu)?
            .try_into()
    }
}

impl From<BlockStreamItem> for BlockStreamResponse {
    fn from(i: BlockStreamItem) -> Self {
        Self {
            response: Some(i.into()),
        }
    }
}

impl From<BlockStreamItem> for GrpcBlockStreamResponse {
    fn from(i: BlockStreamItem) -> Self {
        match i {
            BlockStreamItem::NewBlock(b) => {
                GrpcBlockStreamResponse::NewBlock(b.into())
            }
            BlockStreamItem::Reorg(bs) => {
                GrpcBlockStreamResponse::ReorganizedBlocks(bs.into())
            }
        }
    }
}

impl TryFrom<GrpcBlockStreamResponse> for BlockStreamItem {
    type Error = MessageConversionError;

    fn try_from(s: GrpcBlockStreamResponse) -> Result<Self, Self::Error> {
        Ok(match s {
            GrpcBlockStreamResponse::NewBlock(b) => {
                BlockStreamItem::NewBlock(b.try_into()?)
            }

            GrpcBlockStreamResponse::ReorganizedBlocks(bs) => {
                BlockStreamItem::Reorg(bs.try_into()?)
            }
        })
    }
}

impl<T: Serialize> TryFrom<Vec<BlockState<T>>> for GrpcStates {
    type Error = serde_json::Error;

    fn try_from(bs: Vec<BlockState<T>>) -> Result<Self, Self::Error> {
        let states: Result<_, _> =
            bs.into_iter().map(|x| x.try_into()).collect();
        let states = states?;
        Ok(GrpcStates { states })
    }
}

impl<T: DeserializeOwned> TryFrom<GrpcStates> for Vec<BlockState<T>> {
    type Error = StateConversionError;

    fn try_from(bs: GrpcStates) -> Result<Self, Self::Error> {
        let blocks_result: Vec<Result<BlockState<T>, _>> =
            bs.states.into_iter().map(|x| x.try_into()).collect();

        blocks_result.into_iter().collect()
    }
}

impl From<Vec<Arc<Block>>> for GrpcBlocks {
    fn from(bs: Vec<Arc<Block>>) -> Self {
        let blocks: Vec<GrpcBlock> = bs.into_iter().map(|x| x.into()).collect();
        GrpcBlocks { blocks }
    }
}

impl TryFrom<GrpcBlocks> for Vec<Arc<Block>> {
    type Error = MessageConversionError;

    fn try_from(bs: GrpcBlocks) -> Result<Self, Self::Error> {
        let blocks_result: Vec<Result<Arc<Block>, _>> =
            bs.blocks.into_iter().map(|x| x.try_into()).collect();

        blocks_result.into_iter().collect()
    }
}

impl TryFrom<Id> for QueryBlock {
    type Error = MessageConversionError;

    fn try_from(i: Id) -> Result<Self, Self::Error> {
        Ok(match i {
            Id::Depth(d) => QueryBlock::BlockDepth(d as usize),

            Id::BlockHash(h) => {
                QueryBlock::BlockHash(h.try_into().context(MalformedSnafu)?)
            }

            Id::BlockNumber(n) => QueryBlock::BlockNumber(n.into()),
        })
    }
}

impl From<QueryBlock> for GrpcQueryBlock {
    fn from(b: QueryBlock) -> Self {
        let id = match b {
            QueryBlock::BlockDepth(d) => Some(Id::Depth(d as u64)),

            QueryBlock::BlockHash(h) => Some(Id::BlockHash(h.into())),

            QueryBlock::BlockNumber(n) => Some(Id::BlockNumber(n.as_u64())),

            QueryBlock::Block(b) => Some(Id::BlockHash(b.hash.into())),

            QueryBlock::Latest => None,
        };

        Self { id }
    }
}

impl TryFrom<GrpcQueryBlock> for QueryBlock {
    type Error = MessageConversionError;

    fn try_from(b: GrpcQueryBlock) -> Result<Self, Self::Error> {
        Ok(match b.id {
            Some(i) => i.try_into()?,
            None => QueryBlock::Latest,
        })
    }
}

impl TryFrom<GrpcBlock> for Arc<Block> {
    type Error = MessageConversionError;

    fn try_from(b: GrpcBlock) -> Result<Self, Self::Error> {
        let ret = Block {
            hash: b
                .hash
                .ok_or(MessageNilError {
                    message: "Block".to_owned(),
                    field: "hash".to_owned(),
                })
                .context(NilSnafu)?
                .try_into()
                .context(MalformedSnafu)?,

            number: b.number.into(),

            parent_hash: b
                .parent_hash
                .ok_or(MessageNilError {
                    message: "Block".to_owned(),
                    field: "parent_hash".to_owned(),
                })
                .context(NilSnafu)?
                .try_into()
                .context(MalformedSnafu)?,

            timestamp: b.timestamp.into(),

            logs_bloom: b
                .logs_bloom
                .ok_or(MessageNilError {
                    message: "Block".to_owned(),
                    field: "logs_bloom".to_owned(),
                })
                .context(NilSnafu)?
                .try_into()
                .context(MalformedSnafu)?,
        };

        Ok(Arc::new(ret))
    }
}

impl From<Arc<Block>> for GrpcBlock {
    fn from(b: Arc<Block>) -> Self {
        Self {
            hash: Some(b.hash.into()),
            number: b.number.as_u64(),
            parent_hash: Some(b.parent_hash.into()),
            timestamp: b.timestamp.as_u64(),
            logs_bloom: Some(b.logs_bloom.into()),
        }
    }
}

impl TryFrom<Hash> for H256 {
    type Error = MessageMalformedError;

    fn try_from(h: Hash) -> Result<Self, Self::Error> {
        let data = h.data;
        let len = data.len();

        if len != 32 {
            return Err(MessageMalformedError {
                message: "Block".to_owned(),
                field: "data".to_owned(),
                reason: format!("length of `{}` is different than 32", len),
            });
        }

        Ok(H256::from_slice(&data))
    }
}

impl From<H256> for Hash {
    fn from(h: H256) -> Self {
        Self {
            data: h.to_fixed_bytes().into(),
        }
    }
}

impl TryFrom<GrpcBloom> for Bloom {
    type Error = MessageMalformedError;

    fn try_from(b: GrpcBloom) -> Result<Self, Self::Error> {
        let data = b.data;
        let len = data.len();

        if len != 256 {
            return Err(MessageMalformedError {
                message: "Bloom".to_owned(),
                field: "data".to_owned(),
                reason: format!("length of `{}` is different than 256", len),
            });
        }

        Ok(Bloom::from_slice(&data))
    }
}

impl From<Bloom> for GrpcBloom {
    fn from(b: Bloom) -> Self {
        Self {
            data: b.to_fixed_bytes().into(),
        }
    }
}
