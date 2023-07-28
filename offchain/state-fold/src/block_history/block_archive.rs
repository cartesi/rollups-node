// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::block_tree::BlockTree;

use state_fold_types::Block;
use state_fold_types::BlocksSince;

use ethers::providers::Middleware;
use ethers::types::{BlockId, BlockNumber, H256, U64};

use std::convert::TryInto;
use std::sync::Arc;
use tokio::sync::RwLock;

use snafu::{ensure, ResultExt, Snafu};

#[derive(Debug, Snafu)]
pub enum BlockArchiveError<M: ethers::providers::Middleware + 'static> {
    // #[snafu(display("Ethers provider error: {}", source))]
    // EthersProviderError {
    //     source: Box<dyn std::error::Error + Send + Sync>,
    // },
    #[snafu(display("Ethers provider error: {}", source))]
    EthersProviderError { source: M::Error },

    #[snafu(display("Requested block incomplete"))]
    BlockIncomplete {},

    #[snafu(display("Requested block unavailable"))]
    BlockUnavailable {},

    #[snafu(display(
        "Previous block `{}` ahead of latest block `{}`",
        previous_number,
        latest_number
    ))]
    PreviousAheadOfLatest {
        previous_number: usize,
        latest_number: usize,
    },

    #[snafu(display(
        "The depth `{}` is over the `{}` maximum",
        depth,
        max_depth
    ))]
    BlockOutOfRange { depth: usize, max_depth: usize },

    #[snafu(display(
        "Depth of `{}` higher than latest block `{}`",
        depth,
        latest
    ))]
    DepthTooHigh { depth: usize, latest: usize },
}

pub type Result<T, M> = std::result::Result<T, BlockArchiveError<M>>;

pub struct BlockArchive<M: Middleware> {
    middleware: Arc<M>,
    block_tree: RwLock<BlockTree>,
    max_depth: usize,
}

impl<M: Middleware + 'static> BlockArchive<M> {
    pub(crate) async fn new(
        middleware: Arc<M>,
        max_depth: usize,
    ) -> Result<Self, M> {
        let block_tree = {
            let latest_block =
                fetch_block(middleware.as_ref(), BlockNumber::Latest).await?;

            RwLock::new(BlockTree::new(Arc::new(latest_block)))
        };

        Ok(Self {
            middleware,
            block_tree,
            max_depth,
        })
    }

    pub(crate) async fn update_latest_block(
        &self,
        block: Arc<Block>,
    ) -> Result<(), M> {
        let mut block_tree = self.block_tree.write().await;

        let mut parent_number = block.number - 1;
        let mut parent_hash = block.parent_hash;
        block_tree.update_latest_block(block);

        while let Some(b) = block_tree.block_with_number(&parent_number) {
            if b.hash == parent_hash {
                break;
            }

            let new_block = self.fetch_block(parent_hash).await?;
            parent_number = new_block.number - 1;
            parent_hash = new_block.parent_hash;
            block_tree.insert_block(new_block);
        }

        Ok(())
    }
}

