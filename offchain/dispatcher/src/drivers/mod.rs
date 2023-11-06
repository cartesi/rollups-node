// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

pub mod context;
pub mod machine;

pub use context::Context;
pub use machine::MachineDriver;

#[cfg(test)]
mod mock;
