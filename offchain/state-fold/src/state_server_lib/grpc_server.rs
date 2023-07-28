// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use grpc_interfaces::state_fold_server;
use state_fold_server::state_fold_server::StateFold;
use state_fold_server::{
    query_block::Id,
    state_stream_response::Response as GrpcStateStreamResponse,
    states_since_response::Response as GrpcStatesSinceResponse,
    Block as GrpcBlock, BlockState as GrpcBlockState, BlockStreamResponse,
    BlocksSinceResponse, Hash as GrpcHash, InitialState as GrpcInitialState,
    QueryBlock, QueryBlockRequest, QueryBlocksSinceRequest, QueryStateRequest,
    QueryStatesSinceRequest, StateStreamResponse, States as GrpcStates,
    StatesSinceResponse, SubscribeNewBlocksRequest, SubscribeNewStatesRequest,
};

use tonic::{Request, Response, Status};

use ethers::providers::Middleware;
use ethers::types::H256;

use crate::block_history::{BlockArchive, BlockArchiveError, BlockSubscriber};
use crate::state_fold::{Foldable, StateFoldEnvironment};
use state_fold_types::{Block, BlockState, BlockStreamItem, BlocksSince};

use futures::future::try_join_all;
use futures::stream::StreamExt;
use serde;
use serde_json;
use std::pin::Pin;
use std::sync::Arc;
use tokio_stream::Stream;

pub struct StateServer<M: Middleware + 'static, UD, F: Foldable<UserData = UD>>
{
    pub block_subscriber: Arc<BlockSubscriber<M>>,
    pub env: Arc<StateFoldEnvironment<M, UD>>,
    __phantom: std::marker::PhantomData<F>,
}

impl<M: Middleware + 'static, UD, F: Foldable<UserData = UD>>
    StateServer<M, UD, F>
{
    pub fn new(
        block_subscriber: Arc<BlockSubscriber<M>>,
        env: Arc<StateFoldEnvironment<M, UD>>,
    ) -> Self {
        Self {
            block_subscriber,
            env,
            __phantom: std::marker::PhantomData,
        }
    }
}

#[tonic::async_trait]
impl<
        M: Middleware + 'static,
        UD: Send + Sync + 'static,
        F: Foldable<UserData = UD> + 'static,
    > StateFold for StateServer<M, UD, F>
