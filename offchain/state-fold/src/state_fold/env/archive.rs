// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::state_fold::Foldable;

use super::train::Train;

use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;

pub(crate) struct Archive<F>
where
    F: Foldable,
{
    safety_margin: usize,
    trains: RwLock<HashMap<F::InitialState, Arc<Train<F>>>>,
}

impl<F> Archive<F>
where
    F: Foldable + 'static,
{
    pub fn new(safety_margin: usize) -> Self {
        Self {
            safety_margin,
            trains: RwLock::new(HashMap::new()),
        }
    }

    pub async fn get_train(
        &self,
        initial_state: &F::InitialState,
    ) -> Arc<Train<F>> {
        if let Some(train) = self.trains.read().await.get(initial_state) {
            return Arc::clone(train);
        }

        let train =
            Arc::new(Train::new(initial_state.clone(), self.safety_margin));

        self.trains
            .write()
            .await
            .insert(initial_state.clone(), Arc::clone(&train));

        train
    }
}
