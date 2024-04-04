// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use ethers::{
    signers::{AwsSigner as InnerAwsSigner, AwsSignerError, Signer},
    types::{
        transaction::{eip2718::TypedTransaction, eip712::Eip712},
        Address, Signature,
    },
};
use rusoto_core::{HttpClient, Region};
use rusoto_kms::KmsClient;

use super::aws_credentials::AwsCredentialsProvider;

/// The `AwsSigner` (re)implements the `Signer` trait for the `InnerAwsSigner`.
///
/// We do not use an `InnerAwsSigner` directly because of lifetime and
/// borrow restrictions imposed by the underlying libraries.
///
/// Instead, we instantiate a new `InnerAwsSigner` every time we call
/// a function from `Signer`.
#[derive(Debug, Clone)]
pub struct AwsSigner {
    region: Region,
    key_id: String,
    chain_id: u64,
    address: Address,
}

/// Creates a `KmsClient` instance.
fn create_kms(region: &Region) -> KmsClient {
    let request_dispatcher = HttpClient::new().expect("http client TLS error");
    let region = region.clone();
    let credentials_provider = AwsCredentialsProvider::new()
        .expect("could not instantiate AWS credentials provider");
    KmsClient::new_with(request_dispatcher, credentials_provider, region)
}

impl AwsSigner {
    pub async fn new(
        key_id: String,
        chain_id: u64,
        region: Region,
    ) -> Result<Self, AwsSignerError> {
        let kms = create_kms(&region);
        let aws_signer =
            InnerAwsSigner::new(&kms, key_id.clone(), chain_id).await?;
        Ok(Self {
            region,
            key_id,
            chain_id,
            address: aws_signer.address(),
        })
    }
}

/// Calls the async `$method` from an `InnerAwsSigner` instance.
/// Reinstantiates the `InnerAwsSigner`.
macro_rules! inner_aws_signer_call {
    ($aws_signer: expr,
     $method: ident,
     $argument: expr) => {
        InnerAwsSigner::new(
            &create_kms(&$aws_signer.region),
            &$aws_signer.key_id.clone(),
            $aws_signer.chain_id,
        )
        .await?
        .$method($argument)
        .await
    };
}

#[async_trait]
impl Signer for AwsSigner {
    type Error = AwsSignerError;

    async fn sign_message<S: Send + Sync + AsRef<[u8]>>(
        &self,
        message: S,
    ) -> Result<Signature, Self::Error> {
        inner_aws_signer_call!(self, sign_message, message)
    }

    async fn sign_transaction(
        &self,
        message: &TypedTransaction,
    ) -> Result<Signature, Self::Error> {
        inner_aws_signer_call!(self, sign_transaction, message)
    }

    async fn sign_typed_data<T: Eip712 + Send + Sync>(
        &self,
        payload: &T,
    ) -> Result<Signature, Self::Error> {
        inner_aws_signer_call!(self, sign_typed_data, payload)
    }

    fn address(&self) -> Address {
        self.address
    }

    fn chain_id(&self) -> u64 {
        self.chain_id
    }

    fn with_chain_id<T: Into<u64>>(self, chain_id: T) -> Self {
        Self {
            key_id: self.key_id.clone(),
            chain_id: chain_id.into(),
            region: self.region.clone(),
            address: self.address,
        }
    }
}

#[cfg(test)]
pub mod tests {
    use rusoto_core::Region;

    use crate::signer::aws_signer::AwsSigner;

    #[tokio::test]
    async fn new_aws_signer_with_error() {
        let invalid_key_id = "invalid".to_string();
        let aws_signer =
            AwsSigner::new(invalid_key_id, 0, Region::UsEast1).await;
        assert!(aws_signer.is_err());
    }
}
