// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use eth_state_fold::Foldable;
use eth_state_fold_types::BlockStreamItem;
use rollups_events::{Address, DAppMetadata};
use std::sync::Arc;
use tokio_stream::StreamExt;
use tracing::{error, info, instrument, trace, warn};
use types::foldables::input_box::{InputBox, InputBoxInitialState};

use crate::{
    config::EthInputReaderConfig,
    error::{BlockArchiveSnafu, BrokerSnafu, EthInputReaderError},
    machine::{driver::MachineDriver, rollups_broker::BrokerFacade},
    metrics::EthInputReaderMetrics,
    setup::create_environment,
};

use snafu::{whatever, ResultExt};

#[instrument(level = "trace", skip_all)]
pub async fn start(
    config: EthInputReaderConfig,
    metrics: EthInputReaderMetrics,
) -> Result<(), EthInputReaderError> {
    info!("Setting up eth-input-reader with config: {:?}", config);

    let dapp_metadata = DAppMetadata {
        chain_id: config.chain_id,
        dapp_address: Address::new(config.dapp_deployment.dapp_address.into()),
    };

    trace!("Creating broker connection");
    let broker =
        BrokerFacade::new(config.broker_config.clone(), dapp_metadata.clone())
            .await
            .context(BrokerSnafu)?;

    trace!("Creating block subscription, environment and context");
    let (subscriber, env, mut context) =
        create_environment(&config, &broker, dapp_metadata, metrics).await?;

    let mut block_subscription = subscriber
        .subscribe_new_blocks_at_depth(config.subscription_depth)
        .await
        .context(BlockArchiveSnafu)?;

    trace!("Creating machine driver and blockchain driver");
    let machine_driver =
        MachineDriver::new(config.dapp_deployment.dapp_address);

    let initial_state = InputBoxInitialState {
        input_box_address: Arc::new(
            config.rollups_deployment.input_box_address,
        ),
    };

    trace!("Starting eth-input-reader...");
    loop {
        match block_subscription.next().await {
            Some(Ok(BlockStreamItem::NewBlock(b))) => {
                // Normal operation, react on newest block.
                trace!(
                    "Received block number {} and hash {:?}, parent: {:?}",
                    b.number,
                    b.hash,
                    b.parent_hash
                );

                trace!("Querying rollup state");
                let state =
                    InputBox::get_state_for_block(&initial_state, b, &env)
                        .await
                        .expect("should get state");

                trace!("Reacting to state with `machine_driver`");
                machine_driver
                    .react(&mut context, &state.block, &state.state, &broker)
                    .await
                    .context(BrokerSnafu)?;
            }

            Some(Ok(BlockStreamItem::Reorg(bs))) => {
                error!(
                    "Deep blockchain reorg of {} blocks; new latest has number {:?}, hash {:?}, and parent {:?}",
                    bs.len(),
                    bs.last().map(|b| b.number),
                    bs.last().map(|b| b.hash),
                    bs.last().map(|b| b.parent_hash)
                );
                error!("Bailing...");
                whatever!("deep blockchain reorg");
            }

            Some(Err(e)) => {
                warn!(
                    "Subscription returned error `{}`; waiting for next block...",
                    e
                );
            }

            None => {
                whatever!("subscription closed");
            }
        }
    }
}
