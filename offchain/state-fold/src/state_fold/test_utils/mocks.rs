// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::state_fold::{
    FoldMiddleware, Foldable, StateFoldEnvironment, SyncMiddleware,
};

use state_fold_test::mock_middleware::MockError;
use state_fold_types::Block;

use async_trait::async_trait;
use ethers::providers::Middleware;
use std::sync::Arc;

#[derive(Clone, Debug)]
pub(crate) struct MockFold;

#[async_trait]
impl Foldable for MockFold {
    type InitialState = ();
    type Error = MockError;
    type UserData = ();

    async fn sync<M: Middleware>(
        _initial_state: &Self::InitialState,
        _block: &Block,
        _env: &StateFoldEnvironment<M, ()>,
        _access: Arc<SyncMiddleware<M>>,
    ) -> Result<Self, Self::Error> {
        unreachable!()
    }

    async fn fold<M: Middleware>(
        _previous_state: &Self,
        _block: &Block,
        _env: &StateFoldEnvironment<M, ()>,
        _access: Arc<FoldMiddleware<M>>,
    ) -> Result<Self, Self::Error> {
        unreachable!()
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) struct IncrementFold {
    pub(crate) low_hash: u64,
    pub(crate) n: u64,
    pub(crate) initial_state: u64,
}

#[async_trait]
impl Foldable for IncrementFold {
    type InitialState = u64;
    type Error = MockError;
    type UserData = ();

    async fn sync<M: Middleware>(
        initial_state: &Self::InitialState,
        block: &Block,
        _env: &StateFoldEnvironment<M, ()>,
        _access: Arc<SyncMiddleware<M>>,
    ) -> Result<Self, Self::Error> {
        Ok(Self {
            low_hash: block.hash.to_low_u64_be(),
            n: block.number.as_u64() + initial_state,
            initial_state: *initial_state,
        })
    }

    async fn fold<M: Middleware>(
        previous_state: &Self,
        block: &Block,
        _env: &StateFoldEnvironment<M, ()>,
        _access: Arc<FoldMiddleware<M>>,
    ) -> Result<Self, Self::Error> {
        assert_eq!(
            previous_state.n + 1,
            block.number.as_u64() + previous_state.initial_state
        );

        Ok(Self {
            low_hash: block.hash.to_low_u64_be(),
            n: previous_state.n + 1,
            initial_state: previous_state.initial_state,
        })
    }
}
