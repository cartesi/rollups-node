// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use super::Address;
use clap::Parser;
use prometheus_client::encoding::EncodeLabelSet;
use serde_json::Value;
use std::{fs::File, io::BufReader};

/// DApp metadata used to define the stream keys
#[derive(Clone, Debug, Default, Hash, Eq, PartialEq, EncodeLabelSet)]
pub struct DAppMetadata {
    pub chain_id: u64,
    pub dapp_address: Address,
}

/// CLI configuration used to generate the DApp metadata
#[derive(Debug, Parser)]
pub struct DAppMetadataCLIConfig {
    /// Chain identifier
    #[arg(long, env, default_value = "0")]
    chain_id: u64,

    /// Address of rollups dapp
    #[arg(long, env)]
    dapp_contract_address: Option<String>,

    /// Path to file with address of rollups dapp
    #[arg(long, env)]
    dapp_contract_address_file: Option<String>,
}

impl From<DAppMetadataCLIConfig> for DAppMetadata {
    fn from(cli_config: DAppMetadataCLIConfig) -> DAppMetadata {
        let dapp_contract_address_raw = match cli_config.dapp_contract_address {
            Some(address) => address,
            None => {
                let path = cli_config
                    .dapp_contract_address_file
                    .expect("Configuration missing dapp address");
                let file = File::open(path).expect("Dapp json read file error");
                let reader = BufReader::new(file);
                let mut json: Value = serde_json::from_reader(reader)
                    .expect("Dapp json parse error");
                match json["address"].take() {
                    Value::String(s) => s,
                    Value::Null => panic!("Configuration missing dapp address"),
                    _ => panic!("Dapp json wrong type error"),
                }
            }
        };

        let dapp_contract_address: [u8; 20] =
            hex::decode(&dapp_contract_address_raw[2..])
                .expect("Dapp json parse error")
                .try_into()
                .expect("Dapp address with wrong size");

        DAppMetadata {
            chain_id: cli_config.chain_id,
            dapp_address: dapp_contract_address.into(),
        }
    }
}
