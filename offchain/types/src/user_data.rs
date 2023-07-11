// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use state_fold_types::ethers::types::Address;

use std::{collections::HashSet, sync::Arc};

#[derive(Debug, Default)]
pub struct UserData {
    addresses: HashSet<Arc<Address>>,
}

impl UserData {
    pub fn get(&mut self, address: Address) -> Arc<Address> {
        // Method `get_or_insert` of HashSet is still unstable
        match self.addresses.get(&address) {
            Some(s) => Arc::clone(s),
            None => {
                let s = Arc::new(address);
                assert!(self.addresses.insert(s.clone()));
                s
            }
        }
    }
}
