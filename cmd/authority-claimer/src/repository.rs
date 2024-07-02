// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use ethers::types::H256;
use sea_query::{Alias, Expr, Func, Iden, Order, PostgresQueryBuilder, Query};
use sea_query_binder::SqlxBinder;
use snafu::{ResultExt, Snafu};
use sqlx::{pool::PoolConnection, PgPool, Pool, Postgres};
use std::fmt::Debug;
use std::sync::Arc;

use crate::rollups_events::{Address, Hash, RollupsClaim};

/// The `Repository` queries the database and gets an unsubmitted claim
#[async_trait]
pub trait Repository: Debug {
    type Error: snafu::Error + 'static;

    async fn get_claim(&mut self) -> Result<RollupsClaim, Self::Error>;

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
    // Connection is not thread safe to share between threads, we use connection pool
    db_pool: Arc<Pool<Postgres>>,
}

impl DefaultRepository {
    /// Create database connection pool, wait until database server is available with backoff strategy
    pub async fn new(endpoint: String) -> Result<Self, RepositoryError> {
        let connection = PgPool::connect(&endpoint)
            .await
            .context(DatabaseSqlxSnafu)?;
        Ok(Self {
            db_pool: Arc::new(connection),
        })
    }

    /// Obtain the connection from the connection pool
    fn conn(&self) -> PoolConnection<Postgres> {
        self.db_pool.try_acquire().unwrap()
    }
}

/// Basic queries that fetch by primary_key
#[async_trait]
impl Repository for DefaultRepository {
    type Error = RepositoryError;

    async fn get_claim(&mut self) -> Result<RollupsClaim, Self::Error> {
        let claim: RollupsClaim;
        let mut conn = self.conn();
        let (sql, values) =
            Query::select()
                .columns([
                    Claim::Id,
                    Claim::OutputMerkleRootHash,
                    Claim::ApplicationAddress,
                ])
                .from(Claim::Table)
                .and_where(Expr::col(Claim::Status).eq(Func::cast_as(
                    "PENDING",
                    Alias::new("\"ClaimStatus\""),
                )))
                .order_by(Claim::Id, Order::Desc)
                .limit(1)
                .build_sqlx(PostgresQueryBuilder);

        loop {
            let result = sqlx::query_as_with::<_, ClaimsResponse, _>(
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
                        dapp_address: Address::new(
                            row.application_address.try_into().unwrap(),
                        ),
                        output_merkle_root_hash: Hash::new(
                            row.output_merkle_root_hash.try_into().unwrap(),
                        ),
                    };
                    break;
                }
                // The division was invalid
                None => continue,
            }
        }

        Ok(claim)
    }

    async fn update_claim(
        &mut self,
        id: u64,
        tx_hash: H256,
    ) -> Result<(), Self::Error> {
        let mut conn = self.conn();
        let (sql, values) = Query::update()
            .table(Claim::Table)
            .values([
                (Claim::Status, "SUBMITTED".into()),
                (
                    Claim::TransactionHash,
                    tx_hash.as_fixed_bytes().to_vec().into(),
                ),
            ])
            .and_where(Expr::col(Claim::Id).eq(id))
            .build_sqlx(PostgresQueryBuilder);

        let _result = sqlx::query_with(&sql, values)
            .execute(&mut *conn)
            .await
            .context(DatabaseSqlxSnafu)?;
        Ok(())
    }
}

#[derive(Iden)]
enum Claim {
    Table,
    Id,
    Status,
    OutputMerkleRootHash,
    TransactionHash,
    ApplicationAddress,
}

#[derive(sqlx::FromRow, Debug, Clone)]
#[allow(dead_code)]
struct ClaimsResponse {
    id: i64,
    output_merkle_root_hash: Vec<u8>,
    application_address: Vec<u8>,
}
