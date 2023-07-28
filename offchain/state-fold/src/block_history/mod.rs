// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

mod block_archive;
mod block_subscriber;
mod block_tree;

pub mod config;

pub use block_archive::BlockArchive;
pub use block_subscriber::BlockSubscriber;

pub use block_archive::BlockArchiveError;
pub use block_subscriber::BlockSubscriberError;
pub use block_subscriber::SubscriptionError;

pub use block_archive::{
    current_block_number, fetch_block, fetch_block_at_depth,
};
