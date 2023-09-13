// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use ethers::{
    signers::{
        coins_bip39::English, AwsSignerError, LocalWallet, MnemonicBuilder,
        Signer, WalletError,
    },
    types::{
        transaction::{eip2718::TypedTransaction, eip712::Eip712},
        Address, Signature,
    },
};
use snafu::{ResultExt, Snafu};

use crate::{auth::AuthConfig, signer::aws_signer::AwsSigner};

/// The `ConditionalSigner` is implementing conditional dispatch (instead of
/// dynamic dispatch) by hand for objects that implement the `Sender` trait.
///
/// We had to do this because (1) we cannot create a `Box<dyn Signer>` and
/// (2) using parametric types would move this complexity to the main loop,
/// which is undesirable.
#[derive(Debug, Clone)]
pub enum ConditionalSigner {
    LocalWallet(LocalWallet),
    AwsSigner(AwsSigner),
}

#[derive(Debug, Snafu)]
pub enum ConditionalSignerError {
    #[snafu(display("Local wallet signer error"))]
    LocalWallet { source: WalletError },

    #[snafu(display("AWS KMS signer error"))]
    AwsSigner { source: AwsSignerError },
}

impl ConditionalSigner {
    pub async fn new(
        chain_id: u64,
        auth_config: &AuthConfig,
    ) -> Result<Self, ConditionalSignerError> {
        match auth_config.clone() {
            AuthConfig::Mnemonic {
                mnemonic,
                account_index,
            } => {
                const DEFAULT_ACCOUNT_INDEX: u32 = 0;
                let index = account_index.unwrap_or(DEFAULT_ACCOUNT_INDEX);
                let wallet = MnemonicBuilder::<English>::default()
                    .phrase(mnemonic.as_str())
                    .index(index)
                    .context(LocalWalletSnafu)?
                    .build()
                    .context(LocalWalletSnafu)?
                    .with_chain_id(chain_id);
                Ok(ConditionalSigner::LocalWallet(wallet))
            }
            AuthConfig::Aws { key_id, region } => {
                AwsSigner::new(key_id, chain_id, region)
                    .await
                    .map(ConditionalSigner::AwsSigner)
                    .context(AwsSignerSnafu)
            }
        }
    }
}

#[async_trait]
impl Signer for ConditionalSigner {
    type Error = ConditionalSignerError;

    async fn sign_message<S: Send + Sync + AsRef<[u8]>>(
        &self,
        message: S,
    ) -> Result<Signature, Self::Error> {
        match &self {
            Self::LocalWallet(local_wallet) => local_wallet
                .sign_message(message)
                .await
                .context(LocalWalletSnafu),
            Self::AwsSigner(aws_signer) => aws_signer
                .sign_message(message)
                .await
                .context(AwsSignerSnafu),
        }
    }

    async fn sign_transaction(
        &self,
        message: &TypedTransaction,
    ) -> Result<Signature, Self::Error> {
        match &self {
            Self::LocalWallet(local_wallet) => local_wallet
                .sign_transaction(message)
                .await
                .context(LocalWalletSnafu),
            Self::AwsSigner(aws_signer) => aws_signer
                .sign_transaction(message)
                .await
                .context(AwsSignerSnafu),
        }
    }

    async fn sign_typed_data<T: Eip712 + Send + Sync>(
        &self,
        payload: &T,
    ) -> Result<Signature, Self::Error> {
        match &self {
            Self::LocalWallet(local_wallet) => local_wallet
                .sign_typed_data(payload)
                .await
                .context(LocalWalletSnafu),
            Self::AwsSigner(aws_signer) => aws_signer
                .sign_typed_data(payload)
                .await
                .context(AwsSignerSnafu),
        }
    }

    fn address(&self) -> Address {
        match &self {
            Self::LocalWallet(local_wallet) => local_wallet.address(),
            Self::AwsSigner(aws_signer) => aws_signer.address(),
        }
    }

    fn chain_id(&self) -> u64 {
        match &self {
            Self::LocalWallet(local_wallet) => local_wallet.chain_id(),
            Self::AwsSigner(aws_signer) => aws_signer.chain_id(),
        }
    }

    fn with_chain_id<T: Into<u64>>(self, chain_id: T) -> Self {
        match &self {
            Self::LocalWallet(local_wallet) => {
                Self::LocalWallet(local_wallet.clone().with_chain_id(chain_id))
            }
            Self::AwsSigner(aws_signer) => {
                Self::AwsSigner(aws_signer.clone().with_chain_id(chain_id))
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use ethers::types::{
        transaction::{eip2718::TypedTransaction, eip2930::AccessList},
        Address, Eip1559TransactionRequest,
    };
    use ethers_signers::Signer;

    use crate::{auth::AuthConfig, signer::ConditionalSigner};

    // --------------------------------------------------------------------------------------------
    // new
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn new_local_wallet_conditional_signer() {
        let conditional_signer = local_wallet_conditional_signer().await;
        assert!(matches!(
            conditional_signer,
            ConditionalSigner::LocalWallet(_)
        ));
    }

    // --------------------------------------------------------------------------------------------
    // sign_transaction
    // --------------------------------------------------------------------------------------------

    #[tokio::test]
    async fn sign_transaction_with_local_wallet_conditional_signer() {
        let conditional_signer = local_wallet_conditional_signer().await;
        let message = eip1559_message();
        let result = conditional_signer.sign_transaction(&message).await;
        assert!(result.is_ok());
    }

    // --------------------------------------------------------------------------------------------
    // auxiliary
    // --------------------------------------------------------------------------------------------

    const CHAIN_ID: u64 = 1;
    const MNEMONIC: &str =
        "indoor dish desk flag debris potato excuse depart ticket judge file exit";

    async fn local_wallet_conditional_signer() -> ConditionalSigner {
        let auth_config = AuthConfig::Mnemonic {
            mnemonic: MNEMONIC.to_string(),
            account_index: Some(1),
        };
        ConditionalSigner::new(CHAIN_ID, &auth_config)
            .await
            .unwrap()
    }

    fn eip1559_message() -> TypedTransaction {
        TypedTransaction::Eip1559(
            Eip1559TransactionRequest::new()
                .from(Address::default())
                .to(Address::default())
                .gas(555)
                .value(1337)
                .data(vec![1, 2, 3])
                .nonce(1)
                .access_list(AccessList::default())
                .max_priority_fee_per_gas(10)
                .max_fee_per_gas(20)
                .chain_id(CHAIN_ID),
        )
    }
}
