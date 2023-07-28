// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::{FoldableError, UserData};

use state_fold::state_fold::{
    utils as fold_utils, FoldMiddleware, Foldable, StateFoldEnvironment,
    SyncMiddleware,
};
use state_fold_types::{
    ethers::{
        contract::LogMeta,
        prelude::EthEvent,
        providers::Middleware,
        types::{Address, TxHash},
    },
    Block,
};

use anyhow::{ensure, Context};
use async_trait::async_trait;
use im::{HashMap, Vector};
use serde::{Deserialize, Serialize};
use std::sync::{Arc, Mutex};

#[derive(Clone, Debug, Eq, Hash, PartialEq, Serialize, Deserialize)]
pub struct InputBoxInitialState {
    pub input_box_address: Arc<Address>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Input {
    pub sender: Arc<Address>,
    pub payload: Vec<u8>,
    pub block_added: Arc<Block>,
    pub dapp: Arc<Address>,
    pub tx_hash: Arc<TxHash>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct DAppInputBox {
    pub inputs: Vector<Arc<Input>>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct InputBox {
    pub input_box_address: Arc<Address>,
    pub dapp_input_boxes: Arc<HashMap<Arc<Address>, Arc<DAppInputBox>>>,
}

#[async_trait]
impl Foldable for InputBox {
    type InitialState = InputBoxInitialState;
    type Error = FoldableError;
    type UserData = Mutex<UserData>;

    async fn sync<M: Middleware + 'static>(
        initial_state: &Self::InitialState,
        _block: &Block,
        env: &StateFoldEnvironment<M, Self::UserData>,
        access: Arc<SyncMiddleware<M>>,
    ) -> Result<Self, Self::Error> {
        let input_box_address = Arc::clone(&initial_state.input_box_address);

        Ok(Self {
            dapp_input_boxes: updated_inputs(
                None,
                access,
                env,
                &input_box_address,
                None,
            )
            .await?,
            input_box_address,
        })
    }

    async fn fold<M: Middleware + 'static>(
        previous_state: &Self,
        block: &Block, // TODO: when new version of state-fold gets released, change this to Arc
        // and save on cloning.
        env: &StateFoldEnvironment<M, Self::UserData>,
        access: Arc<FoldMiddleware<M>>,
    ) -> Result<Self, Self::Error> {
        let input_box_address = Arc::clone(&previous_state.input_box_address);

        if !(fold_utils::contains_address(
            &block.logs_bloom,
            &input_box_address,
        ) && (fold_utils::contains_topic(
            &block.logs_bloom,
            &contracts::input_box::InputAddedFilter::signature(),
        ))) {
            return Ok(previous_state.clone());
        }

        Ok(Self {
            dapp_input_boxes: updated_inputs(
                Some(&previous_state.dapp_input_boxes),
                access,
                env,
                &input_box_address,
                None,
            )
            .await?,

            input_box_address,
        })
    }
}

async fn updated_inputs<M1: Middleware + 'static, M2: Middleware + 'static>(
    previous_input_boxes: Option<&HashMap<Arc<Address>, Arc<DAppInputBox>>>,
    provider: Arc<M1>,
    env: &StateFoldEnvironment<M2, <InputBox as Foldable>::UserData>,
    contract_address: &Address,
    block_opt: Option<Block>, // TODO: Option<Arc<Block>>,
) -> Result<Arc<HashMap<Arc<Address>, Arc<DAppInputBox>>>, FoldableError> {
    let mut input_boxes =
        previous_input_boxes.cloned().unwrap_or(HashMap::new());

    let new_inputs =
        fetch_all_new_inputs(provider, env, contract_address, block_opt)
            .await?;

    for input in new_inputs {
        let dapp = input.dapp.clone();
        let input = Arc::new(input);

        input_boxes
            .entry(dapp)
            .and_modify(|i| {
                let mut new_input_box = (**i).clone();
                new_input_box.inputs.push_back(input.clone());
                *i = Arc::new(new_input_box);
            })
            .or_insert_with(|| {
                Arc::new(DAppInputBox {
                    inputs: im::vector![input],
                })
            });
    }

    Ok(Arc::new(input_boxes))
}

async fn fetch_all_new_inputs<
    M1: Middleware + 'static,
    M2: Middleware + 'static,
>(
    provider: Arc<M1>,
    env: &StateFoldEnvironment<M2, <InputBox as Foldable>::UserData>,
    contract_address: &Address,
    block_opt: Option<Block>, // TODO: Option<Arc<Block>>,
) -> Result<Vec<Input>, FoldableError> {
    use contracts::input_box::*;
    let contract = InputBox::new(*contract_address, Arc::clone(&provider));

    // Retrieve `InputAdded` events
    let inputs_futures: Vec<_> = contract
        .input_added_filter()
        .query_with_meta()
        .await
        .context("Error querying for input added events")?
        .into_iter()
        .map(|(e, meta)| Input::build_input(env, e, meta, &block_opt))
        .collect();

    let inputs_results = futures::future::join_all(inputs_futures).await;

    let inputs = {
        let inputs: Result<Vec<Input>, _> =
            inputs_results.into_iter().collect();

        inputs?
    };

    Ok(inputs)
}

impl Input {
    async fn build_input<M: Middleware + 'static>(
        env: &StateFoldEnvironment<M, <InputBox as Foldable>::UserData>,
        event: contracts::input_box::InputAddedFilter,
        meta: LogMeta,
        block_opt: &Option<Block>, // TODO: &Option<Arc<Block>>
    ) -> Result<Self, FoldableError> {
        let block =
            match block_opt {
                Some(ref b) => Arc::new(b.clone()), // TODO: remove Arc::new

                None => env.block_with_hash(&meta.block_hash).await.context(
                    format!("Could not query block `{:?}`", meta.block_hash),
                )?,
            };

        meta_consistent_with_block(&meta, &block)?;

        let mut user_data = env
            .user_data()
            .lock()
            .expect("Mutex should never be poisoned");

        let sender = user_data.get(event.sender);
        let dapp = user_data.get(event.dapp);

        Ok(Self {
            sender,
            payload: event.input.to_vec(),
            dapp,
            block_added: block,
            tx_hash: Arc::new(meta.transaction_hash),
        })
    }
}

fn meta_consistent_with_block(
    meta: &LogMeta,
    block: &Block,
) -> Result<(), anyhow::Error> {
    ensure!(
        meta.block_hash == block.hash,
        "Sanity check failed: meta and block `block_hash` do not match"
    );

    ensure!(
        meta.block_number == block.number,
        "Sanity check failed: meta and block `block_number` do not match"
    );

    Ok(())
}
