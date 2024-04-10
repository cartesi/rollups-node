// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::{
    contracts::iconsensus::{IConsensus, InputRange},
    metrics::AuthorityClaimerMetrics,
    rollups_events::{Address, DAppMetadata, RollupsClaim},
    signer::{ConditionalSigner, ConditionalSignerError},
};
use async_trait::async_trait;
use eth_tx_manager::{
    config::TxManagerConfig,
    database::FileSystemDatabase as Database,
    gas_oracle::DefaultGasOracle as GasOracle,
    manager::Configuration,
    time::DefaultTime as Time,
    transaction::{Priority, Transaction, Value},
    Chain,
};
use ethers::{
    self,
    middleware::SignerMiddleware,
    providers::{
        Http, HttpRateLimitRetryPolicy, MockProvider, Provider, RetryClient,
    },
    types::{NameOrAddress, H160},
};
use snafu::{OptionExt, ResultExt, Snafu};
use std::{fmt::Debug, sync::Arc};
use tracing::{info, trace};
use url::{ParseError, Url};

/// The `TransactionSender` sends claims to the blockchain.
///
/// It should wait for N blockchain confirmations.
#[async_trait]
pub trait TransactionSender: Sized + Debug {
    type Error: snafu::Error + 'static;

    /// The `send_rollups_claim_transaction` function consumes the
    /// `TransactionSender` object and then returns it to avoid
    /// that processes use the transaction sender concurrently.
    async fn send_rollups_claim_transaction(
        self,
        rollups_claim: RollupsClaim,
    ) -> Result<Self, Self::Error>;
}

// ------------------------------------------------------------------------------------------------
// DefaultTransactionSender
// ------------------------------------------------------------------------------------------------

type Middleware =
    Arc<SignerMiddleware<Provider<RetryClient<Http>>, ConditionalSigner>>;

type TransactionManager =
    eth_tx_manager::TransactionManager<Middleware, GasOracle, Database, Time>;

type TrasactionManagerError =
    eth_tx_manager::Error<Middleware, GasOracle, Database>;

/// Instantiates the tx-manager calling `new` or `force_new`.
macro_rules! tx_manager {
    ($new: ident, $middleware: expr, $database_path: expr, $chain: expr) => {
        TransactionManager::$new(
            $middleware.clone(),
            GasOracle::new(),
            Database::new($database_path.clone()),
            $chain,
            Configuration::default(),
        )
        .await
    };
}

#[derive(Debug)]
pub struct DefaultTransactionSender {
    tx_manager: TransactionManager,
    confirmations: usize,
    priority: Priority,
    from: ethers::types::Address,
    iconsensus: IConsensus<Provider<MockProvider>>,
    chain_id: u64,
    metrics: AuthorityClaimerMetrics,
}

#[derive(Debug, Snafu)]
pub enum TransactionSenderError {
    #[snafu(display("Invalid provider URL"))]
    ProviderUrl { source: ParseError },

    #[snafu(display("Failed to initialize the transaction signer"))]
    Signer { source: ConditionalSignerError },

    #[snafu(display("Transaction manager error"))]
    TransactionManager { source: TrasactionManagerError },

    #[snafu(display("Internal ethers-rs error: tx `to` should not be null"))]
    InternalEthers,

    #[snafu(display(
        "Internal configuration error: expected address, found ENS name"
    ))]
    InternalConfig,
}

/// Creates the (layered) middleware instance to be sent to the tx-manager.
fn create_middleware(
    conditional_signer: ConditionalSigner,
    provider_url: String,
) -> Result<Middleware, TransactionSenderError> {
    const MAX_RETRIES: u32 = 10;
    const INITIAL_BACKOFF: u64 = 1000;
    let url = Url::parse(&provider_url).context(ProviderUrlSnafu)?;
    let base_layer = Http::new(url);
    let retry_layer = Provider::new(RetryClient::new(
        base_layer,
        Box::new(HttpRateLimitRetryPolicy),
        MAX_RETRIES,
        INITIAL_BACKOFF,
    ));
    let signer_layer = SignerMiddleware::new(retry_layer, conditional_signer);
    Ok(Arc::new(signer_layer))
}

/// Creates the tx-manager instance.
/// NOTE: tries to re-instantiate the tx-manager only once.
async fn create_tx_manager(
    conditional_signer: ConditionalSigner,
    provider_url: String,
    database_path: String,
    chain: Chain,
) -> Result<TransactionManager, TransactionSenderError> {
    let middleware = create_middleware(conditional_signer, provider_url)?;
    let result = tx_manager!(new, middleware, database_path, chain);
    let tx_manager =
        if let Err(TrasactionManagerError::NonceTooLow { .. }) = result {
            info!("Nonce too low! Clearing the tx-manager database.");
            tx_manager!(force_new, middleware, database_path, chain)
                .context(TransactionManagerSnafu)?
        } else {
            let (tx_manager, receipt) =
                result.context(TransactionManagerSnafu)?;
            trace!("Database claim transaction confirmed: `{:?}`", receipt);
            tx_manager
        };
    Ok(tx_manager)
}

impl DefaultTransactionSender {
    pub async fn new(
        tx_manager_config: TxManagerConfig,
        tx_manager_priority: Priority,
        conditional_signer: ConditionalSigner,
        iconsensus: Address,
        from: ethers::types::Address,
        chain_id: u64,
        metrics: AuthorityClaimerMetrics,
    ) -> Result<Self, TransactionSenderError> {
        let chain: Chain = (&tx_manager_config).into();

        let tx_manager = create_tx_manager(
            conditional_signer,
            tx_manager_config.provider_http_endpoint.clone(),
            tx_manager_config.database_path.clone(),
            chain,
        )
        .await?;

        let iconsensus = {
            let (provider, _mock) = Provider::mocked();
            let provider = Arc::new(provider);
            let address: H160 = iconsensus.into_inner().into();
            IConsensus::new(address, provider)
        };

        Ok(Self {
            tx_manager,
            confirmations: tx_manager_config.default_confirmations,
            priority: tx_manager_priority,
            iconsensus,
            from,
            chain_id,
            metrics,
        })
    }
}

#[async_trait]
impl TransactionSender for DefaultTransactionSender {
    type Error = TransactionSenderError;

    async fn send_rollups_claim_transaction(
        self,
        rollups_claim: RollupsClaim,
    ) -> Result<Self, Self::Error> {
        let dapp_address = rollups_claim.dapp_address.clone();

        let transaction = {
            let input_range = InputRange {
                first_index: rollups_claim.first_index as u64,
                last_index: rollups_claim.last_index as u64,
            };
            let call = self
                .iconsensus
                .submit_claim(
                    H160(dapp_address.inner().to_owned()),
                    input_range,
                    rollups_claim.epoch_hash.into_inner(),
                )
                .from(self.from);
            let to = match call.tx.to().context(InternalEthersSnafu)? {
                NameOrAddress::Address(a) => *a,
                _ => return Err(TransactionSenderError::InternalConfig),
            };
            Transaction {
                from: self.from,
                to,
                value: Value::Nothing,
                call_data: call.tx.data().cloned(),
            }
        };

        trace!("Built claim transaction: `{:?}`", transaction);

        let (tx_manager, receipt) = self
            .tx_manager
            .send_transaction(transaction, self.confirmations, self.priority)
            .await
            .context(TransactionManagerSnafu)?;
        self.metrics
            .claims_sent
            .get_or_create(&DAppMetadata {
                chain_id: self.chain_id,
                dapp_address,
            })
            .inc();
        trace!("Claim transaction confirmed: `{:?}`", receipt);

        Ok(Self { tx_manager, ..self })
    }
}
