// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use ethabi::ethereum_types::BloomInput;
use ethers::types::{Address, Bloom, H256, U256, U64};

pub fn contains_address(bloom: &Bloom, address: &Address) -> bool {
    bloom.contains_input(BloomInput::Raw(address.as_bytes()))
}

pub fn contains_topic<T: Into<TopicInput>>(bloom: &Bloom, topic: T) -> bool {
    let x: TopicInput = topic.into();
    bloom.contains_input(BloomInput::Raw(&x.0))
}

#[derive(Default)]
pub struct TopicInput([u8; 32]);

impl From<&U256> for TopicInput {
    fn from(src: &U256) -> Self {
        let mut this = Self::default();
        src.to_big_endian(&mut this.0);
        this
    }
}

impl From<&U64> for TopicInput {
    fn from(src: &U64) -> Self {
        let x = U256::from(src.as_u64());
        TopicInput::from(&x)
    }
}

impl From<&H256> for TopicInput {
    fn from(src: &H256) -> Self {
        Self(src.to_fixed_bytes())
    }
}

impl From<&Address> for TopicInput {
    fn from(src: &Address) -> Self {
        let mut this = Self::default();
        let bytes = src.as_fixed_bytes();
        for (i, x) in this.0[12..].iter_mut().enumerate() {
            *x = bytes[i];
        }
        this
    }
}