where
    F::InitialState: serde::de::DeserializeOwned + 'static,
    F: serde::Serialize,
{
    type SubscribeNewBlocksStream =
        Pin<Box<dyn Stream<Item = Result<BlockStreamResponse, Status>> + Send>>;

    type SubscribeNewStatesStream =
        Pin<Box<dyn Stream<Item = Result<StateStreamResponse, Status>> + Send>>;

    #[tracing::instrument(skip_all)]
    async fn query_block(
        &self,
        request: Request<QueryBlockRequest>,
    ) -> Result<Response<GrpcBlock>, Status> {
        let query_block = request.into_inner().query_block;

        tracing::trace!("received `query_block` request `{:?}`", query_block);

        let block = get_block_from_archive(
            &self.block_subscriber.block_archive,
            query_block,
        )
        .await?;

        Ok(Response::new(block.into()))
    }

    #[tracing::instrument(skip_all)]
    async fn query_blocks_since(
        &self,
        request: Request<QueryBlocksSinceRequest>,
    ) -> Result<Response<BlocksSinceResponse>, Status> {
        let message = request.into_inner();
        let depth = message.depth as usize;
        let previous_block = message.previous_block;

        tracing::trace!(
            "received `query_blocks_since` request: depth = `{:?}`; previous_block = `{:?}`",
            depth,
            previous_block
        );

        let block = get_block_with_hash(
            previous_block,
            &self.block_subscriber.block_archive,
        )
        .await?;

        let diff = self
            .block_subscriber
            .block_archive
            .blocks_since(depth, block)
            .await
            .map_err(|e| match e {
                BlockArchiveError::BlockOutOfRange { .. } => {
                    Status::out_of_range(format!("{:?}", e))
                }
                e => Status::unavailable(format!("{:?}", e)),
            })?;

        Ok(Response::new(BlocksSinceResponse {
            response: Some(diff.into()),
        }))
    }

    #[tracing::instrument(skip_all)]
    async fn subscribe_new_blocks(
        &self,
        request: Request<SubscribeNewBlocksRequest>,
    ) -> Result<Response<Self::SubscribeNewBlocksStream>, Status> {
        let depth = request.into_inner().confirmations as usize;

        tracing::trace!(
            "received `subscribe_new_blocks` request: depth = `{:?}`",
            depth
        );

        let stream = self
            .block_subscriber
            .subscribe_new_blocks_at_depth(depth)
            .await
            .map_err(|e| Status::unavailable(format!("{:?}", e)))?;

        let stream = stream.map(|x| {
            x.map(|x| x.into())
                .map_err(|e| Status::unavailable(format!("{:?}", e)))
        });

        Ok(Response::new(Box::pin(stream)))
    }

    #[tracing::instrument(skip_all)]
    async fn query_state(
        &self,
        request: Request<QueryStateRequest>,
    ) -> Result<Response<GrpcBlockState>, Status> {
        let message = request.into_inner();
        let query_block = message.query_block;
        let initial_state = message.initial_state;

        tracing::trace!(
            "received `query_state` request: query_block = `{:?}`; initial_state = `{:?}`",
            query_block,
            initial_state
        );

        let initial_state: F::InitialState =
            convert_initial_state::<F>(initial_state)?;

        let block = get_block_from_archive(
            &self.block_subscriber.block_archive,
            query_block,
        )
        .await?;

        let state = F::get_state_for_block(&initial_state, block, &self.env)
            .await
            .map_err(|e| Status::unavailable(format!("{:?}", e)))?;

        let serialized_state = state
            .try_into()
            .map_err(|e| Status::internal(format!("{:?}", e)))?;

        Ok(Response::new(serialized_state))
    }

    #[tracing::instrument(skip_all)]
    async fn query_states_since(
        &self,
        request: Request<QueryStatesSinceRequest>,
    ) -> Result<Response<StatesSinceResponse>, Status> {
        let message = request.into_inner();
        let depth = message.depth as usize;
        let initial_state = message.initial_state;

        tracing::trace!(
            "received `query_states_since` request: depth = `{:?}`; initial_state = `{:?}`",
            depth,
            initial_state
        );

        let initial_state: F::InitialState =
            convert_initial_state::<F>(initial_state)?;

        let block = get_block_with_hash(
            message.previous_block,
            &self.block_subscriber.block_archive,
        )
        .await?;

        let diff = self
            .block_subscriber
            .block_archive
            .blocks_since(depth, block)
            .await
            .map_err(|e| match e {
                BlockArchiveError::BlockOutOfRange { .. } => {
                    Status::out_of_range(format!("{:?}", e))
                }
                e => Status::unavailable(format!("{:?}", e)),
            })?;

        let state_diff = match diff {
            BlocksSince::Normal(bs) => GrpcStatesSinceResponse::NewStates(
                map_blocks_into_grpc_states::<_, _, F>(
                    bs,
                    &initial_state,
                    &self.env,
                )
                .await?,
            ),

            BlocksSince::Reorg(bs) => {
                GrpcStatesSinceResponse::ReorganizedStates(
                    map_blocks_into_grpc_states::<_, _, F>(
                        bs,
                        &initial_state,
                        &self.env,
                    )
                    .await?,
                )
            }
        };

        Ok(Response::new(StatesSinceResponse {
            response: Some(state_diff),
        }))
    }

    #[tracing::instrument(skip_all)]
    async fn subscribe_new_states(
        &self,
        request: Request<SubscribeNewStatesRequest>,
    ) -> Result<Response<Self::SubscribeNewStatesStream>, Status> {
        let message = request.into_inner();
        let depth = message.confirmations as usize;
        let initial_state = message.initial_state;

        tracing::trace!(
            "received `subscribe_new_states` request: depth = `{:?}`; initial_state = `{:?}`",
            depth,
            initial_state
        );

        let initial_state: Arc<F::InitialState> =
            Arc::new(convert_initial_state::<F>(initial_state)?);
        let env = Arc::clone(&self.env);

        let stream = self
            .block_subscriber
            .subscribe_new_blocks_at_depth(depth)
            .await
            .map_err(|e| Status::unavailable(format!("{:?}", e)))?;

        let stream = stream.then(move |item| {
            let initial_state = Arc::clone(&initial_state);
            let env = Arc::clone(&env);

            async move {
                match item {
                    Ok(BlockStreamItem::NewBlock(block)) => {
                        let block_state = get_foldable_state::<_, _, F>(
                            &initial_state,
                            block,
                            &env,
                        )
                        .await?;

                        let response = Some(GrpcStateStreamResponse::NewState(
                            block_state.try_into().map_err(|e| {
                                Status::internal(format!("{:?}", e))
                            })?,
                        ));

                        Ok(StateStreamResponse { response })
                    }

                    Ok(BlockStreamItem::Reorg(blocks)) => {
                        let block_states =
                            map_blocks_into_grpc_states::<_, _, F>(
                                blocks,
                                &initial_state,
                                &env,
                            )
                            .await?;

                        let response =
                            Some(GrpcStateStreamResponse::ReorganizedStates(
                                block_states.try_into().map_err(|e| {
                                    Status::internal(format!("{:?}", e))
                                })?,
                            ));

                        Ok(StateStreamResponse { response })
                    }

                    Err(e) => Err(Status::unavailable(format!("{:?}", e))),
                }
            }
        });

        Ok(Response::new(Box::pin(stream)))
    }
}

