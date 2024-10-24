// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use std::collections::HashMap;

use backoff::ExponentialBackoff;
use rollups_events::{
    Address, Broker, BrokerConfig, BrokerEndpoint, DAppMetadata, Event,
    RedactedUrl, RollupsClaim, RollupsClaimsStream, RollupsData, RollupsInput,
    RollupsInputsStream, RollupsOutput, RollupsOutputsStream, Url,
    ADDRESS_SIZE, INITIAL_ID,
};
use testcontainers::{
    clients::Cli, core::WaitFor, images::generic::GenericImage, Container,
};
use tokio::sync::Mutex;

const CHAIN_ID: u64 = 0;
const DAPP_ADDRESS: Address = Address::new([0xfa; ADDRESS_SIZE]);
const CONSUME_TIMEOUT: usize = 10_000; // ms

async fn start_redis(
    docker: &Cli,
) -> (Container<GenericImage>, BrokerEndpoint, Mutex<Broker>) {
    tracing::trace!("starting redis docker container");
    let image = GenericImage::new("redis", "6.2").with_wait_for(
        WaitFor::message_on_stdout("Ready to accept connections"),
    );
    let node = docker.run(image);
    let port = node.get_host_port_ipv4(6379);
    let endpoint = BrokerEndpoint::Single(
        Url::parse(&format!("redis://127.0.0.1:{}", port))
            .map(RedactedUrl::new)
            .expect("failed to parse Redis Url"),
    );

    let backoff = ExponentialBackoff::default();
    let config = BrokerConfig {
        redis_endpoint: endpoint.clone(),
        consume_timeout: CONSUME_TIMEOUT,
        backoff,
    };

    tracing::trace!(?endpoint, "connecting to redis with rollups_events crate");
    let client = Mutex::new(
        Broker::new(config)
            .await
            .expect("failed to connect to broker"),
    );

    (node, endpoint, client)
}

// ------------------------------------------------------------------------------------------------

pub struct BrokerFixture<'d> {
    _node: Container<'d, GenericImage>,
    client: Mutex<Broker>,
    inputs_stream: RollupsInputsStream,
    claims_stream: RollupsClaimsStream,
    outputs_stream: RollupsOutputsStream,
    redis_endpoint: BrokerEndpoint,
    chain_id: u64,
    dapp_address: Address,
}

impl BrokerFixture<'_> {
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn setup(docker: &Cli) -> BrokerFixture<'_> {
        tracing::info!("setting up redis fixture");

        let (redis_node, redis_endpoint, redis_client) =
            start_redis(&docker).await;

        let metadata = DAppMetadata {
            chain_id: CHAIN_ID,
            dapp_address: DAPP_ADDRESS.clone(),
        };

        BrokerFixture {
            _node: redis_node,
            client: redis_client,
            inputs_stream: RollupsInputsStream::new(&metadata),
            claims_stream: RollupsClaimsStream::new(&metadata),
            outputs_stream: RollupsOutputsStream::new(&metadata),
            redis_endpoint,
            chain_id: CHAIN_ID,
            dapp_address: DAPP_ADDRESS,
        }
    }

    pub fn redis_endpoint(&self) -> &BrokerEndpoint {
        &self.redis_endpoint
    }

    pub fn chain_id(&self) -> u64 {
        self.chain_id
    }

    pub fn dapp_address(&self) -> &Address {
        &self.dapp_address
    }

    pub fn dapp_metadata(&self) -> DAppMetadata {
        DAppMetadata {
            chain_id: self.chain_id,
            dapp_address: self.dapp_address.clone(),
        }
    }

    /// Obtain the latest event from the rollups inputs stream
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn get_latest_input_event(&self) -> Option<Event<RollupsInput>> {
        tracing::trace!("getting latest input event");
        self.client
            .lock()
            .await
            .peek_latest(&self.inputs_stream)
            .await
            .expect("failed to get latest input event")
    }

    /// Produce the input event given the data
    /// Return the produced event id
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn produce_input_event(&self, data: RollupsData) -> String {
        tracing::trace!(?data, "producing rollups-input event");
        let last_event = self.get_latest_input_event().await;
        let epoch_index = match last_event.as_ref() {
            Some(event) => match event.payload.data {
                RollupsData::AdvanceStateInput { .. } => {
                    event.payload.epoch_index
                }
                RollupsData::FinishEpoch {} => event.payload.epoch_index + 1,
            },
            None => 0,
        };
        let previous_inputs_sent_count = match last_event.as_ref() {
            Some(event) => event.payload.inputs_sent_count,
            None => 0,
        };
        let inputs_sent_count = match data {
            RollupsData::AdvanceStateInput { .. } => {
                previous_inputs_sent_count + 1
            }
            RollupsData::FinishEpoch {} => previous_inputs_sent_count,
        };
        let parent_id = match last_event {
            Some(event) => event.id,
            None => INITIAL_ID.to_owned(),
        };
        let input = RollupsInput {
            parent_id,
            epoch_index,
            inputs_sent_count,
            data,
        };
        self.produce_raw_input_event(input).await
    }

    /// Produce the input event given the input
    /// This may produce inconsistent inputs
    /// Return the produced event id
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn produce_raw_input_event(&self, input: RollupsInput) -> String {
        tracing::trace!(?input, "producing rollups-input raw event");
        self.client
            .lock()
            .await
            .produce(&self.inputs_stream, input)
            .await
            .expect("failed to produce event")
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

    /// Produce an output event
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn produce_output(&self, output: RollupsOutput) {
        tracing::trace!(?output, "producing rollups-outputs event");
        self.client
            .lock()
            .await
            .produce(&self.outputs_stream, output)
            .await
            .expect("failed to produce output");
    }
}

