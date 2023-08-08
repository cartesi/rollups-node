// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use http_server::{CounterRef, FamilyRef, Registry};
use rollups_events::DAppMetadata;

const METRICS_PREFIX: &str = "cartesi_rollups_eth_input_reader";

fn prefixed_metrics(name: &str) -> String {
    format!("{}_{}", METRICS_PREFIX, name)
}

#[derive(Debug, Clone, Default)]
pub struct EthInputReaderMetrics {
    pub advance_inputs_sent: FamilyRef<DAppMetadata, CounterRef>,
    pub finish_epochs_sent: FamilyRef<DAppMetadata, CounterRef>,
}

impl From<EthInputReaderMetrics> for Registry {
    fn from(metrics: EthInputReaderMetrics) -> Self {
        let mut registry = Registry::default();
        registry.register(
            prefixed_metrics("advance_inputs_sent"),
            "Counts the number of <advance_input>s sent",
            metrics.advance_inputs_sent,
        );
        registry.register(
            prefixed_metrics("finish_epochs_sent"),
            "Counts the number of <finish_epoch>s sent",
            metrics.finish_epochs_sent,
        );
        registry
    }
}
