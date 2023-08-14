// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use std::time::Duration;

use backoff::ExponentialBackoffBuilder;
use rollups_events::{
    BrokerConfig, BrokerEndpoint, BrokerError, DAppMetadata, RedactedUrl,
    RollupsClaim, Url,
};
use snafu::Snafu;
use test_fixtures::BrokerFixture;
use testcontainers::clients::Cli;

use crate::listener::DefaultBrokerListener;

#[derive(Clone, Debug, Snafu)]
pub enum MockError {
    EndError,
    InternalError,
    MockError,
}

pub async fn setup_broker(
    docker: &Cli,
    should_fail: bool,
) -> Result<(BrokerFixture, DefaultBrokerListener), BrokerError> {
    let fixture = BrokerFixture::setup(docker).await;

    let redis_endpoint = if should_fail {
        BrokerEndpoint::Single(RedactedUrl::new(
            Url::parse("https://invalid.com").unwrap(),
        ))
    } else {
        fixture.redis_endpoint().clone()
    };

    let config = BrokerConfig {
        redis_endpoint,
        consume_timeout: 300000,
        backoff: ExponentialBackoffBuilder::new()
            .with_initial_interval(Duration::from_millis(1000))
            .with_max_elapsed_time(Some(Duration::from_millis(3000)))
            .build(),
    };
    let metadata = DAppMetadata {
        chain_id: fixture.chain_id(),
        dapp_address: fixture.dapp_address().clone(),
    };
    let broker = DefaultBrokerListener::new(config, metadata).await?;
    Ok((fixture, broker))
}

pub async fn produce_rollups_claims(
    fixture: &BrokerFixture<'_>,
    n: usize,
    epoch_index_start: usize,
) -> Vec<RollupsClaim> {
    let mut rollups_claims = Vec::new();
    for i in 0..n {
        let mut rollups_claim = RollupsClaim::default();
        rollups_claim.epoch_index = (i + epoch_index_start) as u64;
        fixture.produce_rollups_claim(rollups_claim.clone()).await;
        rollups_claims.push(rollups_claim);
    }
    rollups_claims
}

/// The last claim should trigger an `EndError` error.
pub async fn produce_last_claim(
    fixture: &BrokerFixture<'_>,
    epoch_index: usize,
) -> Vec<RollupsClaim> {
    produce_rollups_claims(fixture, 1, epoch_index).await
}
