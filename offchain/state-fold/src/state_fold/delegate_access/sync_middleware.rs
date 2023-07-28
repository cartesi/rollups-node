// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::error::*;
use super::partition_events::*;

use ethers::core::types::{
    transaction::eip2718::TypedTransaction, BlockId, BlockNumber, Bytes,
    Filter, FilterBlockOption, Log, U64,
};
use ethers::providers::{FromErr, Middleware};

use async_trait::async_trait;
use std::sync::Arc;

use snafu::ResultExt;

#[derive(Debug)]
pub struct SyncMiddleware<M> {
    inner: Arc<M>,
    genesis: U64,
    block_number: U64,
    query_limit_error_codes: Vec<i32>,
    concurrent_events_fetch: usize,
    maximum_events_per_response: usize,
}

impl<M> SyncMiddleware<M>
where
    M: Middleware,
{
    pub fn new(
        inner: Arc<M>,
        genesis: U64,
        block_number: U64,
        query_limit_error_codes: Vec<i32>,
        concurrent_events_fetch: usize,
        maximum_events_per_response: usize,
    ) -> Self {
        Self {
            inner,
            genesis,
            block_number,
            query_limit_error_codes,
            concurrent_events_fetch,
            maximum_events_per_response,
        }
    }

    pub fn get_inner(&self) -> Arc<M> {
        Arc::clone(&self.inner)
    }
}

#[async_trait]
impl<M> Middleware for SyncMiddleware<M>
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
        // blocks given during instantiation.
        let block = block.or_else(|| Some(self.block_number.into()));
        self.inner().call(tx, block).await.map_err(FromErr::from)
    }

    async fn get_logs(
        &self,
        filter: &Filter,
    ) -> std::result::Result<Vec<Log>, Self::Error> {
        let partition_events =
            PartitionEvents::new(self.concurrent_events_fetch, self, filter);

        let (start, end) = match filter.block_option {
            FilterBlockOption::Range {
                from_block: Some(BlockNumber::Number(s)),
                to_block: Some(BlockNumber::Number(e)),
            } => (s.as_u64(), e.as_u64()),

            FilterBlockOption::Range {
                from_block: Some(BlockNumber::Number(s)),
                ..
            } => (s.as_u64(), self.block_number.as_u64()),

            FilterBlockOption::Range {
                to_block: Some(BlockNumber::Number(e)),
                ..
            } => (self.genesis.as_u64(), e.as_u64()),

            FilterBlockOption::AtBlockHash(h) => {
                let b = self
                    .inner
                    .get_block(h)
                    .await
                    .context(EthersProviderSnafu)?
                    .ok_or(snafu::NoneError)
                    .context(BlockUnavailableSnafu)?
                    .number
                    .ok_or(snafu::NoneError)
                    .context(BlockIncompleteSnafu)?
                    .as_u64();

                (b, b)
            }

            _ => (self.genesis.as_u64(), self.block_number.as_u64()),
        };

        let mut logs = partition_events
            .get_events(start, end)
            .await
            .map_err(|err_arr| PartitionSnafu { sources: err_arr }.build())?;

        super::utils::sort_logs(&mut logs)?;
        Ok(logs)
    }
}

#[async_trait]
impl<M> PartitionProvider<Log, Filter> for SyncMiddleware<M>
where
    M: Middleware + 'static,
{
    type ProviderErr = <<Self as Middleware>::Inner as Middleware>::Error;

    async fn fetch_events_with_range_inner(
        &self,
        data: &Filter,
        from_block: u64,
        to_block: u64,
    ) -> std::result::Result<Vec<Log>, Self::ProviderErr> {
        let filter = data.clone().from_block(from_block).to_block(to_block);
        let logs = self.inner().get_logs(&filter).await?;
        Ok(logs)
    }

    fn should_retry_with_partition(&self, err: &Self::ProviderErr) -> bool {
        for code in &self.query_limit_error_codes {
            let s = format!("{:?}", err);
            if s.contains(&code.to_string()) {
                return true;
            }
        }

        false
    }

    fn maximum_events_per_response(&self) -> usize {
        self.maximum_events_per_response
    }
}

#[cfg(test)]
pub mod tests {
    use crate::state_fold::StateFoldEnvironment;

    use ethers::providers::Middleware;
    use ethers::types::Address;
    use state_fold_types::Block;

    use state_fold_test::simple_storage::SimpleStorage;

    pub async fn sync_query_test<M: Middleware + 'static>(
        account: Address,
        deployed_address: Address,
        env: &StateFoldEnvironment<M, ()>,
        blocks: (&Block, &Block, &Block, &Block),
    ) {
        // Test at block_hash0
        {
            let m = env.sync_access(blocks.0);
            let simple_storage = SimpleStorage::new(deployed_address, m);

            let value = simple_storage.get_value().call().await.unwrap();
            assert_eq!(value, "initial value");

            let event =
                simple_storage.value_changed_filter().query().await.unwrap();
            assert_eq!(event.len(), 1);
            assert_eq!(event[0].old_author, Address::zero());
            assert_eq!(event[0].author, account);
            assert_eq!(event[0].old_value, "");
            assert_eq!(event[0].new_value, "initial value");
        }

        // Test at blocks._hash1
        {
            let m = env.sync_access(blocks.1);
            let simple_storage = SimpleStorage::new(deployed_address, m);
            let value = simple_storage.get_value().call().await.unwrap();
            assert_eq!(value, "this");

            let event =
                simple_storage.value_changed_filter().query().await.unwrap();
            assert_eq!(event.len(), 2);
            assert_eq!(event[1].old_author, account);
            assert_eq!(event[1].author, account);
            assert_eq!(event[1].old_value, "initial value");
            assert_eq!(event[1].new_value, "this");

            let event = simple_storage
                .value_changed_filter()
                .topic2(Address::zero())
                .query()
                .await
                .unwrap();
            assert_eq!(event.len(), 1);

            let event = simple_storage
                .value_changed_filter()
                .topic1(account)
                .query()
                .await
                .unwrap();
            assert_eq!(event.len(), 2);
        }

        // Test at blocks._hash2
        {
            let m = env.sync_access(blocks.2);
            let simple_storage = SimpleStorage::new(deployed_address, m);
            let value = simple_storage.get_value().call().await.unwrap();
            assert_eq!(value, "that");

            let event =
                simple_storage.value_changed_filter().query().await.unwrap();
            assert_eq!(event.len(), 3);
            assert_eq!(event[2].old_author, account);
            assert_eq!(event[2].author, account);
            assert_eq!(event[2].old_value, "this");
            assert_eq!(event[2].new_value, "that");
        }

        // Test at blocks._hash3
        {
            let m = env.sync_access(blocks.3);
            let simple_storage = SimpleStorage::new(deployed_address, m);
            let value = simple_storage.get_value().call().await.unwrap();
            assert_eq!(value, "other");

            let event =
                simple_storage.value_changed_filter().query().await.unwrap();
            assert_eq!(event.len(), 4);
            assert_eq!(event[3].old_author, account);
            assert_eq!(event[3].author, account);
            assert_eq!(event[3].old_value, "that");
            assert_eq!(event[3].new_value, "other");

            let value = simple_storage
                .get_value()
                .block(blocks.0.hash)
                .call()
                .await
                .unwrap();
            assert_eq!(value, "initial value");

            let event = simple_storage
                .value_changed_filter()
                .to_block(blocks.0.number)
                .query()
                .await
                .unwrap();
            assert_eq!(event.len(), 1);
        }
    }
}
