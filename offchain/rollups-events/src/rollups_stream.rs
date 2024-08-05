// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::Address;
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

pub fn parse_stream_with_key(key: String, inner_key: &str) -> (u64, Address) {
    let mut re = r"^\{chain-([^:]+):dapp-([^}]+)\}:".to_string();
    re.push_str(inner_key);
    re.push_str("$");
    let re = regex::Regex::new(&re).unwrap();
    let caps = re.captures(&key).unwrap();

    let chain_id = caps
        .get(1)
        .unwrap()
        .as_str()
        .to_string()
        .parse::<u64>()
        .unwrap();
    let address = caps.get(2).unwrap().as_str().to_string();
    let address =
        serde_json::from_value(serde_json::Value::String(address)).unwrap();

    return (chain_id, address);
}

/// Declares a struct that implements the BrokerStream interface
/// The generated key has the format `{chain-<chain_id>:dapp-<dapp_address>}:<key>`.
/// The curly braces define a hash tag to ensure that all of a dapp's streams
/// are located in the same node when connected to a Redis cluster.
macro_rules! decl_broker_stream {
    ($stream: ident, $payload: ty, $key: literal) => {
        #[derive(Debug, Clone, PartialEq, Eq, Hash)]
        pub struct $stream {
            key: String,
            pub chain_id: u64,
            pub dapp_address: Address,
        }

        impl crate::broker::BrokerStream for $stream {
            type Payload = $payload;

            fn key(&self) -> &str {
                &self.key
            }
        }

        impl crate::broker::BrokerMultiStream for $stream {
            fn from_key(key: String) -> Self {
                let (chain_id, dapp_address) =
                    crate::parse_stream_with_key(key.clone(), $key);
                Self {
                    key: key,
                    chain_id: chain_id,
                    dapp_address: dapp_address,
                }
            }
        }

        impl $stream {
            pub fn new(metadata: &crate::rollups_stream::DAppMetadata) -> Self {
                let chain_id = metadata.chain_id;
                let dapp_address = metadata.dapp_address.clone();
                Self {
                    key: format!(
                        "{{chain-{}:dapp-{}}}:{}",
                        chain_id,
                        hex::encode(dapp_address.inner()),
                        $key
                    ),
                    chain_id: chain_id,
                    dapp_address: dapp_address,
                }
            }
        }
    };
}

pub(crate) use decl_broker_stream;

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{broker::BrokerMultiStream, BrokerStream, ADDRESS_SIZE};
    use serde::{Deserialize, Serialize};

    #[derive(Debug, Clone, Eq, PartialEq, Serialize, Deserialize)]
    pub struct MockPayload;

    decl_broker_stream!(MockStream, MockPayload, "rollups-mock");

    #[test]
    fn it_generates_the_key() {
        let metadata = DAppMetadata {
            chain_id: 123,
            dapp_address: Address::new([0xfa; ADDRESS_SIZE]),
        };
        let stream = MockStream::new(&metadata);
        assert_eq!(stream.key, "{chain-123:dapp-fafafafafafafafafafafafafafafafafafafafa}:rollups-mock");
    }

    #[test]
    fn it_parses_the_key() {
        let metadata = DAppMetadata {
            chain_id: 123,
            dapp_address: Address::new([0xfe; ADDRESS_SIZE]),
        };

        let stream = MockStream::new(&metadata);
        let expected = "{chain-123:dapp-fefefefefefefefefefefefefefefefefefefefe}:rollups-mock";
        let key = stream.key().to_string();
        assert_eq!(expected, &key);

        let stream = MockStream::from_key(key);
        assert_eq!(metadata.chain_id, stream.chain_id);
        assert_eq!(metadata.dapp_address, stream.dapp_address);
    }
}
