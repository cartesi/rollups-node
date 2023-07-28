// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_recursion::async_recursion;
use async_trait::async_trait;
use tokio::sync::Semaphore;

/// Outcome of fetching events.
#[derive(Debug)]
pub enum FetchOutcome<T, E> {
    /// Data fetched in a terminal state. The default case.
    Terminal(Result<T, E>),
    /// Failed to fetch data due to requesting events for a too large block
    /// range. Caller should retry by splitting the input data into
    /// sub-ranges and call fetch on every sub-range.
    RangeTooLarge,
}

#[async_trait]
pub trait PartitionProvider<Event, PartitionData>
where
    Event: Send,
    PartitionData: Send + Sync,
{
    type ProviderErr: std::error::Error + Send;

    async fn fetch_events_with_range(
        &self,
        data: &PartitionData,
        from_block: u64,
        to_block: u64,
    ) -> FetchOutcome<Vec<Event>, Self::ProviderErr> {
        match self
            .fetch_events_with_range_inner(data, from_block, to_block)
            .await
        {
            Ok(events) => {
                if events.len() > self.maximum_events_per_response() {
                    FetchOutcome::RangeTooLarge
                } else {
                    FetchOutcome::Terminal(Ok(events))
                }
            }
            Err(e) => {
                if self.should_retry_with_partition(&e) {
                    if from_block >= to_block {
                        FetchOutcome::Terminal(Err(e))
                    } else {
                        FetchOutcome::RangeTooLarge
                    }
                } else {
                    FetchOutcome::Terminal(Err(e))
                }
            }
        }
    }

    async fn fetch_events_with_range_inner(
        &self,
        data: &PartitionData,
        from_block: u64,
        to_block: u64,
    ) -> Result<Vec<Event>, Self::ProviderErr>;

    fn should_retry_with_partition(&self, err: &Self::ProviderErr) -> bool;

    fn maximum_events_per_response(&self) -> usize {
        usize::MAX
    }
}

#[derive(Debug)]
pub struct PartitionEvents<'a, Event, PartitionData, Provider>
where
    Event: Send + Sync,
    PartitionData: Send + Sync,
    Provider: PartitionProvider<Event, PartitionData> + Send + Sync,
{
    semaphore: Semaphore,
    provider: &'a Provider,
    partition_data: &'a PartitionData,
    __phantom: std::marker::PhantomData<Event>,
}

