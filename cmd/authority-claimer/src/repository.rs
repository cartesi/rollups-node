// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use sea_query::{Expr, Iden, Order, PostgresQueryBuilder, Query};
use sea_query_binder::SqlxBinder;
use snafu::{ResultExt, Snafu};
use sqlx::{pool::PoolConnection, PgPool, Pool, Postgres};
use std::sync::Arc;
use std::{fmt::Debug, num::TryFromIntError};

use crate::rollups_events::{Address, HexArrayError, RollupsClaim};

/// The `Repository` queries the database and gets an unsubmitted claim
#[async_trait]
pub trait Repository: Debug {
    type Error: snafu::Error + 'static;

    async fn get_claim(&mut self) -> Result<RollupsClaim, Self::Error>;

    async fn update_claim(&mut self, index: Address)
        -> Result<(), Self::Error>;
}

#[derive(Debug, Snafu)]
#[snafu(visibility(pub(super)))]
pub enum RepositoryError {
    #[snafu(display("database error"))]
    DatabaseError { source: sqlx::Error },

    #[snafu(display("hex conversion error"))]
    HexArrayError { source: HexArrayError },

    #[snafu(display("int conversion error"))]
    TryFromIntError { source: TryFromIntError },
}

#[derive(Clone, Debug)]
pub struct DefaultRepository {
    // Connection is not thread safe to share between threads, we use connection pool
    db_pool: Arc<Pool<Postgres>>,
}

impl DefaultRepository {
    /// Create database connection pool, wait until database server is available with backoff strategy
    pub async fn new(endpoint: String) -> Result<Self, RepositoryError> {
        let connection =
            PgPool::connect(&endpoint).await.context(DatabaseSnafu)?;
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
        let mut conn = self.conn();
        let (sql, values) = Query::select()
            .columns([
                Claims::Epoch,
                Claims::FirstInputIndex,
                Claims::LastInputIndex,
                Claims::EpochHash,
                Claims::ApplicationAddress,
            ])
            .from(Claims::Table)
            .order_by(Claims::Id, Order::Desc)
            .limit(1)
            .build_sqlx(PostgresQueryBuilder);

        let row =
            sqlx::query_as_with::<_, ClaimsStruct, _>(&sql, values.clone())
                .fetch_one(&mut *conn)
                .await
                .context(DatabaseSnafu)?;

        let claim = RollupsClaim {
            dapp_address: row
                .application_address
                .try_into()
                .context(HexArraySnafu)?,
            epoch_index: row.epoch.try_into().context(TryFromIntSnafu)?,
            epoch_hash: row.epoch_hash.try_into().context(HexArraySnafu)?,
            first_index: row
                .first_input_index
                .try_into()
                .context(TryFromIntSnafu)?,
            last_index: row
                .last_input_index
                .try_into()
                .context(TryFromIntSnafu)?,
        };

        Ok(claim)
    }

    async fn update_claim(
        &mut self,
        dapp_address: Address,
    ) -> Result<(), Self::Error> {
        let mut conn = self.conn();
        let a = hex::encode(dapp_address.inner());
        let (sql, values) = Query::update()
            .table(Claims::Table)
            .values([(Claims::Status, "SUBMITTED".into())])
            .and_where(Expr::col(Claims::ApplicationAddress).eq(a))
            .build_sqlx(PostgresQueryBuilder);

        let _result = sqlx::query_with(&sql, values)
            .execute(&mut *conn)
            .await
            .context(DatabaseSnafu)?;
        Ok(())
    }
}

#[derive(Iden)]
enum Claims {
    Table,
    Id,
    Epoch,
    FirstInputIndex,
    LastInputIndex,
    EpochHash,
    ApplicationAddress,
    Status,
}

#[derive(sqlx::FromRow, Debug, Clone)]
#[allow(dead_code)]
struct ClaimsStruct {
    id: i32,
    epoch: i32,
    first_input_index: i32,
    last_input_index: i32,
    epoch_hash: String,
    application_address: String,
    status: String,
}
