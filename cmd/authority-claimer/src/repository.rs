// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use ethers::types::H256;
use sea_query::{Alias, Expr, Func, Iden, Order, PostgresQueryBuilder, Query};
use sea_query_binder::SqlxBinder;
use snafu::{ResultExt, Snafu};
use sqlx::{
    pool::PoolConnection, postgres::PgPoolOptions, types::Decimal, Pool,
    Postgres,
};
use std::sync::Arc;
use std::{fmt::Debug, time::Duration};

use crate::rollups_events::{Address, Hash, RollupsClaim};

const REPOSITORY_MIN_CONNECTIONS: u32 = 2;
const REPOSITORY_MAX_CONNECTIONS: u32 = 10;
const REPOSITORY_ACQUIRE_TIMEOUT: Duration = Duration::new(15, 0);

/// The `Repository` queries the database and gets an unsubmitted claim
#[async_trait]
pub trait Repository: Debug {
    type Error: snafu::Error + 'static;

    async fn get_claim(
        &mut self,
    ) -> Result<(RollupsClaim, Address), Self::Error>;

    async fn update_claim(
        &mut self,
        id: u64,
        tx_hash: H256,
    ) -> Result<(), Self::Error>;
}

#[derive(Debug, Snafu)]
#[snafu(visibility(pub(super)))]
pub enum RepositoryError {
    #[snafu(display("database error"))]
    DatabaseSqlx { source: sqlx::Error },

    #[snafu(display("int conversion error"))]
    IntConversion { source: std::num::TryFromIntError },
}

#[derive(Clone, Debug)]
pub struct DefaultRepository {
    // Connection is not thread-safe, we use a connection pool
    db_pool: Arc<Pool<Postgres>>,
}

impl DefaultRepository {
    /// Create database connection pool, wait until database server is available
    pub fn new(endpoint: String) -> Result<Self, RepositoryError> {
        let connection = PgPoolOptions::new()
            .acquire_timeout(REPOSITORY_ACQUIRE_TIMEOUT)
            .min_connections(REPOSITORY_MIN_CONNECTIONS)
            .max_connections(REPOSITORY_MAX_CONNECTIONS)
            .connect_lazy(&endpoint)
            .context(DatabaseSqlxSnafu)?;
        Ok(Self {
            db_pool: Arc::new(connection),
        })
    }

    /// Obtain a connection from the connection pool
    async fn conn(&self) -> PoolConnection<Postgres> {
        self.db_pool
            .acquire()
            .await
            .expect("No connections available in the pool")
    }
}

#[async_trait]
impl Repository for DefaultRepository {
    type Error = RepositoryError;

    async fn get_claim(
        &mut self,
    ) -> Result<(RollupsClaim, Address), Self::Error> {
        let claim: RollupsClaim;
        let iconsensus_address: Address;
        let mut conn = self.conn().await;
        let (sql, values) = Query::select()
            .columns([
                (Epoch::Table, Epoch::Id),
                (Epoch::Table, Epoch::ClaimHash),
                (Epoch::Table, Epoch::ApplicationAddress),
                (Epoch::Table, Epoch::LastBlock),
            ])
            .column((Application::Table, Application::IconsensusAddress))
            .from(Epoch::Table)
            .inner_join(
                Application::Table,
                Expr::col((Epoch::Table, Epoch::ApplicationAddress))
                    .equals((Application::Table, Application::ContractAddress)),
            )
            .and_where(Expr::col((Epoch::Table, Epoch::Status)).eq(
                Func::cast_as("CLAIM_COMPUTED", Alias::new("\"EpochStatus\"")),
            ))
            .order_by(Epoch::Index, Order::Asc)
            .order_by((Epoch::Table, Epoch::Id), Order::Asc)
            .limit(1)
            .build_sqlx(PostgresQueryBuilder);

        loop {
            let result = sqlx::query_as_with::<_, QueryResponse, _>(
                &sql,
                values.clone(),
            )
            .fetch_optional(&mut *conn)
            .await
            .context(DatabaseSqlxSnafu)?;

            match result {
                Some(row) => {
                    claim = RollupsClaim {
                        id: row.id.try_into().context(IntConversionSnafu)?,
                        last_block: row.last_block.try_into().unwrap(),
                        dapp_address: Address::new(
                            row.application_address.try_into().unwrap(),
                        ),
                        output_merkle_root_hash: Hash::new(
                            row.claim_hash.try_into().unwrap(),
                        ),
                    };
                    iconsensus_address = Address::new(
                        row.iconsensus_address.try_into().unwrap(),
                    );
                    break;
                }
                None => continue,
            }
        }

        let _ = conn.close().await.context(DatabaseSqlxSnafu)?;
        Ok((claim, iconsensus_address))
    }

    async fn update_claim(
        &mut self,
        id: u64,
        tx_hash: H256,
    ) -> Result<(), Self::Error> {
        let mut conn = self.conn().await;
        let (sql, values) = Query::update()
            .table(Epoch::Table)
            .values([
                (
                    Epoch::Status,
                    Func::cast_as(
                        "CLAIM_SUBMITTED",
                        Alias::new("\"EpochStatus\""),
                    )
                    .into(),
                ),
                (
                    Epoch::TransactionHash,
                    tx_hash.as_fixed_bytes().to_vec().into(),
                ),
            ])
            .and_where(Expr::col(Epoch::Id).eq(id))
            .build_sqlx(PostgresQueryBuilder);

        let _result = sqlx::query_with(&sql, values)
            .execute(&mut *conn)
            .await
            .context(DatabaseSqlxSnafu)?;

        let _ = conn.close().await.context(DatabaseSqlxSnafu)?;
        Ok(())
    }
}

#[derive(Iden)]
enum Epoch {
    Table,
    Id,
    Index,
    Status,
    LastBlock,
    ClaimHash,
    TransactionHash,
    ApplicationAddress,
}

#[derive(Iden)]
enum Application {
    Table,
    ContractAddress,
    IconsensusAddress,
}

#[derive(sqlx::FromRow, Debug, Clone)]
#[allow(dead_code)]
struct QueryResponse {
    id: i64,
    last_block: Decimal,
    claim_hash: Vec<u8>,
    application_address: Vec<u8>,
    iconsensus_address: Vec<u8>,
}
