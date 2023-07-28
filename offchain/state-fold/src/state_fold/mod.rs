// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

pub mod config;
pub mod error;
pub mod utils;

mod delegate_access;
mod env;
mod foldable;

pub use delegate_access::{AccessError, FoldMiddleware, SyncMiddleware};
pub use env::StateFoldEnvironment;
pub use foldable::Foldable;

#[cfg(test)]
mod test_utils;
