// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use backoff::{future::retry, ExponentialBackoff, ExponentialBackoffBuilder};
use clap::Parser;
use redis::aio::{ConnectionLike, ConnectionManager};
use redis::cluster::ClusterClient;
use redis::cluster_async::ClusterConnection;
use redis::streams::{
    StreamId, StreamRangeReply, StreamReadOptions, StreamReadReply,
};
use redis::{
    AsyncCommands, Client, Cmd, Pipeline, RedisError, RedisFuture, Value,
};
use serde::{de::DeserializeOwned, Serialize};
use snafu::{ResultExt, Snafu};
use std::fmt;
use std::time::Duration;

pub use redacted::{RedactedUrl, Url};

pub mod indexer;

pub const INITIAL_ID: &str = "0";

/// The `BrokerConnection` enum implements the `ConnectionLike` trait
/// to satisfy the `AsyncCommands` trait bounds.
/// As `AsyncCommands` requires its implementors to be `Sized`, we couldn't
/// use a trait object instead.
#[derive(Clone)]
enum BrokerConnection {
    ConnectionManager(ConnectionManager),
    ClusterConnection(ClusterConnection),
}

impl ConnectionLike for BrokerConnection {
    fn req_packed_command<'a>(
        &'a mut self,
        cmd: &'a Cmd,
    ) -> RedisFuture<'a, Value> {
        match self {
            Self::ConnectionManager(connection) => {
                connection.req_packed_command(cmd)
            }
            Self::ClusterConnection(connection) => {
                connection.req_packed_command(cmd)
            }
        }
    }

    fn req_packed_commands<'a>(
        &'a mut self,
        cmd: &'a Pipeline,
        offset: usize,
        count: usize,
    ) -> RedisFuture<'a, Vec<Value>> {
        match self {
            Self::ConnectionManager(connection) => {
                connection.req_packed_commands(cmd, offset, count)
            }
            Self::ClusterConnection(connection) => {
                connection.req_packed_commands(cmd, offset, count)
            }
        }
    }

    fn get_db(&self) -> i64 {
        match self {
            Self::ConnectionManager(connection) => connection.get_db(),
            Self::ClusterConnection(connection) => connection.get_db(),
        }
    }
}

/// Client that connects to the broker
#[derive(Clone)]
pub struct Broker {
    connection: BrokerConnection,
    backoff: ExponentialBackoff,
    consume_timeout: usize,
}

impl Broker {
    /// Create a new client
    /// The broker_address should be in the format redis://host:port/db.
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn new(config: BrokerConfig) -> Result<Self, BrokerError> {
        tracing::trace!(?config, "connecting to broker");

        let connection = retry(config.backoff.clone(), || async {
            match config.redis_endpoint.clone() {
                BrokerEndpoint::Single(endpoint) => {
                    tracing::trace!("creating Redis Client");
                    let client = Client::open(endpoint.inner().as_str())?;

                    tracing::trace!("creating Redis ConnectionManager");
                    let connection = ConnectionManager::new(client).await?;

                    Ok(BrokerConnection::ConnectionManager(connection))
                }
                BrokerEndpoint::Cluster(endpoints) => {
                    tracing::trace!("creating Redis Cluster Client");
                    let client = ClusterClient::new(
                        endpoints
                            .iter()
                            .map(|endpoint| endpoint.inner().as_str())
                            .collect(),
                    )?;
                    tracing::trace!("connecting to Redis Cluster");
                    let connection = client.get_async_connection().await?;
                    Ok(BrokerConnection::ClusterConnection(connection))
                }
            }
        })
        .await
        .context(ConnectionSnafu)?;

        tracing::trace!("returning successful connection");
        Ok(Self {
            connection,
            backoff: config.backoff,
            consume_timeout: config.consume_timeout,
        })
    }

