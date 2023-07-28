// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::block_archive::Result as BlockArchiveResult;
use super::block_archive::{BlockArchive, BlockArchiveError};

use state_fold_types::{Block, BlockError, BlockStreamItem, BlocksSince};

use ethers::providers::{Middleware, Provider, ProviderError, Ws};
use std::sync::Arc;
use tokio::sync::{oneshot, watch};
use tokio_stream::{Stream, StreamExt};

use snafu::{ResultExt, Snafu};

#[derive(Debug, Snafu, Clone)]
pub enum BlockSubscriberError<M: Middleware + 'static> {
    #[snafu(display("Ethers provider error: {}", source))]
    EthersProviderError { source: M::Error },

    #[snafu(display("Got incomplete block"))]
    BlockIncomplete { err: BlockError },

    #[snafu(display("New block subscriber timeout: {}", source))]
    NewBlockSubscriberTimeout { source: Arc<std::io::Error> },

    #[snafu(display("Ethers subscription dropped"))]
    EthersSubscriptionDropped {},

    #[snafu(display("Failed to stablish Ws connection"))]
    ProviderWsConnectError { source: Arc<ProviderError> },
}

pub type Result<T, M> = std::result::Result<T, Arc<BlockSubscriberError<M>>>;

#[derive(Debug, Snafu)]
pub enum SubscriptionError<M: Middleware + 'static> {
    #[snafu(display("Subscriber dropped: {}", source))]
    SubscriptionDropped {
        source: tokio::sync::watch::error::RecvError,
    },

    #[snafu(display("Error while accessing block archive: {}", source))]
    ArchiveError { source: BlockArchiveError<M> },
}
pub type SubscriptionResult<T, M> =
    std::result::Result<T, SubscriptionError<M>>;

pub struct BlockSubscriber<M: Middleware + 'static> {
    pub handle: tokio::task::JoinHandle<Result<(), Provider<Ws>>>,
    pub block_archive: Arc<BlockArchive<M>>,
    pub shutdown_notifier: watch::Receiver<Option<Result<(), Provider<Ws>>>>,

    new_block_alarm: watch::Receiver<()>,
    _kill_switch: oneshot::Sender<()>,
}

impl<M: Middleware + 'static> BlockSubscriber<M> {
    pub async fn start(
        middleware: Arc<M>,
        ws_url: String,
        subscriber_timeout: std::time::Duration,
        max_depth: usize,
    ) -> super::block_archive::Result<Self, M> {
        let archive =
            Arc::new(BlockArchive::new(middleware.clone(), max_depth).await?);

        let (kill_tx, kill_rx) = oneshot::channel();
        let (shutdown_sender, shutdown_notifier) = watch::channel(None);
        let (new_block_tx, new_block_alarm) = watch::channel(());

        let block_archive = archive.clone();

        // Create background task and detach it.
        let handle = tokio::spawn(async move {
            // Create future of `background_process` main loop. This
            // future will run against the kill_switch.
            let task = background_process(
                ws_url,
                archive,
                new_block_tx,
                subscriber_timeout,
            );
            tokio::pin!(task);

            tokio::select! {
                res = &mut task => {
                    tracing::error!(
                        "subscriber `background_process` exited with error: {:?}",
                        res
                    );

                    let _ = shutdown_sender.send(Some(res.clone()));
                    res
                },

                _ = kill_rx => {
                    Ok(())
                }
            }
        });

        Ok(Self {
            handle,
            block_archive,
            shutdown_notifier,
            new_block_alarm,
            _kill_switch: kill_tx,
        })
    }

    pub async fn wait_for_completion(&self) -> Result<(), Provider<Ws>> {
        let mut notifier = self.shutdown_notifier.clone();

        {
            let ret = notifier.borrow();
            if let Some(r) = ret.clone() {
                return r;
            }
        }

        loop {
            notifier.changed().await.expect(
                "`shutdown_notifier` should never be closed in this context",
            );

            let ret = notifier.borrow();
            if let Some(r) = ret.clone() {
                break r;
            } else {
                continue;
            }
        }
    }

    pub async fn subscribe_new_blocks_at_depth(
        &self,
        depth: usize,
    ) -> BlockArchiveResult<
        impl Stream<Item = SubscriptionResult<BlockStreamItem, M>> + Unpin,
        M,
    > {
        let archive = self.block_archive.clone();
        let mut alarm = self.new_block_alarm.clone();

        let mut previous = archive.block_at_depth(depth).await?;

        Ok(Box::pin(async_stream::try_stream! {
            while let () = alarm.changed().await.context(SubscriptionDroppedSnafu)? {
                let diff = archive
                    .blocks_since(depth, Arc::clone(&previous))
                    .await
                    .context(ArchiveSnafu)?;

                match diff {
                    BlocksSince::Normal(blocks) => {
                        if let Some(p) = blocks.last() {
                            previous = p.clone();
                            for b in blocks {
                                yield BlockStreamItem::NewBlock(b);
                            }
                        }
                    }

                    BlocksSince::Reorg(blocks) => {
                        if let Some(p) = blocks.last() {
                            previous = p.clone();
                        }

                        yield BlockStreamItem::Reorg(blocks);
                    }
                }
            }
        }))
    }
}