impl<M: Middleware + 'static> BlockArchive<M> {
    pub async fn latest_block(&self) -> Arc<Block> {
        self.block_tree.read().await.latest_block()
    }

    pub async fn block_at_depth(&self, depth: usize) -> Result<Arc<Block>, M> {
        let latest = self.latest_block().await;

        ensure!(
            U64::from(depth) <= latest.number,
            DepthTooHighSnafu {
                depth,
                latest: latest.number.as_usize()
            }
        );

        let block_number = latest.number - U64::from(depth);
        self.block_with_number(block_number).await
    }

    pub async fn block_with_number(
        &self,
        block_number: U64,
    ) -> Result<Arc<Block>, M> {
        if let Some(b) = self
            .block_tree
            .read()
            .await
            .block_with_number(&block_number)
        {
            return Ok(b);
        }

        let b = self.fetch_block(block_number).await?;
        self.insert_block(Arc::clone(&b)).await;

        Ok(b)
    }

    pub async fn blocks_since(
        &self,
        depth: usize,
        previous: Arc<Block>,
    ) -> Result<BlocksSince, M> {
        let latest = self.latest_block().await;

        ensure!(
            depth <= self.max_depth,
            BlockOutOfRangeSnafu {
                depth,
                max_depth: self.max_depth
            }
        );

        ensure!(
            previous.number <= latest.number,
            PreviousAheadOfLatestSnafu {
                previous_number: previous.number.as_usize(),
                latest_number: latest.number.as_usize()
            }
        );

        let diff = latest.number.as_usize() - previous.number.as_usize();
        if diff <= depth {
            return Ok(BlocksSince::Normal(Vec::new()));
        }
        let number_of_new_blocks = diff - depth;

        let diff = self
            .build_ancestral_stack(previous, latest, number_of_new_blocks)
            .await?;

        Ok(diff)
    }

    pub async fn block_with_hash(
        &self,
        block_hash: &H256,
    ) -> Result<Arc<Block>, M> {
        if let Some(b) =
            self.block_tree.read().await.block_with_hash(block_hash)
        {
            return Ok(b);
        }

        let b = self.fetch_block(*block_hash).await?;
        self.insert_block(Arc::clone(&b)).await;

        Ok(b)
    }
}

impl<M: Middleware + 'static> BlockArchive<M> {
    async fn fetch_block<T: Into<BlockId> + Send + Sync>(
        &self,
        block_id: T,
    ) -> Result<Arc<Block>, M> {
        Ok(Arc::new(
            fetch_block(self.middleware.as_ref(), block_id).await?,
        ))
    }

    async fn insert_block(&self, block: Arc<Block>) {
        self.block_tree.write().await.insert_block(block);
    }

    async fn build_ancestral_stack(
        &self,
        previous: Arc<Block>,
        leaf: Arc<Block>,
        number_of_new_blocks: usize,
    ) -> Result<BlocksSince, M> {
        let mut stack =
            self.build_stack_from_leaf(previous.number, leaf).await?;

        let len = stack.len();

        match stack.last() {
            Some(last) if last.hash == previous.hash => {
                stack.pop();
                stack.reverse();
                stack.truncate(number_of_new_blocks);
                Ok(BlocksSince::Normal(stack))
            }

            Some(_) => {
                self.extend_stack_to_ancestor(&mut stack, previous).await?;
                stack.reverse();

                let spillover = stack.len() - len;
                stack.truncate(number_of_new_blocks + spillover);

                Ok(BlocksSince::Reorg(stack))
            }

            None => Ok(BlocksSince::Normal(stack)),
        }
    }

    async fn build_stack_from_leaf(
        &self,
        ancestor_number: U64,
        leaf: Arc<Block>,
    ) -> Result<Vec<Arc<Block>>, M> {
        let mut stack = Vec::new();
        let mut current = leaf.clone();

        while current.number != ancestor_number {
            let new_current =
                self.block_with_hash(&current.parent_hash).await?;
            stack.push(current);
            current = new_current;
        }

        stack.push(current);
        Ok(stack)
    }

    async fn extend_stack_to_ancestor(
        &self,
        stack: &mut Vec<Arc<Block>>,
        uncle: Arc<Block>,
    ) -> Result<(), M> {
        let last = stack.last().expect(
            "should not call `extend_stack_to_ancestor` with empty stack",
        );

        assert_eq!(uncle.number, last.number);
        assert_ne!(uncle.hash, last.hash);

        let mut current_uncle_parent = uncle.parent_hash;
        let mut current_parent = last.parent_hash;

        while current_parent != current_uncle_parent {
            let current = self.block_with_hash(&current_parent).await?;
            current_parent = current.parent_hash;

            let current_uncle =
                self.block_with_hash(&current_uncle_parent).await?;
            current_uncle_parent = current_uncle.parent_hash;

            stack.push(current);
        }

        Ok(())
    }
}

