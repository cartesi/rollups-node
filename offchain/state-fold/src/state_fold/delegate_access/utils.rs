// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::error::*;

use ethers::core::types::Log;
use ethers::providers::Middleware;

pub fn sort_logs<M: Middleware>(logs: &mut Vec<Log>) -> Result<(), M> {
    for log in logs.iter() {
        if !(log.block_number.is_some() && log.log_index.is_some()) {
            return LogUnavailableSnafu {}.fail();
        }
    }

    logs.sort_by(|a, b| {
        let c = a.block_number.unwrap().cmp(&b.block_number.unwrap());
        if let std::cmp::Ordering::Equal = c {
            a.log_index.unwrap().cmp(&b.log_index.unwrap())
        } else {
            c
        }
    });

    Ok(())
}
