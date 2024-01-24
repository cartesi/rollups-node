// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use eth_state_client_lib::StateServer;
use eth_state_fold_types::{Block, BlockStreamItem};
use rollups_events::DAppMetadata;
use std::sync::Arc;
use tokio_stream::StreamExt;
use tracing::{error, instrument, trace, warn};
use types::foldables::{InputBox, InputBoxInitialState};

use crate::{
    config::DispatcherConfig,
    drivers::{machine::MachineDriver, Context},
    error::{BrokerSnafu, DispatcherError, StateServerSnafu},
    machine::{rollups_broker::BrokerFacade, BrokerSend},
    metrics::DispatcherMetrics,
    setup::{create_block_subscription, create_context, create_state_server},
};

use snafu::{whatever, ResultExt};

#[instrument(level = "trace", skip_all)]
pub async fn start(
    config: DispatcherConfig,
    metrics: DispatcherMetrics,
) -> Result<(), DispatcherError> {
    trace!("Setting up dispatcher");

    let dapp_metadata = DAppMetadata {
        chain_id: config.chain_id,
        dapp_address: config.blockchain_config.dapp_address.clone(),
    };

    trace!("Creating state-server connection");
    let state_server = create_state_server(&config.sc_config).await?;

    trace!("Starting block subscription with confirmations");
    let mut block_subscription = create_block_subscription(
        &state_server,
        config.sc_config.default_confirmations,
    )
    .await?;

    trace!("Creating broker connection");
    let broker =
        BrokerFacade::new(config.broker_config.clone(), dapp_metadata.clone())
            .await
            .context(BrokerSnafu)?;

    trace!("Creating machine driver and blockchain driver");
    let mut machine_driver = MachineDriver::new(
        config
            .blockchain_config
            .dapp_address
            .clone()
            .into_inner()
            .into(),
    );

    let initial_state = InputBoxInitialState {
        dapp_address: Arc::new(
            config
                .blockchain_config
                .dapp_address
                .clone()
                .into_inner()
                .into(),
        ),
        input_box_address: Arc::new(
            config
                .blockchain_config
                .input_box_address
                .clone()
                .into_inner()
                .into(),
        ),
    };

    trace!("Creating context");
    let mut context = create_context(
        &(config.clone()),
        &state_server,
        &broker,
        dapp_metadata,
        metrics,
    )
    .await?;

    trace!("Starting dispatcher...");
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
                process_block(
                    &b,
                    &state_server,
                    &initial_state,
                    &mut context,
                    &mut machine_driver,
                    &broker,
                )
                .await?
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

#[instrument(level = "trace", skip_all)]
#[allow(clippy::too_many_arguments)]
async fn process_block(
    block: &Block,

    state_server: &impl StateServer<
        InitialState = InputBoxInitialState,
        State = InputBox,
    >,
    initial_state: &InputBoxInitialState,

    context: &mut Context,
    machine_driver: &mut MachineDriver,

    broker: &impl BrokerSend,
) -> Result<(), DispatcherError> {
    trace!("Querying rollup state");
    let state = state_server
        .query_state(initial_state, block.hash)
        .await
        .context(StateServerSnafu)?;

    // Drive machine
    trace!("Reacting to state with `machine_driver`");
    machine_driver
        .react(context, &state.block, &state.state, broker)
        .await
        .context(BrokerSnafu)?;

    Ok(())
}
