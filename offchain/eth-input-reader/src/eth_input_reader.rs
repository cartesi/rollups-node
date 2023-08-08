// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use eth_state_client_lib::StateServer;
use eth_state_fold_types::{Block, BlockStreamItem};
use rollups_events::{Address, DAppMetadata};
use tokio_stream::StreamExt;
use tracing::{error, info, instrument, trace, warn};
use types::foldables::authority::rollups::{RollupsInitialState, RollupsState};

use crate::{
    config::EthInputReaderConfig,
    error::{BrokerSnafu, EthInputReaderError, StateServerSnafu},
    machine::{
        driver::MachineDriver, rollups_broker::BrokerFacade, BrokerReceive,
        BrokerSend, Context,
    },
    metrics::EthInputReaderMetrics,
    setup::{create_block_subscription, create_context, create_state_server},
};

use snafu::{whatever, ResultExt};

#[instrument(level = "trace", skip_all)]
pub async fn start(
    config: EthInputReaderConfig,
    metrics: EthInputReaderMetrics,
) -> Result<(), EthInputReaderError> {
    info!("Setting up eth-input-reader with config: {:?}", config);

    let dapp_metadata = DAppMetadata {
        chain_id: config.tx_config.chain_id,
        dapp_address: Address::new(config.dapp_deployment.dapp_address.into()),
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

    trace!("Creating context");
    let mut context =
        create_context(&config, &state_server, &broker, dapp_metadata, metrics)
            .await?;

    trace!("Creating machine driver and blockchain driver");
    let mut machine_driver =
        MachineDriver::new(config.dapp_deployment.dapp_address);

    let initial_state = RollupsInitialState {
        history_address: config.rollups_deployment.history_address,
        input_box_address: config.rollups_deployment.input_box_address,
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
        InitialState = RollupsInitialState,
        State = RollupsState,
    >,
    initial_state: &RollupsInitialState,

    context: &mut Context,
    machine_driver: &mut MachineDriver,

    broker: &(impl BrokerSend + BrokerReceive),
) -> Result<(), EthInputReaderError> {
    trace!("Querying rollup state");
    let state = state_server
        .query_state(initial_state, block.hash)
        .await
        .context(StateServerSnafu)?;

    // Drive machine
    trace!("Reacting to state with `machine_driver`");
    machine_driver
        .react(context, &state.block, &state.state.input_box, broker)
        .await
        .context(BrokerSnafu)?;

    Ok(())
}
