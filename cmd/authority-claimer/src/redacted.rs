// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use std::fmt;

/// Wrapper that redacts the entire field
#[derive(Clone)]
pub struct Redacted<T: Clone>(T);

impl<T: Clone> Redacted<T> {
    pub fn new(data: T) -> Redacted<T> {
        Self(data)
    }

    pub fn inner(&self) -> &T {
        &self.0
    }

    pub fn into_inner(self) -> T {
        self.0
    }
}

impl<T: Clone> fmt::Debug for Redacted<T> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "[REDACTED]")
    }
}

#[test]
fn redacts_debug_fmt() {
    let password = Redacted::new("super-security");
    assert_eq!(format!("{:?}", password), "[REDACTED]");
}
