// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use ethers::core::types::{H256, U64};
use state_fold_types::Block;

use std::collections::HashMap;
use std::sync::Arc;

pub(crate) struct BlockTree {
    tree: HashMap<H256, Arc<Block>>,
    number_map: HashMap<U64, H256>,
    latest: Arc<Block>,
}

impl BlockTree {
    pub fn new(start_block: Arc<Block>) -> Self {
        Self {
            latest: Arc::clone(&start_block),
            number_map: HashMap::from([(start_block.number, start_block.hash)]),
            tree: HashMap::from([(start_block.hash, start_block)]),
        }
    }

    pub fn block_with_hash(&self, hash: &H256) -> Option<Arc<Block>> {
        self.tree.get(hash).cloned()
    }

    pub fn block_with_number(&self, number: &U64) -> Option<Arc<Block>> {
        let hash = self.number_map.get(number)?;
        self.block_with_hash(hash)
    }

    pub fn insert_block(&mut self, block: Arc<Block>) {
        self.number_map.insert(block.number, block.hash);
        self.tree.insert(block.hash, block);
    }

    pub fn latest_block(&self) -> Arc<Block> {
        Arc::clone(&self.latest)
    }

    pub fn update_latest_block(&mut self, block: Arc<Block>) {
        self.latest = Arc::clone(&block);
        self.insert_block(block);
    }
}
