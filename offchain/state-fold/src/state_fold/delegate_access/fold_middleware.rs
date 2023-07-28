// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::error::*;

use ethers::core::types::{
    transaction::eip2718::TypedTransaction, BlockId, Bytes, Filter, Log, H256,
};
use ethers::providers::{FromErr, Middleware};

use async_trait::async_trait;
use std::sync::Arc;

#[derive(Debug)]
pub struct FoldMiddleware<M> {
    inner: Arc<M>,
    block_hash: H256,
}

impl<M> FoldMiddleware<M>
where
    M: Middleware,
{
    pub fn new(inner: Arc<M>, block_hash: H256) -> Self {
        Self { inner, block_hash }
    }
}

#[async_trait]
impl<M> Middleware for FoldMiddleware<M>
where
    M: Middleware + 'static,
{
    type Error = AccessError<M>;
    type Provider = M::Provider;
    type Inner = M;

    fn inner(&self) -> &M {
        Arc::as_ref(&self.inner)
    }

    async fn call(
        &self,
        tx: &TypedTransaction,
        block: Option<BlockId>,
    ) -> std::result::Result<Bytes, Self::Error> {
        // If user provides a block, we use it. Otherwise, we use the default
        // block given during instantiation.
        let block = block.or_else(|| Some(self.block_hash.into()));
        self.inner().call(tx, block).await.map_err(FromErr::from)
    }

    async fn get_logs(
        &self,
        filter: &Filter,
    ) -> std::result::Result<Vec<Log>, Self::Error> {
        // Unlike call, we always override user provided range. This is a
        // limitation of ethers, because the type that holds the range is
        // private.
        let filter = filter.clone().at_block_hash(self.block_hash);
        let mut logs = self
            .inner()
            .get_logs(&filter)
            .await
            .map_err(FromErr::from)?;

        super::utils::sort_logs(&mut logs)?;
        Ok(logs)
    }
}

#[cfg(test)]
pub mod tests {
    use crate::state_fold::{utils, StateFoldEnvironment};

    use ethers::providers::Middleware;
    use ethers::types::{Address, U256};
    use state_fold_types::Block;

    use state_fold_test::simple_storage::SimpleStorage;

    pub async fn fold_query_test<M: Middleware + 'static>(
        account: Address,
        deployed_address: Address,
        env: &StateFoldEnvironment<M, ()>,
        blocks: (&Block, &Block, &Block, &Block),
    ) {
        // Test at block_hash0
        {
            let m = env.fold_access(blocks.0);
            let simple_storage = SimpleStorage::new(deployed_address, m);

            let value = simple_storage.get_value().call().await.unwrap();
            assert_eq!(value, "initial value");

            let event =
                simple_storage.value_changed_filter().query().await.unwrap();
            assert_eq!(event.len(), 1);
            assert_eq!(event[0].old_author, Address::zero());
            assert_eq!(event[0].author, account);
            assert_eq!(event[0].n, 0.into());
            assert_eq!(event[0].old_value, "");
            assert_eq!(event[0].new_value, "initial value");

            let bloom = blocks.0.logs_bloom;
            assert!(utils::contains_address(&bloom, &deployed_address));
            assert!(utils::contains_topic(&bloom, &account));
            assert!(utils::contains_topic(&bloom, &Address::zero()));
            assert!(utils::contains_topic(&bloom, &U256::from(0)));
        }

        // Test at block_hash1
        {
            let m = env.fold_access(blocks.1);
            let simple_storage = SimpleStorage::new(deployed_address, m);

            let value = simple_storage.get_value().call().await.unwrap();
            assert_eq!(value, "this");

            let event =
                simple_storage.value_changed_filter().query().await.unwrap();
            assert_eq!(event.len(), 1);
            assert_eq!(event[0].old_author, account);
            assert_eq!(event[0].author, account);
            assert_eq!(event[0].old_value, "initial value");
            assert_eq!(event[0].new_value, "this");

            let event = simple_storage
                .value_changed_filter()
                .topic1(account)
                .query()
                .await
                .unwrap();
            assert_eq!(event.len(), 1);

            let event = simple_storage
                .value_changed_filter()
                .topic1(Address::zero())
                .query()
                .await
                .unwrap();
            assert_eq!(event.len(), 0);

            let bloom = blocks.1.logs_bloom;
            assert!(utils::contains_address(&bloom, &deployed_address));
            assert!(utils::contains_topic(&bloom, &account));
            assert!(utils::contains_topic(&bloom, &U256::from(1)));
        }

        // Test at block_hash2
        {
            let m = env.fold_access(blocks.2);
            let simple_storage = SimpleStorage::new(deployed_address, m);

            let value = simple_storage.get_value().call().await.unwrap();
            assert_eq!(value, "that");

            let event =
                simple_storage.value_changed_filter().query().await.unwrap();
            assert_eq!(event.len(), 1);
            assert_eq!(event[0].old_author, account);
            assert_eq!(event[0].author, account);
            assert_eq!(event[0].old_value, "this");
            assert_eq!(event[0].new_value, "that");

            let bloom = blocks.2.logs_bloom;
            assert!(utils::contains_address(&bloom, &deployed_address));
            assert!(utils::contains_topic(&bloom, &account));
            assert!(utils::contains_topic(&bloom, &U256::from(2)));
        }

        // Test at block_hash3
        {
            let m = env.fold_access(blocks.3);
            let simple_storage = SimpleStorage::new(deployed_address, m);

            let value = simple_storage.get_value().call().await.unwrap();
            assert_eq!(value, "other");

            let event =
                simple_storage.value_changed_filter().query().await.unwrap();
            assert_eq!(event.len(), 1);
            assert_eq!(event[0].old_author, account);
            assert_eq!(event[0].author, account);
            assert_eq!(event[0].old_value, "that");
            assert_eq!(event[0].new_value, "other");

            // test override block
            let value = simple_storage
                .get_value()
                .block(blocks.0.hash)
                .call()
                .await
                .unwrap();
            assert_eq!(value, "initial value");

            // Default overrides given block.
            let event = simple_storage
                .value_changed_filter()
                .at_block_hash(blocks.0.hash)
                .query()
                .await
                .unwrap();
            assert_eq!(event.len(), 1);
            assert_eq!(event[0].old_author, account);
            assert_eq!(event[0].author, account);
            assert_eq!(event[0].old_value, "that");
            assert_eq!(event[0].new_value, "other");

            let bloom = blocks.3.logs_bloom;
            assert!(utils::contains_address(&bloom, &deployed_address));
            assert!(utils::contains_topic(&bloom, &account));
            assert!(utils::contains_topic(&bloom, &U256::from(3)));
        }
    }
}