#[tracing::instrument(skip_all)]
async fn background_process<M: Middleware + 'static>(
    ws_url: String,
    block_archive: Arc<BlockArchive<M>>,
    new_block_alarm: watch::Sender<()>,
    subscriber_timeout: std::time::Duration,
) -> Result<(), Provider<Ws>> {
    loop {
        tracing::trace!("Starting Ws connection at {}", ws_url);
        let provider = Provider::connect(ws_url.clone())
            .await
            .map_err(Arc::new)
            .context(ProviderWsConnectSnafu)?;

        tracing::trace!("Starting block subscription");
        let subscription = provider
            .subscribe_blocks()
            .await
            .context(EthersProviderSnafu)
            .map(|subscription| {
                Box::pin(subscription.timeout(subscriber_timeout).map(|x| {
                    let block_header = x
                        .map_err(|e| Arc::new(e.into()))
                        .context(NewBlockSubscriberTimeoutSnafu)?;

                    let block = block_header
                        .try_into()
                        .map_err(|err| BlockIncompleteSnafu { err }.build())?;

                    Ok(Arc::new(block))
                }))
            })?;

        match listen_and_broadcast(
            block_archive.clone(),
            &new_block_alarm,
            subscription,
        )
        .await
        {
            Err(e) => {
                tracing::warn!(
                    "`listen_and_broadcast` error `{}`, retrying subscription",
                    e
                );

                continue;
            }

            Ok(()) => {
                tracing::debug!(
                    "Stopping BlockSubscriber `background_process`"
                );
                return Ok(());
            }
        }
    }
}

#[tracing::instrument(skip_all)]
async fn listen_and_broadcast<M: Middleware + 'static>(
    block_archive: Arc<BlockArchive<M>>,
    new_block_alarm: &watch::Sender<()>,
    mut subscription: impl Stream<Item = Result<Arc<Block>, M>> + Send + Unpin,
) -> Result<(), M> {
    // Listen to new blocks and notify subscribers.
    loop {
        // Block on waiting for new block.
        let new_head = subscription
            .next()
            .await
            .ok_or(snafu::NoneError)
            .context(EthersSubscriptionDroppedSnafu)??;

        tracing::trace!(
            "Subscriber received block with number `{}` and hash `{}`",
            new_head.number,
            new_head.hash
        );

        // Insert in archive
        let _ = block_archive.update_latest_block(new_head).await;

        // Send new block to subscribers.
        if new_block_alarm.send(()).is_err() {
            tracing::debug!(
                "BlockSubscriber has no subscriptions, stopping `listen_and_broadcast`"
            );

            return Ok(());
        }
    }
}