pub async fn fetch_block<
    M: Middleware + 'static,
    T: Into<BlockId> + Send + Sync,
>(
    middleware: &M,
    block_id: T,
) -> Result<Block, M> {
    middleware
        .get_block(block_id)
        .await
        .context(EthersProviderSnafu)?
        .ok_or(snafu::NoneError)
        .context(BlockUnavailableSnafu)?
        .try_into()
        .map_err(|_| snafu::NoneError)
        .context(BlockIncompleteSnafu)
}

pub async fn current_block_number<M: Middleware + 'static>(
    middleware: &M,
) -> Result<U64, M> {
    middleware
        .get_block_number()
        .await
        .context(EthersProviderSnafu)
}

pub async fn fetch_block_at_depth<M: Middleware + 'static>(
    middleware: &M,
    current: U64,
    depth: usize,
) -> Result<Block, M> {
    ensure!(
        current > depth.into(),
        DepthTooHighSnafu {
            depth,
            latest: current.as_usize()
        }
    );

    fetch_block(middleware, current - depth).await
}

#[cfg(test)]
mod tests {
    use super::BlockArchive;
    use state_fold_test::mock_middleware::MockMiddleware;
    use state_fold_types::BlocksSince;

    use std::sync::Arc;

    async fn instantiate_all(
    ) -> (Arc<MockMiddleware>, BlockArchive<MockMiddleware>) {
        let max_depth = 100;
        let m = MockMiddleware::new(128).await;
        let archive =
            BlockArchive::new(Arc::clone(&m), max_depth).await.unwrap();

        (m, archive)
    }

    async fn update_archive_with_latest(
        m: &MockMiddleware,
        a: &BlockArchive<MockMiddleware>,
    ) {
        a.update_latest_block(Arc::new(m.get_latest_block().await.unwrap()))
            .await
            .unwrap();
    }

    #[tokio::test]
    async fn basic_test() {
        let (m, archive) = instantiate_all().await;

        let start_block = m.get_latest_block().await.unwrap();
        assert_eq!(archive.latest_block().await.hash, start_block.hash);
        assert_eq!(
            archive.block_with_number(128.into()).await.unwrap().hash,
            start_block.hash
        );

        let previous = archive.block_at_depth(8).await.unwrap();
        assert_eq!(previous.number, 120.into());
        assert_eq!(
            archive.block_with_number(120.into()).await.unwrap().hash,
            previous.hash
        );
    }

    #[tokio::test]
    async fn straight_test() {
        let (m, archive) = instantiate_all().await;

        let mut previous = archive.block_at_depth(8).await.unwrap();
        let mut latest = m.get_latest_block().await.unwrap().hash;

        for i in 0..128 {
            latest = m.add_block(latest).await.unwrap();
            update_archive_with_latest(&m, &archive).await;
            let diff = archive.blocks_since(8, previous).await.unwrap();

            previous = match diff {
                BlocksSince::Normal(v) => {
                    assert_eq!(v.len(), 1);
                    let p = v[0].clone();
                    assert_eq!(p.number, (121 + i).into());
                    p
                }

                BlocksSince::Reorg(_) => {
                    panic!("Expected no reorg");
                }
            };
        }
    }

    #[tokio::test]
    async fn descending_depth_test() {
        let (_m, archive) = instantiate_all().await;
        let previous = archive.block_at_depth(8).await.unwrap();

        for i in (0..8).rev() {
            let diff = archive.blocks_since(i, previous.clone()).await.unwrap();
            assert!(matches!(diff, BlocksSince::Normal(v) if v.len() == 8 - i));
        }
    }

