// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use http_server::{CounterRef, FamilyRef, Registry};
use rollups_events::DAppMetadata;

const METRICS_PREFIX: &str = "cartesi_rollups_authority_claimer";

fn prefixed_metrics(name: &str) -> String {
    format!("{}_{}", METRICS_PREFIX, name)
}

#[derive(Debug, Clone, Default)]
pub struct AuthorityClaimerMetrics {
    pub claims_sent: FamilyRef<DAppMetadata, CounterRef>,
}

impl AuthorityClaimerMetrics {
    pub fn new() -> Self {
        Self::default()
    }
}

impl From<AuthorityClaimerMetrics> for Registry {
    fn from(metrics: AuthorityClaimerMetrics) -> Self {
        let mut registry = Registry::default();
        registry.register(
            prefixed_metrics("claims_sent"),
            "Counts the number of claims sent",
            metrics.claims_sent,
        );
        registry
    }
}