impl<'a, E, D, P> PartitionEvents<'a, E, D, P>
where
    E: Send + Sync,
    D: Send + Sync,
    P: PartitionProvider<E, D> + Send + Sync,
{
    pub fn new(
        concurrent_workers: usize,
        provider: &'a P,
        partition_data: &'a D,
    ) -> Self {
        let semaphore = Semaphore::new(concurrent_workers);
        PartitionEvents {
            semaphore,
            provider,
            partition_data,
            __phantom: std::marker::PhantomData,
        }
    }

    pub async fn get_events(
        &'a self,
        start_block: u64,
        end_block: u64,
    ) -> Result<Vec<E>, Vec<P::ProviderErr>> {
        self.get_events_rec(start_block, end_block).await
    }

    #[async_recursion]
    async fn get_events_rec(
        &'a self,
        start_block: u64,
        end_block: u64,
    ) -> Result<Vec<E>, Vec<P::ProviderErr>> {
        let res = {
            // Make number of concurrent fetches bounded.
            let _permit = self.semaphore.acquire().await;
            self.provider
                .fetch_events_with_range(
                    self.partition_data,
                    start_block,
                    end_block,
                )
                .await
        };

        match res {
            FetchOutcome::Terminal(result) => result.map_err(|e| vec![e]),
            FetchOutcome::RangeTooLarge => {
                let middle = {
                    let blocks = 1 + end_block - start_block;
                    let half = blocks / 2;
                    start_block + half - 1
                };

                let first_fut = self.get_events_rec(start_block, middle);
                let second_fut = self.get_events_rec(middle + 1, end_block);

                let (first_res, second_res) =
                    futures::join!(first_fut, second_fut);

                match (first_res, second_res) {
                    (Ok(mut first), Ok(second)) => {
                        first.extend(second);
                        Ok(first)
                    }

                    (Err(mut first), Err(second)) => {
                        first.extend(second);
                        Err(first)
                    }

                    (Err(err), _) | (_, Err(err)) => Err(err),
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::PartitionEvents;
    use super::PartitionProvider;
    use crate::state_fold::delegate_access::partition_events::FetchOutcome;
    use async_trait::async_trait;

    pub struct MockProvider1 {}
    pub struct MockProviderData {}

    #[async_trait]
    impl PartitionProvider<u64, MockProviderData> for MockProvider1 {
        type ProviderErr = std::io::Error;

        async fn fetch_events_with_range_inner(
            &self,
            _: &MockProviderData,
            start_block: u64,
            end_block: u64,
        ) -> Result<Vec<u64>, Self::ProviderErr> {
            async {
                if start_block == end_block {
                    Ok(vec![start_block])
                } else {
                    Err(std::io::Error::new(
                        std::io::ErrorKind::Other,
                        "oh no!",
                    ))
                }
            }
            .await
        }

        fn should_retry_with_partition(&self, _: &Self::ProviderErr) -> bool {
            true
        }
    }

    #[tokio::test]
    async fn test_partition_simple1() {
        let provider = MockProvider1 {};
        let partition =
            PartitionEvents::new(1, &provider, &MockProviderData {});

        let ret = partition.get_events(0, 10000).await;
        assert_eq!((0..=10000).collect::<Vec<u64>>(), ret.unwrap());
    }

    #[tokio::test]
    async fn test_partition_simple2() {
        let provider = MockProvider1 {};
        let partition =
            PartitionEvents::new(16, &provider, &MockProviderData {});

        let ret = partition.get_events(0, 10000).await;
        assert_eq!((0..=10000).collect::<Vec<u64>>(), ret.unwrap());
    }

    pub struct MockProvider2 {}

    #[async_trait]
    impl PartitionProvider<u64, MockProviderData> for MockProvider2 {
        type ProviderErr = std::io::Error;

        async fn fetch_events_with_range_inner(
            &self,
            _: &MockProviderData,
            start_block: u64,
            end_block: u64,
        ) -> Result<Vec<u64>, Self::ProviderErr> {
            async {
                if end_block - start_block <= 4 {
                    // println!("{} {}", start_block, end_block);
                    Ok((start_block..=end_block).collect::<Vec<u64>>())
                } else {
                    Err(std::io::Error::new(
                        std::io::ErrorKind::Other,
                        "oh no!",
                    ))
                }
            }
            .await
        }

        fn should_retry_with_partition(&self, _: &Self::ProviderErr) -> bool {
            true
        }
    }

    #[tokio::test]
    async fn test_partition_simple3() {
        let provider = MockProvider2 {};
        let partition =
            PartitionEvents::new(16, &provider, &MockProviderData {});

        let ret = partition.get_events(0, 10000).await;
        assert_eq!((0..=10000).collect::<Vec<u64>>(), ret.unwrap());
    }

    #[tokio::test]
    async fn test_partition_provider_fails_due_to_block_range() {
        let provider = MockProvider2 {};

        let outcome = provider
            .fetch_events_with_range(&MockProviderData {}, 0, 10000)
            .await;

        assert!(matches!(outcome, FetchOutcome::RangeTooLarge));
    }

    #[tokio::test]
    async fn test_partition_provider_has_too_large_response() {
        use std::io::Error;

        struct MockProvider {}

        #[async_trait]
        impl PartitionProvider<u64, MockProviderData> for MockProvider {
            type ProviderErr = Error;

            async fn fetch_events_with_range_inner(
                &self,
                _data: &MockProviderData,
                _from_block: u64,
                _to_block: u64,
            ) -> Result<Vec<u64>, Self::ProviderErr> {
                Ok([0].repeat(10))
            }

            fn should_retry_with_partition(
                &self,
                _err: &Self::ProviderErr,
            ) -> bool {
                false
            }

            fn maximum_events_per_response(&self) -> usize {
                5
            }
        }

        let provider = MockProvider {};

        let outcome = provider
            .fetch_events_with_range(&MockProviderData {}, 0, 10000)
            .await;

        assert!(matches!(outcome, FetchOutcome::RangeTooLarge));
    }
}