    #[tokio::test]
    async fn skipping_test() {
        let (m, archive) = instantiate_all().await;

        let mut previous = archive.block_at_depth(8).await.unwrap();
        let mut latest = m.get_latest_block().await.unwrap().hash;

        for i in 0..128 {
            latest = m.add_block(latest).await.unwrap();

            if i % 2 != 0 {
                update_archive_with_latest(&m, &archive).await;

                let diff = archive.blocks_since(8, previous).await.unwrap();

                previous = match diff {
                    BlocksSince::Normal(v) => {
                        assert_eq!(v.len(), 2);
                        let p = v[0].clone();
                        assert_eq!(p.number, (120 + i).into());
                        let p = v[1].clone();
                        assert_eq!(p.number, (121 + i).into());
                        p
                    }

                    BlocksSince::Reorg(_) => {
                        panic!("Expected no reorg");
                    }
                };
            }
        }
    }

    #[tokio::test]
    async fn branching_no_reorg_test() {
        let (m, archive) = instantiate_all().await;

        let mut previous = archive.block_at_depth(8).await.unwrap();
        let mut latest = m.get_latest_block().await.unwrap().hash;

        let mut temp_latest = latest;
        for i in 0..7 {
            temp_latest = m.add_block(temp_latest).await.unwrap();
            update_archive_with_latest(&m, &archive).await;
            let diff = archive.blocks_since(8, previous.clone()).await.unwrap();

            previous = match diff {
                BlocksSince::Normal(v) => {
                    assert_eq!(v.len(), 1);
                    let p = v[0].clone();
                    assert_eq!(p.number, (121 + i).into());
                    p
                }

                BlocksSince::Reorg(_) => {
                    panic!("Expected no reorg");
                }
            };
        }

        for _ in 0..7 {
            latest = m.add_block(latest).await.unwrap();
            update_archive_with_latest(&m, &archive).await;
            let diff = archive.blocks_since(8, previous.clone()).await.unwrap();

            match diff {
                BlocksSince::Normal(v) => {
                    assert_eq!(v.len(), 0);
                }

                BlocksSince::Reorg(_) => {
                    panic!("Expected no reorg");
                }
            };
        }

        m.add_block(latest).await.unwrap();
        update_archive_with_latest(&m, &archive).await;
        let diff = archive.blocks_since(8, previous).await.unwrap();

        match diff {
            BlocksSince::Normal(v) => {
                assert_eq!(v.len(), 1);
                let p = v[0].clone();
                assert_eq!(p.number, (128).into());
            }

            BlocksSince::Reorg(_) => {
                panic!("Expected no reorg");
            }
        };
    }

    #[tokio::test]
    async fn branching_reorg_test() {
        let (m, archive) = instantiate_all().await;

        let mut previous = archive.block_at_depth(8).await.unwrap();
        let mut latest = m.get_latest_block().await.unwrap().hash;

        let mut temp_latest = latest;
        for i in 0..9 {
            temp_latest = m.add_block(temp_latest).await.unwrap();
            update_archive_with_latest(&m, &archive).await;
            let diff = archive.blocks_since(8, previous.clone()).await.unwrap();

            previous = match diff {
                BlocksSince::Normal(v) => {
                    assert_eq!(v.len(), 1);
                    let p = v[0].clone();
                    assert_eq!(p.number, (121 + i).into());
                    p
                }

                BlocksSince::Reorg(_) => {
                    panic!("Expected no reorg");
                }
            };
        }

        for _ in 0..9 {
            latest = m.add_block(latest).await.unwrap();
            update_archive_with_latest(&m, &archive).await;
            let diff = archive.blocks_since(8, previous.clone()).await.unwrap();

            match diff {
                BlocksSince::Normal(v) => {
                    assert_eq!(v.len(), 0);
                }

                BlocksSince::Reorg(_) => {
                    panic!("Expected no reorg");
                }
            };
        }

        m.add_block(latest).await.unwrap();
        update_archive_with_latest(&m, &archive).await;
        let diff = archive.blocks_since(8, previous).await.unwrap();

        match diff {
            BlocksSince::Normal(_) => {
                panic!("Expected reorg");
            }

            BlocksSince::Reorg(v) => {
                assert_eq!(v.len(), 1);
                let p = v[0].clone();
                assert_eq!(p.number, 129.into());
            }
        };
    }
}
