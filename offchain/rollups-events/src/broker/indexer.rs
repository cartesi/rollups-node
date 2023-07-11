// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//! This module is an indexer-specific extension for the broker
//!
//! It would be too complex to implement the indexer extension as a generic broker method.
//! Instead, we decided to implement the extension that we need for the indexer as a submodule.
//! This extension should be in this crate because it accesses the Redis interface directly.
//! (All Redis interaction should be hidden in this crate.)
use backoff::future::retry;
use redis::streams::{StreamReadOptions, StreamReadReply};
use redis::AsyncCommands;
use snafu::ResultExt;

use super::ConnectionSnafu;
use crate::{
    Broker, BrokerError, BrokerStream, DAppMetadata, Event, RollupsInput,
    RollupsInputsStream, RollupsOutput, RollupsOutputsStream, INITIAL_ID,
};

#[derive(Debug, Clone, Eq, PartialEq)]
pub enum IndexerEvent {
    Input(Event<RollupsInput>),
    Output(Event<RollupsOutput>),
}

#[derive(Debug)]
pub struct IndexerState {
    inputs_last_id: String,
    outputs_last_id: String,
    inputs_stream: RollupsInputsStream,
    outputs_stream: RollupsOutputsStream,
}

impl IndexerState {
    pub fn new(dapp_metadata: &DAppMetadata) -> Self {
        Self {
            inputs_last_id: INITIAL_ID.to_owned(),
            outputs_last_id: INITIAL_ID.to_owned(),
            inputs_stream: RollupsInputsStream::new(dapp_metadata),
            outputs_stream: RollupsOutputsStream::new(dapp_metadata),
        }
    }
}

impl Broker {
    /// Consume an event from the Input stream and if there is none,
    /// consume from the Output stream. This is a blocking operation.
    /// Return IndexerEvent::Input if present or IndexerEvent::Output otherwise
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn indexer_consume(
        &self,
        state: &mut IndexerState,
    ) -> Result<IndexerEvent, BrokerError> {
        let input_stream_key = state.inputs_stream.key();
        let output_stream_key = state.outputs_stream.key();
        let mut reply = retry(self.backoff.clone(), || async {
            let stream_keys = [&input_stream_key, &output_stream_key];
            let last_consumed_ids =
                [&state.inputs_last_id, &state.outputs_last_id];
            tracing::trace!(
                ?stream_keys,
                ?last_consumed_ids,
                "consuming event"
            );
            let opts = StreamReadOptions::default()
                .count(1)
                .block(self.consume_timeout);
            let reply: StreamReadReply = self
                .connection
                .clone()
                .xread_options(&stream_keys, &last_consumed_ids, &opts)
                .await?;
            Ok(reply)
        })
        .await
        .context(ConnectionSnafu)?;

        let input_stream_id = reply
            .keys
            .iter_mut()
            .find(|stream| stream.key == input_stream_key)
            .and_then(|stream| stream.ids.pop());
        if let Some(stream_id) = input_stream_id {
            tracing::trace!("found input event; parsing it");
            let event: Event<RollupsInput> = stream_id.try_into()?;
            state.inputs_last_id = event.id.clone();
            return Ok(IndexerEvent::Input(event));
        }

        let output_stream_id = reply
            .keys
            .iter_mut()
            .find(|stream| stream.key == output_stream_key)
            .and_then(|stream| stream.ids.pop());
        if let Some(stream_id) = output_stream_id {
            tracing::trace!("found output event; parsing it");
            let event: Event<RollupsOutput> = stream_id.try_into()?;
            state.outputs_last_id = event.id.clone();
            return Ok(IndexerEvent::Output(event));
        }

        tracing::trace!("indexer consume timed out");
        Err(BrokerError::ConsumeTimeout)
    }
}
