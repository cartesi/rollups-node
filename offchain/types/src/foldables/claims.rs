// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::{FoldableError, UserData};

use state_fold::state_fold::{
    utils as fold_utils, FoldMiddleware, Foldable, StateFoldEnvironment,
    SyncMiddleware,
};
use state_fold_types::{
    ethers::{
        prelude::EthEvent,
        providers::Middleware,
        types::{Address, H256, U256},
    },
    Block,
};

use anyhow::Context;
use async_trait::async_trait;
use im::{HashMap, Vector};
use serde::{Deserialize, Serialize};
use std::sync::{Arc, Mutex};

#[derive(Clone, Debug, Eq, Hash, PartialEq, Serialize, Deserialize)]
pub struct HistoryInitialState {
    pub history_address: Arc<Address>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Claim {
    pub epoch_hash: H256,

    // Both "closed/inclusive"
    pub start_input_index: usize,
    pub end_input_index: usize,

    pub claim_timestamp: u64,
}

impl From<(contracts::history::Claim, U256)> for Claim {
    fn from(x: (contracts::history::Claim, U256)) -> Self {
        let c = x.0;
        let t = x.1;
        Self {
            epoch_hash: c.epoch_hash.into(),
            start_input_index: c.first_index as usize,
            end_input_index: c.last_index as usize,
            claim_timestamp: t.as_u64(),
        }
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct DAppClaims {
    pub claims: Vector<Arc<Claim>>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct History {
    pub history_address: Arc<Address>,
    pub dapp_claims: Arc<HashMap<Arc<Address>, Arc<DAppClaims>>>,
}

#[async_trait]
impl Foldable for History {
    type InitialState = HistoryInitialState;
    type Error = FoldableError;
    type UserData = Mutex<UserData>;

    async fn sync<M: Middleware + 'static>(
        initial_state: &Self::InitialState,
        _block: &Block,
        env: &StateFoldEnvironment<M, Self::UserData>,
        access: Arc<SyncMiddleware<M>>,
    ) -> Result<Self, Self::Error> {
        let history_address = Arc::clone(&initial_state.history_address);

        let dapp_claims =
            fetch_history(access, env, &history_address, &HashMap::new())
                .await?;

        Ok(Self {
            history_address,
            dapp_claims,
        })
    }

    async fn fold<M: Middleware + 'static>(
        previous_state: &Self,
        block: &Block, // TODO: when new version of state-fold gets released, change this to Arc
        // and save on cloning.
        env: &StateFoldEnvironment<M, Self::UserData>,
        access: Arc<FoldMiddleware<M>>,
    ) -> Result<Self, Self::Error> {
        let history_address = Arc::clone(&previous_state.history_address);

        if !(fold_utils::contains_address(&block.logs_bloom, &history_address)
            && (fold_utils::contains_topic(
                &block.logs_bloom,
                &contracts::history::NewClaimToHistoryFilter::signature(),
            )))
        {
            return Ok(previous_state.clone());
        }

        let new_dapp_claims = fetch_history(
            access,
            env,
            &history_address,
            &previous_state.dapp_claims,
        )
        .await?;

        Ok(Self {
            history_address,
            dapp_claims: new_dapp_claims,
        })
    }
}

async fn fetch_history<M1: Middleware + 'static, M2: Middleware + 'static>(
    provider: Arc<M1>,
    env: &StateFoldEnvironment<M2, <History as Foldable>::UserData>,
    contract_address: &Address,
    previous_dapp_claims: &HashMap<Arc<Address>, Arc<DAppClaims>>,
) -> Result<Arc<HashMap<Arc<Address>, Arc<DAppClaims>>>, FoldableError> {
    use contracts::history::*;
    let contract = History::new(*contract_address, Arc::clone(&provider));

    let mut dapp_claims = previous_dapp_claims.clone();

    // Retrieve `NewClaim` events
    let claims = contract
        .new_claim_to_history_filter()
        .query_with_meta()
        .await
        .context("Error querying for new claim events")?;

    for (claim, meta) in claims {
        let timestamp = env
            .block_with_hash(&meta.block_hash)
            .await
            .context("Error querying for block")?
            .timestamp;

        let new_claim: Arc<crate::foldables::claims::Claim> =
            Arc::new((claim.claim, timestamp).into());
        let dapp_address = Arc::new(claim.dapp);

        dapp_claims
            .entry(dapp_address)
            .and_modify(|h| {
                let mut new_history = (**h).clone();
                new_history.claims.push_back(new_claim.clone());
                *h = Arc::new(new_history);
            })
            .or_insert_with(|| {
                Arc::new(DAppClaims {
                    claims: im::vector![new_claim],
                })
            });
    }

    Ok(Arc::new(dapp_claims))
}
