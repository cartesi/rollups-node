// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use snafu::Snafu;

use crate::{broker, runner, server_manager};

#[derive(Debug, Snafu)]
#[snafu(visibility(pub(crate)))]
pub enum AdvanceRunnerError {
    #[snafu(display("health check error"))]
    HealthCheckError {
        source: http_health_check::HealthCheckError,
    },

    #[snafu(display("server manager error"))]
    ServerManagerError {
        source: server_manager::ServerManagerError,
    },

    #[snafu(display("broker error"))]
    BrokerError { source: broker::BrokerFacadeError },

    #[snafu(display("runner error"))]
    RunnerError { source: runner::RunnerError },
}
