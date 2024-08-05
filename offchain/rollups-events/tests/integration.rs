// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use std::collections::HashMap;

use backoff::ExponentialBackoff;
use redis::aio::ConnectionManager;
use redis::streams::StreamRangeReply;
use redis::{AsyncCommands, Client};
use serde::{Deserialize, Serialize};
use testcontainers::{
    clients::Cli, core::WaitFor, images::generic::GenericImage, Container,
};

use rollups_events::{
    Broker, BrokerConfig, BrokerEndpoint, BrokerError, BrokerMultiStream,
    BrokerStream, RedactedUrl, Url, INITIAL_ID,
};

const STREAM_KEY: &'static str = "test-stream";
const CONSUME_TIMEOUT: usize = 10;

struct TestState<'d> {
    _node: Container<'d, GenericImage>,
    redis_endpoint: RedactedUrl,
    conn: ConnectionManager,
    backoff: ExponentialBackoff,
}

impl TestState<'_> {
    async fn setup(docker: &Cli) -> TestState {
        let image = GenericImage::new("redis", "6.2").with_wait_for(
            WaitFor::message_on_stdout("Ready to accept connections"),
        );
        let node = docker.run(image);
        let port = node.get_host_port_ipv4(6379);
        let redis_endpoint = Url::parse(&format!("redis://127.0.0.1:{}", port))
            .map(RedactedUrl::new)
            .expect("failed to parse Redis Url");
        let backoff = ExponentialBackoff::default();

        let client = Client::open(redis_endpoint.inner().as_str())
            .expect("failed to create client");
        let conn = ConnectionManager::new(client)
            .await
            .expect("failed to create connection");

        TestState {
            _node: node,
            redis_endpoint,
            conn,
            backoff,
        }
    }

    async fn create_broker(&self) -> Broker {
        let config = BrokerConfig {
            redis_endpoint: BrokerEndpoint::Single(self.redis_endpoint.clone()),
            backoff: self.backoff.clone(),
            consume_timeout: CONSUME_TIMEOUT,
        };
        Broker::new(config)
            .await
            .expect("failed to initialize broker")
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
struct MockPayload {
    data: String,
}

struct MockStream {}

impl BrokerStream for MockStream {
    type Payload = MockPayload;

    fn key(&self) -> &str {
        STREAM_KEY
    }
}

#[test_log::test(tokio::test)]
async fn test_it_produces_events() {
    let docker = Cli::default();
    let mut state = TestState::setup(&docker).await;
    let mut broker = state.create_broker().await;
    // Produce events using the Broker struct
    const N: usize = 3;
    let mut ids = vec![];
    for i in 0..N {
        let data = MockPayload {
            data: i.to_string(),
        };
        let id = broker
            .produce(&MockStream {}, data)
            .await
            .expect("failed to produce");
        ids.push(id);
    }
    // Check the events directly in Redis
    let reply: StreamRangeReply = state
        .conn
        .xrange(STREAM_KEY, "-", "+")
        .await
        .expect("failed to read");
    assert_eq!(reply.ids.len(), 3);
    for i in 0..N {
        let expected = format!(r#"{{"data":"{}"}}"#, i);
        assert_eq!(reply.ids[i].id, ids[i]);
        assert_eq!(reply.ids[i].get::<String>("payload").unwrap(), expected);
    }
}

#[test_log::test(tokio::test)]
async fn test_it_peeks_in_stream_with_no_events() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;
    let mut broker = state.create_broker().await;
    let event = broker
        .peek_latest(&MockStream {})
        .await
        .expect("failed to peek");
    assert!(matches!(event, None));
}

#[test_log::test(tokio::test)]
async fn test_it_peeks_in_stream_with_multiple_events() {
    let docker = Cli::default();
    let mut state = TestState::setup(&docker).await;
    // Produce multiple events directly in Redis
    const N: usize = 3;
    for i in 0..N {
        let id = format!("1-{}", i);
        let data = format!(r#"{{"data":"{}"}}"#, i);
        let _: String = state
            .conn
            .xadd(STREAM_KEY, id, &[("payload", data)])
            .await
            .expect("failed to add events");
    }
    // Peek the event using the Broker struct
    let mut broker = state.create_broker().await;
    let event = broker
        .peek_latest(&MockStream {})
        .await
        .expect("failed to peek");
    if let Some(event) = event {
        assert_eq!(&event.id, "1-2");
        assert_eq!(&event.payload.data, "2");
    } else {
        panic!("expected some event");
    }
}

#[test_log::test(tokio::test)]
async fn test_it_fails_to_peek_event_in_invalid_format() {
    let docker = Cli::default();
    let mut state = TestState::setup(&docker).await;
    // Produce event directly in Redis
    let _: String = state
        .conn
        .xadd(STREAM_KEY, "1-0", &[("wrong_field", "0")])
        .await
        .expect("failed to add events");
    // Peek the event using the Broker struct
    let mut broker = state.create_broker().await;
    let err = broker
        .peek_latest(&MockStream {})
        .await
        .expect_err("failed to get error");
    assert!(matches!(err, BrokerError::InvalidEvent));
}

#[test_log::test(tokio::test)]
async fn test_it_fails_to_peek_event_with_invalid_data_encoding() {
    let docker = Cli::default();
    let mut state = TestState::setup(&docker).await;
    // Produce event directly in Redis
    let _: String = state
        .conn
        .xadd(STREAM_KEY, "1-0", &[("payload", "not a json")])
        .await
        .expect("failed to add events");
    // Peek the event using the Broker struct
    let mut broker = state.create_broker().await;
    let err = broker
        .peek_latest(&MockStream {})
        .await
        .expect_err("failed to get error");
    assert!(matches!(err, BrokerError::InvalidPayload { .. }));
}

#[test_log::test(tokio::test)]
async fn test_it_consumes_events() {
    let docker = Cli::default();
    let mut state = TestState::setup(&docker).await;
    // Produce multiple events directly in Redis
    const N: usize = 3;
    for i in 0..N {
        let id = format!("1-{}", i);
        let data = format!(r#"{{"data":"{}"}}"#, i);
        let _: String = state
            .conn
            .xadd(STREAM_KEY, id, &[("payload", data)])
            .await
            .expect("failed to add events");
    }
    // Consume events using the Broker struct
    let mut broker = state.create_broker().await;
    let mut last_id = INITIAL_ID.to_owned();
    for i in 0..N {
        let event = broker
            .consume_blocking(&MockStream {}, &last_id)
            .await
            .expect("failed to consume");
        assert_eq!(event.id, format!("1-{}", i));
        assert_eq!(event.payload.data, i.to_string());
        last_id = event.id;
    }
}

#[test_log::test(tokio::test)]
async fn test_it_blocks_until_event_is_produced() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;
    // Spawn another thread that sends the event after a few ms
    let handler = {
        let mut conn = state.conn.clone();
        tokio::spawn(async move {
            let duration = std::time::Duration::from_millis(10);
            tokio::time::sleep(duration).await;
            let _: String = conn
                .xadd(STREAM_KEY, "1-0", &[("payload", r#"{"data":"0"}"#)])
                .await
                .expect("failed to write event");
        })
    };
    // In the main thread, wait for the expected event
    let mut broker = state.create_broker().await;
    let event = broker
        .consume_blocking(&MockStream {}, "0")
        .await
        .expect("failed to consume event");
    assert_eq!(event.id, "1-0");
    assert_eq!(event.payload.data, "0");
    handler.await.expect("failed to wait handler");
}

#[test_log::test(tokio::test)]
async fn test_it_consumes_events_without_blocking() {
    let docker = Cli::default();
    let mut state = TestState::setup(&docker).await;
    // Produce multiple events directly in Redis
    const N: usize = 3;
    for i in 0..N {
        let id = format!("1-{}", i);
        let data = format!(r#"{{"data":"{}"}}"#, i);
        let _: String = state
            .conn
            .xadd(STREAM_KEY, id, &[("payload", data)])
            .await
            .expect("failed to add events");
    }
    // Consume events using the Broker struct
    let mut broker = state.create_broker().await;
    let mut last_id = INITIAL_ID.to_owned();
    for i in 0..N {
        let event = broker
            .consume_nonblocking(&MockStream {}, &last_id)
            .await
            .expect("failed to consume")
            .expect("expected event, got None");
        assert_eq!(event.id, format!("1-{}", i));
        assert_eq!(event.payload.data, i.to_string());
        last_id = event.id;
    }
}

#[test_log::test(tokio::test)]
async fn test_it_does_not_block_when_consuming_empty_stream() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;
    let mut broker = state.create_broker().await;
    let event = broker
        .consume_nonblocking(&MockStream {}, INITIAL_ID)
        .await
        .expect("failed to peek");
    assert!(matches!(event, None));
}

// ------------------------------------------------------------------------------------------------

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct AnotherMockStream {
    key: String,
    a: u8,
    b: u8,
}

impl AnotherMockStream {
    fn new(a: u8, b: u8) -> Self {
        let key = format!("{{a-{}:b-{}}}:{}", a, b, STREAM_KEY);
        Self { key, a, b }
    }
}

impl BrokerStream for AnotherMockStream {
    type Payload = MockPayload;

    fn key(&self) -> &str {
        &self.key
    }
}

impl BrokerMultiStream for AnotherMockStream {
    fn from_key(key: String) -> Self {
        let re = r"^\{a-([^:]+):b-([^}]+)\}:test-stream$".to_string();
        let re = regex::Regex::new(&re).unwrap();
        let caps = re.captures(&key).unwrap();

        let a = caps
            .get(1)
            .unwrap()
            .as_str()
            .to_string()
            .parse::<u8>()
            .unwrap();
        let b = caps
            .get(2)
            .unwrap()
            .as_str()
            .to_string()
            .parse::<u8>()
            .unwrap();

        Self { key, a, b }
    }
}

#[test_log::test(tokio::test)]
async fn test_it_consumes_from_multiple_streams() {
    let docker = Cli::default();
    let state = TestState::setup(&docker).await;
    let mut broker = state.create_broker().await;

    // Creates the map of streams to last-consumed-ids.
    let mut streams = HashMap::new();
    let initial_id = INITIAL_ID.to_string();
    streams.insert(AnotherMockStream::new(1, 2), initial_id.clone());
    streams.insert(AnotherMockStream::new(3, 4), initial_id.clone());
    streams.insert(AnotherMockStream::new(5, 6), initial_id.clone());

    // Produces N events for each stream using the broker struct.
    const N: usize = 3;
    for stream in streams.keys() {
        for i in 0..N {
            let data = format!("{}{}{}", stream.a, stream.b, i);
            let payload = MockPayload { data };
            let _ = broker
                .produce(stream, payload)
                .await
                .expect("failed to produce events");
        }
    }

    // Consumes all events using the broker struct.
    let mut counters = HashMap::new();
    for _ in 0..streams.len() {
        let streams_and_events = broker
            .consume_blocking_from_multiple_streams(streams.clone())
            .await
            .expect("failed to consume");

        for (stream, event) in streams_and_events {
            let i = counters
                .entry(stream.clone())
                .and_modify(|n| *n += 1)
                .or_insert(0)
                .clone();

            // Asserts that the payload is correct.
            let expected = format!("{}{}{}", stream.a, stream.b, i);
            assert_eq!(expected, event.payload.data);

            // Updates the map of streams with the last consumed id.
            let replaced = streams.insert(stream, event.id);
            // And asserts that the key from the map was indeed overwritten.
            assert!(replaced.is_some());
        }
    }

    // Asserts that N events were consumed from each stream.
    for counter in counters.values() {
        assert_eq!(N - 1, *counter);
    }

    // Gets one of the streams.
    let stream = streams.clone().into_keys().next().unwrap();
    let expected_stream = stream.clone();

    // Produces the final event.
    let data = "final event".to_string();
    let payload = MockPayload { data };
    let _ = broker
        .produce(&stream, payload)
        .await
        .expect("failed to produce the final event");

    // Consumes the final event.
    let mut streams_and_events = broker
        .consume_blocking_from_multiple_streams(streams)
        .await
        .expect("failed to consume the final event");
    assert_eq!(1, streams_and_events.len());
    let (final_stream, _) = streams_and_events.pop().unwrap();

    // Asserts that the event came from the correct stream.
    assert_eq!(expected_stream, final_stream);
}