    /// Produce an event and return its id
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn produce<S: BrokerStream>(
        &mut self,
        stream: &S,
        payload: S::Payload,
    ) -> Result<String, BrokerError> {
        tracing::trace!("converting payload to JSON string");
        let payload =
            serde_json::to_string(&payload).context(InvalidPayloadSnafu)?;

        let event_id = retry(self.backoff.clone(), || async {
            tracing::trace!(
                stream_key = stream.key(),
                payload,
                "producing event"
            );
            let event_id = self
                .connection
                .clone()
                .xadd(stream.key(), "*", &[("payload", &payload)])
                .await?;

            Ok(event_id)
        })
        .await
        .context(ConnectionSnafu)?;

        tracing::trace!(event_id, "returning event id");
        Ok(event_id)
    }

    /// Peek at the end of the stream
    /// This function doesn't block; if there is no event in the stream it returns None.
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn peek_latest<S: BrokerStream>(
        &mut self,
        stream: &S,
    ) -> Result<Option<Event<S::Payload>>, BrokerError> {
        let mut reply = retry(self.backoff.clone(), || async {
            tracing::trace!(stream_key = stream.key(), "peeking at the stream");
            let reply: StreamRangeReply = self
                .connection
                .clone()
                .xrevrange_count(stream.key(), "+", "-", 1)
                .await?;

            Ok(reply)
        })
        .await
        .context(ConnectionSnafu)?;

        if let Some(event) = reply.ids.pop() {
            tracing::trace!("parsing received event");
            Some(event.try_into()).transpose()
        } else {
            tracing::trace!("stream is empty");
            Ok(None)
        }
    }

    /// Consume the next event in stream
    /// This function blocks until a new event is available.
    /// To consume the first event in the stream, last_consumed_id should be INITIAL_ID.
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn consume_blocking<S: BrokerStream>(
        &mut self,
        stream: &S,
        last_consumed_id: &str,
    ) -> Result<Event<S::Payload>, BrokerError> {
        let mut reply = retry(self.backoff.clone(), || async {
            tracing::trace!(
                stream_key = stream.key(),
                last_consumed_id,
                "consuming event"
            );
            let opts = StreamReadOptions::default()
                .count(1)
                .block(self.consume_timeout);
            let reply: StreamReadReply = self
                .connection
                .clone()
                .xread_options(&[stream.key()], &[last_consumed_id], &opts)
                .await?;

            Ok(reply)
        })
        .await
        .context(ConnectionSnafu)?;

        tracing::trace!("checking for timeout");
        let mut events = reply.keys.pop().ok_or(BrokerError::ConsumeTimeout)?;

        tracing::trace!("checking if event was received");
        let event = events.ids.pop().ok_or(BrokerError::FailedToConsume)?;

        tracing::trace!("parsing received event");
        event.try_into()
    }

    /// Consume the next event in stream without blocking
    /// This function returns None if there are no more remaining events.
    /// To consume the first event in the stream, last_consumed_id should be INITIAL_ID.
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn consume_nonblocking<S: BrokerStream>(
        &mut self,
        stream: &S,
        last_consumed_id: &str,
    ) -> Result<Option<Event<S::Payload>>, BrokerError> {
        let mut reply = retry(self.backoff.clone(), || async {
            tracing::trace!(
                stream_key = stream.key(),
                last_consumed_id,
                "consuming event (non-blocking)"
            );
            let opts = StreamReadOptions::default().count(1);
            let reply: StreamReadReply = self
                .connection
                .clone()
                .xread_options(&[stream.key()], &[last_consumed_id], &opts)
                .await?;

            Ok(reply)
        })
        .await
        .context(ConnectionSnafu)?;

        tracing::trace!("checking if event was received");
        if let Some(mut events) = reply.keys.pop() {
            let event = events.ids.pop().ok_or(BrokerError::FailedToConsume)?;
            tracing::trace!("parsing received event");
            Some(event.try_into()).transpose()
        } else {
            tracing::trace!("stream is empty");
            Ok(None)
        }
    }
}

/// Custom implementation of Debug because ConnectionManager doesn't implement debug
impl fmt::Debug for Broker {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("Broker")
            .field("consume_timeout", &self.consume_timeout)
            .finish()
    }
}