async fn get_block_from_archive<M: Middleware + 'static>(
    archive: &BlockArchive<M>,
    query_block: Option<QueryBlock>,
) -> Result<Arc<Block>, Status> {
    Ok(match query_block {
        Some(QueryBlock {
            id: Some(Id::Depth(d)),
        }) => archive
            .block_at_depth(d as usize)
            .await
            .map_err(|e| Status::unavailable(format!("{:?}", e)))?,

        Some(QueryBlock {
            id: Some(Id::BlockHash(h)),
        }) => archive
            .block_with_hash(
                &h.try_into()
                    .map_err(|e| Status::invalid_argument(format!("{}", e)))?,
            )
            .await
            .map_err(|e| Status::unavailable(format!("{}", e)))?,

        Some(QueryBlock {
            id: Some(Id::BlockNumber(n)),
        }) => archive
            .block_with_number(n.into())
            .await
            .map_err(|e| Status::unavailable(format!("{}", e)))?,

        Some(QueryBlock { id: None }) | None => archive.latest_block().await,
    })
}

async fn get_block_with_hash<M: Middleware + 'static>(
    hash: Option<GrpcHash>,
    archive: &BlockArchive<M>,
) -> Result<Arc<Block>, Status> {
    let hash = convert_hash(hash)?;
    archive
        .block_with_hash(&hash)
        .await
        .map_err(|e| Status::unavailable(format!("{:?}", e)))
}

fn convert_hash(hash: Option<GrpcHash>) -> Result<H256, Status> {
    hash.ok_or(Status::invalid_argument("Previous block hash is nil"))?
        .try_into()
        .map_err(|e| Status::invalid_argument(format!("{:?}", e)))
}

fn convert_initial_state<F: Foldable>(
    is: Option<GrpcInitialState>,
) -> Result<F::InitialState, Status>
where
    F::InitialState: serde::de::DeserializeOwned,
{
    let initial_state_json = is
        .ok_or(Status::invalid_argument("Initial state is nil"))?
        .json_data;

    serde_json::from_str(&initial_state_json)
        .map_err(|e| Status::invalid_argument(format!("{:?}", e)))
}

async fn map_blocks_into_grpc_states<
    M: Middleware + 'static,
    UD,
    F: Foldable<UserData = UD> + serde::Serialize + 'static,
>(
    blocks: Vec<Arc<Block>>,
    initial_state: &F::InitialState,
    env: &StateFoldEnvironment<M, UD>,
) -> Result<GrpcStates, Status> {
    let futs_arr: Vec<_> = blocks
        .into_iter()
        .map(|block| get_foldable_state(initial_state, block, env))
        .collect();

    let arr: Vec<BlockState<F>> = try_join_all(futs_arr).await?;

    arr.try_into()
        .map_err(|e| Status::internal(format!("{:?}", e)))
}

async fn get_foldable_state<
    M: Middleware + 'static,
    UD,
    F: Foldable<UserData = UD> + 'static,
>(
    initial_state: &F::InitialState,
    block: Arc<Block>,
    env: &StateFoldEnvironment<M, UD>,
) -> Result<BlockState<F>, Status> {
    F::get_state_for_block(initial_state, block, env)
        .await
        .map_err(|e| Status::unavailable(format!("{:?}", e)))
}
