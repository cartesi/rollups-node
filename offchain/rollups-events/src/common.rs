// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use base64::{engine::general_purpose::STANDARD as base64_engine, Engine as _};
use prometheus_client::encoding::EncodeLabelValue;
use prometheus_client::encoding::LabelValueEncoder;
use serde::{Deserialize, Deserializer, Serialize, Serializer};
use std::fmt::Write;

pub const ADDRESS_SIZE: usize = 20;
pub const HASH_SIZE: usize = 32;

/// A binary array that is converted to a hex string when serialized
#[derive(Clone, Hash, Eq, PartialEq)]
pub struct HexArray<const N: usize>([u8; N]);

impl<const N: usize> HexArray<N> {
    pub const fn new(data: [u8; N]) -> Self {
        Self(data)
    }

    pub fn inner(&self) -> &[u8; N] {
        &self.0
    }

    pub fn mut_inner(&mut self) -> &mut [u8; N] {
        &mut self.0
    }

    pub fn into_inner(self) -> [u8; N] {
        self.0
    }
}

impl<const N: usize> From<[u8; N]> for HexArray<N> {
    fn from(data: [u8; N]) -> Self {
        Self::new(data)
    }
}

impl<const N: usize> Serialize for HexArray<N> {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        String::serialize(&hex::encode(self.inner()), serializer)
    }
}

impl<'de, const N: usize> Deserialize<'de> for HexArray<N> {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        let string_data = String::deserialize(deserializer)?;
        let vec_data = hex::decode(string_data).map_err(|e| {
            serde::de::Error::custom(format!("fail to decode hex ({})", e))
        })?;
        let data = vec_data
            .try_into()
            .or(Err(serde::de::Error::custom("incorrect array size")))?;
        Ok(Self::new(data))
    }
}

impl<const N: usize> std::fmt::Debug for HexArray<N> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", hex::encode(self.inner()))
    }
}

impl<const N: usize> Default for HexArray<N> {
    fn default() -> Self {
        Self::new([0; N])
    }
}

impl<const N: usize> EncodeLabelValue for HexArray<N> {
    fn encode(
        &self,
        encoder: &mut LabelValueEncoder<'_>,
    ) -> Result<(), std::fmt::Error> {
        write!(encoder, "{}", hex::encode(self.inner()))
    }
}

/// Blockchain hash
pub type Hash = HexArray<HASH_SIZE>;

/// Blockchain address
pub type Address = HexArray<ADDRESS_SIZE>;

/// Rollups payload.
/// When serialized, it is converted to a base64 string
#[derive(Default, Clone, Eq, PartialEq)]
pub struct Payload(Vec<u8>);

impl Payload {
    pub const fn new(data: Vec<u8>) -> Self {
        Self(data)
    }

    pub fn inner(&self) -> &Vec<u8> {
        &self.0
    }

    pub fn mut_inner(&mut self) -> &mut Vec<u8> {
        &mut self.0
    }

    pub fn into_inner(self) -> Vec<u8> {
        self.0
    }
}

impl From<Vec<u8>> for Payload {
    fn from(data: Vec<u8>) -> Self {
        Self::new(data)
    }
}

impl Serialize for Payload {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        String::serialize(&base64_engine.encode(self.inner()), serializer)
    }
}

impl<'de> Deserialize<'de> for Payload {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        let string_data = String::deserialize(deserializer)?;
        let data = base64_engine.decode(string_data).map_err(|e| {
            serde::de::Error::custom(format!("fail to decode base64 ({})", e))
        })?;
        Ok(Payload::new(data))
    }
}

impl std::fmt::Debug for Payload {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", base64_engine.encode(self.inner()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn serialize_array() {
        assert_eq!(
            serde_json::to_string(&Hash::new([0xfa; HASH_SIZE])).unwrap(),
            r#""fafafafafafafafafafafafafafafafafafafafafafafafafafafafafafafafa""#
        );
    }

    #[test]
    fn deserialize_array() {
        assert_eq!(
            serde_json::from_str::<Hash>(
                r#""fafafafafafafafafafafafafafafafafafafafafafafafafafafafafafafafa""#).unwrap(),
            Hash::new([0xfa; HASH_SIZE])
        );
    }

    #[test]
    fn fail_to_deserialized_invalid_array() {
        assert!(serde_json::from_str::<Hash>("\".\"")
            .unwrap_err()
            .to_string()
            .contains("fail to decode hex"));
    }

    #[test]
    fn fail_to_deserialized_array_with_wrong_size() {
        assert!(serde_json::from_str::<Hash>("\"ff\"")
            .unwrap_err()
            .to_string()
            .contains("incorrect array size"));
    }

    #[test]
    fn serialize_payload() {
        assert_eq!(
            serde_json::to_string(&Payload::new(vec![0xfa; 20])).unwrap(),
            "\"+vr6+vr6+vr6+vr6+vr6+vr6+vo=\""
        );
    }

    #[test]
    fn deserialize_payload() {
        assert_eq!(
            serde_json::from_str::<Payload>("\"+vr6+vr6+vr6+vr6+vr6+vr6+vo=\"")
                .unwrap(),
            Payload::new(vec![0xfa; 20])
        );
    }

    #[test]
    fn fail_to_deserialized_invalid_payload() {
        assert!(serde_json::from_str::<Payload>("\".\"")
            .unwrap_err()
            .to_string()
            .contains("fail to decode base64"));
    }
}
