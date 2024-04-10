// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use eth_state_fold_types::contract;
use snafu::prelude::*;
use std::{
    error::Error,
    fs::File,
    path::{Path, PathBuf},
    process::Command,
    str,
};

const ROLLUPS_CONTRACTS_URL: &str =
    "https://registry.npmjs.org/@cartesi/rollups/-/rollups-2.0.0-rc.2.tgz";

fn main() -> Result<(), Box<dyn Error>> {
    let tempdir = tempfile::tempdir()?;
    let tarball = tempdir.path().join("rollups.tgz");
    download_contracts(&tarball)?;
    unzip_contracts(&tarball, tempdir.path())?;

    let contracts = vec![("consensus", "IConsensus", "iconsensus.rs")];
    for (contract_path, contract_name, bindings_file_name) in contracts {
        let source_path = path(tempdir.path(), contract_path, contract_name);
        let output_path: PathBuf =
            [&std::env::var("OUT_DIR").unwrap(), bindings_file_name]
                .iter()
                .collect();
        let source = File::open(&source_path)?;
        let output = File::create(&output_path)?;
        contract::write(contract_name, source, output)?;
    }

    println!("cargo:rerun-if-changed=build.rs");
    Ok(())
}

fn run_cmd(cmd: &str, args: &[&str]) -> Result<(), snafu::Whatever> {
    let output = Command::new(cmd)
        .args(args)
        .output()
        .whatever_context("failed to execute command")?;
    if !output.status.success() {
        let err = str::from_utf8(&output.stderr)
            .whatever_context("failed to convert string")?;
        whatever!("{} exited with error: {}", cmd, err);
    }
    Ok(())
}

fn download_contracts(output: &Path) -> Result<(), snafu::Whatever> {
    run_cmd(
        "curl",
        &[
            ROLLUPS_CONTRACTS_URL,
            "-o",
            output.to_str().expect("failed to convert path"),
        ],
    )
}

fn unzip_contracts(file: &Path, target: &Path) -> Result<(), snafu::Whatever> {
    run_cmd(
        "tar",
        &[
            "zxf",
            file.to_str().expect("failed to convert path"),
            "-C",
            target.to_str().expect("failed to convert path"),
        ],
    )
}

fn path(basedir: &Path, contract_path: &str, contract_name: &str) -> PathBuf {
    basedir
        .join("package/export/artifacts/contracts")
        .join(contract_path)
        .join(format!("{}.sol", contract_name))
        .join(format!("{}.json", contract_name))
}
