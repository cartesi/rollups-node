// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

/// Declares `$contract_name` as a module and includes everything from the `$contract_name` ABI.
macro_rules! contract {
    ($contract_name: ident) => {
        pub mod $contract_name {
            include!(concat!(
                env!("OUT_DIR"),
                "/",
                stringify!($contract_name),
                ".rs"
            ));
        }
    };
}

contract!(input_box);
contract!(authority);
contract!(history);
contract!(cartesi_dapp);
