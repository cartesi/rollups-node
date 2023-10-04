// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

pub(crate) fn main() {
    built::write_built_file()
        .expect("Failed to acquire build-time information");
}
