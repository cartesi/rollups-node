// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::{error::*, BlockServer, StateServer};

use ethers::core::types::H256;

use state_fold_types::ethers;
use state_fold_types::{
    Block, BlockState, BlockStreamItem, BlocksSince, QueryBlock,
    StateStreamItem, StatesSince,
};

use grpc_interfaces::state_fold_server;
use state_fold_server::state_fold_client::StateFoldClient;
use state_fold_server::{
    InitialState, QueryBlockRequest, QueryBlocksSinceRequest,
    QueryStateRequest, QueryStatesSinceRequest, SubscribeNewBlocksRequest,
    SubscribeNewStatesRequest,
};
use tonic::{transport::Channel, Request};

use snafu::ResultExt;

use std::sync::Arc;

use async_trait::async_trait;
use std::pin::Pin;
use tokio_stream::{Stream, StreamExt};

pub struct GrpcStateFoldClient<I, S> {
    client: StateFoldClient<Channel>,
    __marker1: std::marker::PhantomData<I>,
    __marker2: std::marker::PhantomData<S>,
}

impl<I, S> GrpcStateFoldClient<I, S> {
    pub fn new_from_channel(channel: Channel) -> Self {
        let client = StateFoldClient::new(channel);

        Self {
            client,
            __marker1: std::marker::PhantomData,
            __marker2: std::marker::PhantomData,
        }
    }
}

#[async_trait]
impl<I, S> BlockServer for GrpcStateFoldClient<I, S>
where
    I: Send + Sync,
    S: Send + Sync,
{
    async fn query_block(
        &self,
        query_block: impl Into<QueryBlock> + Send + 'static,
    ) -> Result<Arc<Block>> {
        let mut client = self.client.clone();

        let query_block: QueryBlock = query_block.into();
        let request = Request::new(QueryBlockRequest {
            query_block: Some(query_block.into()),
        });

        let block = client
            .query_block(request)
            .await
            .context(TonicSnafu {
                context: "`get_block` request",
            })?
            .into_inner()
            .try_into()
            .context(MessageConversionSnafu {
                context: "`get_block`".to_owned(),
            })?;

        Ok(block)
    }

    async fn query_blocks_since(
        &self,
        previous_block_hash: H256,
        depth: usize,
    ) -> Result<BlocksSince> {
        let mut client = self.client.clone();

        let request = Request::new(QueryBlocksSinceRequest {
            previous_block: Some(previous_block_hash.into()),
            depth: depth as u64,
        });

        let diff = client
            .query_blocks_since(request)
            .await
            .context(TonicSnafu {
                context: "`get_block_diff` request",
            })?
            .into_inner()
            .try_into()
            .context(MessageConversionSnafu {
                context: "`get_block_diff`".to_owned(),
            })?;

        Ok(diff)
    }

    async fn subscribe_blocks(
        &self,
        confirmations: usize,
    ) -> Result<Pin<Box<dyn Stream<Item = Result<BlockStreamItem>> + Send>>>
    {
        let mut client = self.client.clone();

        let request = Request::new(SubscribeNewBlocksRequest {
            confirmations: confirmations as u64,
        });

        let stream = client
            .subscribe_new_blocks(request)
            .await
            .context(TonicSnafu {
                context: "`subscribe_blocks` request",
            })?
            .into_inner();

        let stream = stream.map(|b| -> Result<BlockStreamItem> {
            b.context(TonicSnafu {
                context: "`subscribe_blocks` stream item conversion",
            })?
            .try_into()
            .context(MessageConversionSnafu {
                context: "`subscribe_blocks` stream item conversion",
            })
        });

        Ok(Box::pin(stream))
    }
}

#[async_trait]
impl<I, S> StateServer for GrpcStateFoldClient<I, S>
where
    I: serde::Serialize + Send + Sync,
    S: serde::de::DeserializeOwned + Send + Sync,
{
    type InitialState = I;
    type State = S;

    async fn query_state(
        &self,
        initial_state: &Self::InitialState,
        query_block: impl Into<QueryBlock> + Send + 'static,
    ) -> Result<BlockState<Self::State>> {
        let mut client = self.client.clone();

        let initial_state_json = InitialState {
            json_data: serde_json::to_string(&initial_state)
                .context(SerializeSnafu)?,
        };

        let query_block: QueryBlock = query_block.into();

        let request = Request::new(QueryStateRequest {
            initial_state: Some(initial_state_json),
            query_block: Some(query_block.into()),
        });

        let state = client
            .query_state(request)
            .await
            .context(TonicSnafu {
                context: "`get_state` request",
            })?
            .into_inner()
            .try_into()
            .context(StateConversionSnafu {
                context: "`get_state`".to_owned(),
            })?;

        Ok(state)
    }

    async fn query_states_since(
        &self,
        initial_state: &Self::InitialState,
        previous_block_hash: H256,
        depth: usize,
    ) -> Result<StatesSince<Self::State>> {
        let mut client = self.client.clone();

        let initial_state_json = InitialState {
            json_data: serde_json::to_string(&initial_state)
                .context(SerializeSnafu)?,
        };

        let request = Request::new(QueryStatesSinceRequest {
            initial_state: Some(initial_state_json),
            previous_block: Some(previous_block_hash.into()),
            depth: depth as u64,
        });

        let diff = client
            .query_states_since(request)
            .await
            .context(TonicSnafu {
                context: "`get_state_diff` request",
            })?
            .into_inner()
            .try_into()
            .context(StateConversionSnafu {
                context: "`get_state_diff`".to_owned(),
            })?;

        Ok(diff)
    }

    async fn subscribe_states(
        &self,
        initial_state: &Self::InitialState,
        confirmations: usize,
    ) -> Result<
        Pin<
            Box<dyn Stream<Item = Result<StateStreamItem<Self::State>>> + Send>,
        >,
    > {
        let mut client = self.client.clone();

        let initial_state_json = InitialState {
            json_data: serde_json::to_string(&initial_state)
                .context(SerializeSnafu)?,
        };

        let request = Request::new(SubscribeNewStatesRequest {
            initial_state: Some(initial_state_json),
            confirmations: confirmations as u64,
        });

        let stream = client
            .subscribe_new_states(request)
            .await
            .context(TonicSnafu {
                context: "`subscribe_blocks` request",
            })?
            .into_inner();

        let stream = stream.map(|s| -> Result<StateStreamItem<Self::State>> {
            s.context(TonicSnafu {
                context: "`subscribe_blocks` stream item conversion",
            })?
            .try_into()
            .context(StateConversionSnafu {
                context: "`subscribe_blocks` stream item conversion",
            })
        });

        Ok(Box::pin(stream))
    }
}
