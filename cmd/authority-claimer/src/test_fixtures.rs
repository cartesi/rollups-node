// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::{
    redacted::{RedactedUrl, Url},
    rollups_events::{
        broker::BrokerEndpoint, common::ADDRESS_SIZE, Address, Broker,
        BrokerConfig, DAppMetadata, RollupsClaim, RollupsClaimsStream,
        INITIAL_ID,
    },
};
use backoff::ExponentialBackoff;
use testcontainers::{
    clients::Cli, core::WaitFor, images::generic::GenericImage, Container,
};
use tokio::sync::Mutex;

const CHAIN_ID: u64 = 0;
const DAPP_ADDRESS: Address = Address::new([0xfa; ADDRESS_SIZE]);
const CONSUME_TIMEOUT: usize = 10_000; // ms

pub struct BrokerFixture<'d> {
    _node: Container<'d, GenericImage>,
    client: Mutex<Broker>,
    claims_stream: RollupsClaimsStream,
    redis_endpoint: BrokerEndpoint,
    chain_id: u64,
}

impl BrokerFixture<'_> {
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn setup(docker: &Cli) -> BrokerFixture<'_> {
        tracing::info!("setting up redis fixture");

        tracing::trace!("starting redis docker container");
        let image = GenericImage::new("redis", "6.2").with_wait_for(
            WaitFor::message_on_stdout("Ready to accept connections"),
        );
        let node = docker.run(image);
        let port = node.get_host_port_ipv4(6379);
        let redis_endpoint = BrokerEndpoint::Single(
            Url::parse(&format!("redis://127.0.0.1:{}", port))
                .map(RedactedUrl::new)
                .expect("failed to parse Redis Url"),
        );
        let chain_id = CHAIN_ID;
        let backoff = ExponentialBackoff::default();
        let metadata = DAppMetadata {
            chain_id,
            dapp_address: DAPP_ADDRESS.clone(),
        };
        let claims_stream = RollupsClaimsStream::new(metadata.chain_id);
        let config = BrokerConfig {
            redis_endpoint: redis_endpoint.clone(),
            consume_timeout: CONSUME_TIMEOUT,
            backoff,
        };

        tracing::trace!(
            ?redis_endpoint,
            "connecting to redis with rollups_events crate"
        );
        let client = Mutex::new(
            Broker::new(config)
                .await
                .expect("failed to connect to broker"),
        );
        BrokerFixture {
            _node: node,
            client,
            claims_stream,
            redis_endpoint,
            chain_id,
        }
    }

    pub fn redis_endpoint(&self) -> &BrokerEndpoint {
        &self.redis_endpoint
    }

    pub fn chain_id(&self) -> u64 {
        self.chain_id
    }

    /// Produce the claim given the hash
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn produce_rollups_claim(&self, rollups_claim: RollupsClaim) {
        tracing::trace!(?rollups_claim.epoch_hash, "producing rollups-claim event");
        {
            let last_claim = self
                .client
                .lock()
                .await
                .peek_latest(&self.claims_stream)
                .await
                .expect("failed to get latest claim");
            let epoch_index = match last_claim {
                Some(event) => event.payload.epoch_index + 1,
                None => 0,
            };
            assert_eq!(
                rollups_claim.epoch_index, epoch_index,
                "invalid epoch index",
            );
        }
        self.client
            .lock()
            .await
            .produce(&self.claims_stream, rollups_claim)
            .await
            .expect("failed to produce claim");
    }

    /// Obtain all produced claims
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn consume_all_claims(&self) -> Vec<RollupsClaim> {
        tracing::trace!("consuming all rollups-claims events");
        let mut claims = vec![];
        let mut last_id = INITIAL_ID.to_owned();
        while let Some(event) = self
            .client
            .lock()
            .await
            .consume_nonblocking(&self.claims_stream, &last_id)
            .await
            .expect("failed to consume claim")
        {
            claims.push(event.payload);
            last_id = event.id;
        }
        claims
    }

    /// Obtain the first n produced claims
    /// Panic in case of timeout
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn consume_n_claims(&self, n: usize) -> Vec<RollupsClaim> {
        tracing::trace!(n, "consuming n rollups-claims events");
        let mut claims = vec![];
        let mut last_id = INITIAL_ID.to_owned();
        for _ in 0..n {
            let event = self
                .client
                .lock()
                .await
                .consume_blocking(&self.claims_stream, &last_id)
                .await
                .expect("failed to consume claim");
            claims.push(event.payload);
            last_id = event.id
        }
        claims
    }
}
