// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use block_history::BlockSubscriber;
use state_fold_types::BlockStreamItem;

use ethers::core::utils::Geth;
use ethers::providers::{Http, Middleware, Provider};
use state_fold_types::Block;

use std::sync::Arc;
use tokio_stream::StreamExt;

#[tokio::test]
async fn subscribe_test() -> Result<(), Box<dyn std::error::Error>> {
    let geth = Geth::new().block_time(1u64).spawn();
    let provider = Arc::new(Provider::<Http>::try_from(geth.endpoint())?);

    let block_history = BlockSubscriber::start(
        Arc::clone(&provider),
        geth.ws_endpoint(),
        std::time::Duration::from_secs(3),
        100,
    )
    .await?;

    let mut subscription_latest =
        block_history.subscribe_new_blocks_at_depth(0).await?;

    let current_block =
        get_new_block(subscription_latest.next().await.unwrap()?).number;
    for i in 0u64..10 {
        let head_latest =
            get_new_block(subscription_latest.next().await.unwrap()?).number;
        assert_eq!(current_block + i + 1, head_latest);
    }

    let current_block = provider.get_block_number().await?;
    let mut subscription_past =
        block_history.subscribe_new_blocks_at_depth(1).await?;
    let mut subscription_pastest =
        block_history.subscribe_new_blocks_at_depth(10).await?;

    for i in 0u64..5 {
        let head_past =
            get_new_block(subscription_past.next().await.unwrap()?).number;
        assert_eq!(current_block + i, head_past);
    }

    for i in 0u64..5 {
        let head_pastest =
            get_new_block(subscription_pastest.next().await.unwrap()?).number;
        assert_eq!(current_block - 9 + i, head_pastest);
    }

    Ok(())
}

fn get_new_block(b: BlockStreamItem) -> Arc<Block> {
    match b {
        BlockStreamItem::NewBlock(b) => b,
        BlockStreamItem::Reorg(_) => unreachable!(),
    }
}
