// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use ethers::contract::Abigen;
use std::{
    error::Error,
    fs,
    path::{Path, PathBuf},
    str,
};

const ROLLUPS_CONTRACTS_PATH: &str =
    "../../rollups-contracts/export/artifacts/contracts";

fn main() -> Result<(), Box<dyn Error>> {
    let contract_path = "consensus";
    let contract_name = "IConsensus";
    let bindings_file_name = "iconsensus.rs";

    let source_path = path(contract_path, contract_name);
    let output_path: PathBuf =
        [&std::env::var("OUT_DIR").unwrap(), bindings_file_name]
            .iter()
            .collect();

    let bindings =
        Abigen::new(contract_name, fs::read_to_string(&source_path)?)?
            .generate()?;
    bindings.write_to_file(&output_path)?;

    println!("cargo:rerun-if-changed=build.rs");
    Ok(())
}

fn path(contract_path: &str, contract_name: &str) -> PathBuf {
    Path::new(ROLLUPS_CONTRACTS_PATH)
        .join(contract_path)
        .join(format!("{}.sol", contract_name))
        .join(format!("{}.json", contract_name))
}