// ------------------------------------------------------------------------------------------------

pub struct ClaimerMultidappBrokerFixture<'d> {
    _node: Container<'d, GenericImage>,
    client: Mutex<Broker>,
    redis_endpoint: BrokerEndpoint,
    claims_streams: HashMap<Address, RollupsClaimsStream>,
}

impl ClaimerMultidappBrokerFixture<'_> {
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn setup(
        docker: &Cli,
        dapps: Vec<(u64, Address)>,
    ) -> ClaimerMultidappBrokerFixture<'_> {
        let (redis_node, redis_endpoint, redis_client) =
            start_redis(&docker).await;

        let claims_streams = dapps
            .into_iter()
            .map(|(chain_id, dapp_address)| {
                let dapp_metadata = DAppMetadata {
                    chain_id,
                    dapp_address: dapp_address.clone(),
                };
                let stream = RollupsClaimsStream::new(&dapp_metadata);
                (dapp_address, stream)
            })
            .collect::<Vec<_>>();
        let claims_streams = HashMap::from_iter(claims_streams);

        ClaimerMultidappBrokerFixture {
            _node: redis_node,
            client: redis_client,
            redis_endpoint,
            claims_streams,
        }
    }

    pub fn redis_endpoint(&self) -> &BrokerEndpoint {
        &self.redis_endpoint
    }

    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn dapps_set(&self, dapps: Vec<Address>) {
        self.client.lock().await.dapps_set(dapps).await
    }

    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn dapps_add(&self, dapp: String) {
        self.client.lock().await.dapps_add(dapp).await
    }

    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn dapps_members(&self) -> Vec<String> {
        self.client.lock().await.dapps_members().await
    }

    // Different from the default function,
    // this one requires `rollups_claim.dapp_address` to be set,
    // and to match one of the addresses from the streams.
    #[tracing::instrument(level = "trace", skip_all)]
    pub async fn produce_rollups_claim(&self, rollups_claim: RollupsClaim) {
        tracing::trace!(?rollups_claim.epoch_hash, "producing rollups-claim event");

        let stream = self
            .claims_streams
            .get(&rollups_claim.dapp_address)
            .unwrap()
            .clone();

        {
            let last_claim = self
                .client
                .lock()
                .await
                .peek_latest(&stream)
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
            .produce(&stream, rollups_claim)
            .await
            .expect("failed to produce claim");
    }
}
