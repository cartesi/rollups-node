// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use eth_state_fold_types::{
    ethereum_types::{Address, Bloom, H160, H256},
    Block,
};
use im::{hashmap, Vector};
use rollups_events::RollupsClaim;
use snafu::whatever;
use std::{
    collections::VecDeque,
    ops::{Deref, DerefMut},
    sync::{Arc, Mutex},
};
use types::foldables::{DAppInputBox, Input, InputBox};

use crate::machine::{
    rollups_broker::BrokerFacadeError, BrokerSend, BrokerStatus, RollupStatus,
};

// ------------------------------------------------------------------------------------------------
// auxiliary functions
// ------------------------------------------------------------------------------------------------

pub fn new_block(number: u64) -> Block {
    Block {
        hash: H256::random(),
        number: number.into(),
        parent_hash: H256::random(),
        timestamp: 0.into(),
        logs_bloom: Bloom::default(),
    }
}

pub fn new_input(block: u64) -> Input {
    Input {
        sender: Arc::new(H160::random()),
        payload: vec![],
        block_added: Arc::new(new_block(block)),
        dapp: Arc::new(H160::random()),
        tx_hash: Arc::new(H256::default()),
    }
}

pub fn new_input_box() -> InputBox {
    InputBox {
        dapp_address: Arc::new(H160::random()),
        input_box_address: Arc::new(H160::random()),
        dapp_input_boxes: Arc::new(hashmap! {}),
    }
}

pub fn update_input_box(
    input_box: InputBox,
    dapp_address: Address,
    blocks: Vec<u64>,
) -> InputBox {
    let inputs = blocks
        .iter()
        .map(|block| Arc::new(new_input(*block)))
        .collect::<Vec<_>>();
    let inputs = Vector::from(inputs);
    let dapp_input_boxes = input_box
        .dapp_input_boxes
        .update(Arc::new(dapp_address), Arc::new(DAppInputBox { inputs }));
    InputBox {
        dapp_address: Arc::new(dapp_address),
        input_box_address: input_box.input_box_address,
        dapp_input_boxes: Arc::new(dapp_input_boxes),
    }
}

// ------------------------------------------------------------------------------------------------
// Broker
// ------------------------------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum Event {
    Input(u64), // input index
    Finish,
}

#[derive(Debug)]
pub struct Broker {
    pub rollup_statuses: Mutex<VecDeque<RollupStatus>>,
    pub next_claims: Mutex<VecDeque<RollupsClaim>>,
    pub events: Mutex<Vec<Event>>,
    status_error: bool,
    enqueue_input_error: bool,
    finish_epoch_error: bool,
}

impl Broker {
    fn default() -> Self {
        Self {
            rollup_statuses: Mutex::new(VecDeque::new()),
            next_claims: Mutex::new(VecDeque::new()),
            events: Mutex::new(Vec::new()),
            status_error: false,
            enqueue_input_error: false,
            finish_epoch_error: false,
        }
    }

    pub fn new(
        rollup_statuses: Vec<RollupStatus>,
        next_claims: Vec<RollupsClaim>,
    ) -> Self {
        let mut broker = Self::default();
        broker.rollup_statuses = Mutex::new(rollup_statuses.into());
        broker.next_claims = Mutex::new(next_claims.into());
        broker
    }

    pub fn with_enqueue_input_error() -> Self {
        let mut broker = Self::default();
        broker.enqueue_input_error = true;
        broker
    }

    pub fn with_finish_epoch_error() -> Self {
        let mut broker = Self::default();
        broker.finish_epoch_error = true;
        broker
    }

    fn events_len(&self) -> usize {
        let mutex_guard = self.events.lock().unwrap();
        mutex_guard.deref().len()
    }

    fn get_event(&self, i: usize) -> Event {
        let mutex_guard = self.events.lock().unwrap();
        mutex_guard.deref().get(i).unwrap().clone()
    }

    pub fn assert_state(&self, expected: Vec<Event>) {
        assert_eq!(
            self.events_len(),
            expected.len(),
            "\n{:?}\n{:?}",
            self.events.lock().unwrap().deref(),
            expected
        );
        println!("Events:");
        for (i, expected) in expected.iter().enumerate() {
            let event = self.get_event(i);
            println!("index: {:?} => {:?} - {:?}", i, event, expected);
            assert_eq!(event, *expected);
        }
    }
}

#[async_trait]
impl BrokerStatus for Broker {
    async fn status(&self) -> Result<RollupStatus, BrokerFacadeError> {
        if self.status_error {
            whatever!("status error")
        } else {
            let mut mutex_guard = self.rollup_statuses.lock().unwrap();
            Ok(mutex_guard.deref_mut().pop_front().unwrap())
        }
    }
}

#[async_trait]
impl BrokerSend for Broker {
    async fn enqueue_input(
        &self,
        input_index: u64,
        _: &Input,
    ) -> Result<(), BrokerFacadeError> {
        if self.enqueue_input_error {
            whatever!("enqueue_input error")
        } else {
            let mut mutex_guard = self.events.lock().unwrap();
            mutex_guard.deref_mut().push(Event::Input(input_index));
            Ok(())
        }
    }

    async fn finish_epoch(&self, _: u64) -> Result<(), BrokerFacadeError> {
        if self.finish_epoch_error {
            whatever!("finish_epoch error")
        } else {
            let mut mutex_guard = self.events.lock().unwrap();
            mutex_guard.deref_mut().push(Event::Finish);
            Ok(())
        }
    }
}