/// Trait that defines the type of a stream
pub trait BrokerStream {
    type Payload: Serialize + DeserializeOwned + Clone + Eq + PartialEq;
    fn key(&self) -> &str;
}

/// Event that goes through the broker
#[derive(Debug, Clone, Eq, PartialEq)]
pub struct Event<P: Serialize + DeserializeOwned + Clone + Eq + PartialEq> {
    pub id: String,
    pub payload: P,
}

impl<P: Serialize + DeserializeOwned + Clone + Eq + PartialEq> TryFrom<StreamId>
    for Event<P>
{
    type Error = BrokerError;

    #[tracing::instrument(level = "trace", skip_all)]
    fn try_from(stream_id: StreamId) -> Result<Event<P>, BrokerError> {
        tracing::trace!("getting event payload");
        let payload = stream_id
            .get::<String>("payload")
            .ok_or(BrokerError::InvalidEvent)?;
        let id = stream_id.id;

        tracing::trace!(id, payload, "received event");

        tracing::trace!("parsing JSON payload");
        let payload =
            serde_json::from_str(&payload).context(InvalidPayloadSnafu)?;

        tracing::trace!("returning event");
        Ok(Event { id, payload })
    }
}

#[derive(Debug, Snafu)]
pub enum BrokerError {
    #[snafu(display("error connecting to Redis"))]
    ConnectionError { source: RedisError },

    #[snafu(display("failed to consume event"))]
    FailedToConsume,

    #[snafu(display("timed out when consuming event"))]
    ConsumeTimeout,

    #[snafu(display("event in invalid format"))]
    InvalidEvent,

    #[snafu(display("error parsing event payload"))]
    InvalidPayload { source: serde_json::Error },
}

#[derive(Debug, Clone, Parser)]
#[command(name = "broker")]
pub struct BrokerCLIConfig {
    /// Redis address
    #[arg(long, env, default_value = "redis://127.0.0.1:6379")]
    redis_endpoint: String,

    /// Address list of Redis cluster nodes, defined by a single string
    /// separated by commas. If present, it supersedes `redis_endpoint`.
    /// A single endpoint can be enough as the client will discover
    /// other nodes automatically
    #[arg(long, env, num_args = 1.., value_delimiter = ',')]
    redis_cluster_endpoints: Option<Vec<String>>,

    /// Timeout when consuming input events (in millis)
    #[arg(long, env, default_value = "5000")]
    broker_consume_timeout: usize,

    /// The max elapsed time for backoff in ms
    #[arg(long, env, default_value = "120000")]
    broker_backoff_max_elapsed_duration: u64,
}

#[derive(Debug, Clone)]
pub enum BrokerEndpoint {
    Single(RedactedUrl),
    Cluster(Vec<RedactedUrl>),
}

#[derive(Debug, Clone)]
pub struct BrokerConfig {
    pub redis_endpoint: BrokerEndpoint,
    pub consume_timeout: usize,
    pub backoff: ExponentialBackoff,
}

impl From<BrokerCLIConfig> for BrokerConfig {
    fn from(cli_config: BrokerCLIConfig) -> BrokerConfig {
        let max_elapsed_time = Duration::from_millis(
            cli_config.broker_backoff_max_elapsed_duration,
        );
        let backoff = ExponentialBackoffBuilder::new()
            .with_max_elapsed_time(Some(max_elapsed_time))
            .build();
        let redis_endpoint =
            if let Some(endpoints) = cli_config.redis_cluster_endpoints {
                let urls = endpoints
                    .iter()
                    .map(|endpoint| {
                        RedactedUrl::new(
                            Url::parse(endpoint)
                                .expect("failed to parse Redis URL"),
                        )
                    })
                    .collect();
                BrokerEndpoint::Cluster(urls)
            } else {
                let url = Url::parse(&cli_config.redis_endpoint)
                    .map(RedactedUrl::new)
                    .expect("failed to parse Redis URL");
                BrokerEndpoint::Single(url)
            };
        BrokerConfig {
            redis_endpoint,
            consume_timeout: cli_config.broker_consume_timeout,
            backoff,
        }
    }
}
