// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use backoff::ExponentialBackoff;
use diesel::pg::{Pg, PgConnection};
use diesel::r2d2::{ConnectionManager, Pool, PooledConnection};
use diesel::{insert_into, prelude::*};
use snafu::ResultExt;
use std::sync::Arc;

use super::config::RepositoryConfig;
use super::error::{DatabaseConnectionSnafu, DatabaseSnafu, Error};
use super::pagination::{Connection, Pagination};
use super::schema;
use super::types::{
    Input, InputQueryFilter, Notice, NoticeQueryFilter, OutputEnum, Proof,
    Report, ReportQueryFilter, Voucher, VoucherQueryFilter,
};

pub const POOL_CONNECTION_SIZE: u32 = 3;

#[derive(Clone, Debug)]
pub struct Repository {
    // Connection is not thread safe to share between threads, we use connection pool
    db_pool: Arc<Pool<ConnectionManager<PgConnection>>>,
    backoff: ExponentialBackoff,
}

impl Repository {
    /// Create database connection pool, wait until database server is available with backoff strategy
    pub fn new(config: RepositoryConfig) -> Result<Self, Error> {
        let db_pool = backoff::retry(config.backoff.clone(), || {
            tracing::info!(?config, "trying to create db pool for database");
            Pool::builder()
                .max_size(POOL_CONNECTION_SIZE)
                .build(ConnectionManager::<PgConnection>::new(
                    config.endpoint().into_inner(),
                ))
                .map_err(backoff::Error::transient)
        })
        .context(DatabaseConnectionSnafu)?;
        Ok(Self {
            db_pool: Arc::new(db_pool),
            backoff: config.backoff,
        })
    }

    /// Obtain the connection from the connection pool
    fn conn(
        &self,
    ) -> Result<PooledConnection<ConnectionManager<PgConnection>>, Error> {
        backoff::retry(self.backoff.clone(), || {
            self.db_pool.get().map_err(backoff::Error::transient)
        })
        .context(DatabaseConnectionSnafu)
    }
}

/// Basic queries that fetch by primary_key
impl Repository {
    pub fn get_input(&self, index: i32) -> Result<Input, Error> {
        use schema::inputs::dsl;
        let mut conn = self.conn()?;
        dsl::inputs
            .filter(dsl::index.eq(index))
            .load::<Input>(&mut conn)
            .context(DatabaseSnafu)?
            .pop()
            .ok_or(Error::ItemNotFound {
                item_type: "input".to_owned(),
            })
    }

    pub fn get_voucher(
        &self,
        index: i32,
        input_index: i32,
    ) -> Result<Voucher, Error> {
        let mut conn = self.conn()?;
        use schema::vouchers::dsl;
        dsl::vouchers
            .filter(dsl::index.eq(index))
            .filter(dsl::input_index.eq(input_index))
            .load::<Voucher>(&mut conn)
            .context(DatabaseSnafu)?
            .pop()
            .ok_or(Error::ItemNotFound {
                item_type: "voucher".to_owned(),
            })
    }

    pub fn get_notice(
        &self,
        index: i32,
        input_index: i32,
    ) -> Result<Notice, Error> {
        use schema::notices::dsl;
        let mut conn = self.conn()?;
        dsl::notices
            .filter(dsl::index.eq(index))
            .filter(dsl::input_index.eq(input_index))
            .load::<Notice>(&mut conn)
            .context(DatabaseSnafu)?
            .pop()
            .ok_or(Error::ItemNotFound {
                item_type: "notice".to_owned(),
            })
    }

    pub fn get_report(
        &self,
        index: i32,
        input_index: i32,
    ) -> Result<Report, Error> {
        use schema::reports::dsl;
        let mut conn = self.conn()?;
        dsl::reports
            .filter(dsl::index.eq(index))
            .filter(dsl::input_index.eq(input_index))
            .load::<Report>(&mut conn)
            .context(DatabaseSnafu)?
            .pop()
            .ok_or(Error::ItemNotFound {
                item_type: "report".to_owned(),
            })
    }

    pub fn get_proof(
        &self,
        input_index: i32,
        output_index: i32,
        output_enum: OutputEnum,
    ) -> Result<Option<Proof>, Error> {
        use schema::proofs::dsl;
        let mut conn = self.conn()?;
        dsl::proofs
            .filter(dsl::input_index.eq(input_index))
            .filter(dsl::output_index.eq(output_index))
            .filter(dsl::output_enum.eq(output_enum))
            .load::<Proof>(&mut conn)
            .map(|mut proofs| proofs.pop())
            .context(DatabaseSnafu)
    }
}

/// Basic queries to insert rollups' outputs
impl Repository {
    pub fn insert_input(&self, input: Input) -> Result<(), Error> {
        use schema::inputs;
        let mut conn = self.conn()?;
        insert_into(inputs::table)
            .values(&input)
            .on_conflict_do_nothing()
            .execute(&mut conn)
            .context(DatabaseSnafu)?;
        tracing::trace!("Input {} was written to the db", input.index);
        Ok(())
    }

    pub fn insert_notice(&self, notice: Notice) -> Result<(), Error> {
        use schema::notices;
        let mut conn = self.conn()?;
        insert_into(notices::table)
            .values(&notice)
            .on_conflict_do_nothing()
            .execute(&mut conn)
            .context(DatabaseSnafu)?;
        tracing::trace!(
            "Notice {} from Input {} was written to the db",
            notice.index,
            notice.input_index
        );
        Ok(())
    }

    pub fn insert_voucher(&self, voucher: Voucher) -> Result<(), Error> {
        use schema::vouchers;
        let mut conn = self.conn()?;
        insert_into(vouchers::table)
            .values(&voucher)
            .on_conflict_do_nothing()
            .execute(&mut conn)
            .context(DatabaseSnafu)?;
        tracing::trace!(
            "Voucher {} from Input {} was written to the db",
            voucher.index,
            voucher.input_index
        );
        Ok(())
    }

    pub fn insert_report(&self, report: Report) -> Result<(), Error> {
        use schema::reports;
        let mut conn = self.conn()?;
        insert_into(reports::table)
            .values(&report)
            .on_conflict_do_nothing()
            .execute(&mut conn)
            .context(DatabaseSnafu)?;
        tracing::trace!(
            "Report {} from Input {} was written to the db",
            report.index,
            report.input_index
        );
        Ok(())
    }

    pub fn insert_proof(&self, proof: Proof) -> Result<(), Error> {
        use schema::proofs;
        let mut conn = self.conn()?;
        insert_into(proofs::table)
            .values(&proof)
            .on_conflict_do_nothing()
            .execute(&mut conn)
            .context(DatabaseSnafu)?;
        tracing::trace!(
            "Proof for {:?} {} of Input {} was written to the db",
            proof.output_enum,
            proof.output_index,
            proof.input_index
        );
        Ok(())
    }
}

/// Generate a boxed query from an input query filter
impl InputQueryFilter {
    fn to_query(&self) -> schema::inputs::BoxedQuery<'_, Pg> {
        use schema::inputs::dsl;
        let mut query = dsl::inputs.into_boxed();
        if let Some(other) = self.index_greater_than {
            query = query.filter(dsl::index.gt(other));
        }
        if let Some(other) = self.index_lower_than {
            query = query.filter(dsl::index.lt(other));
        }
        query
    }
}

/// Generate a boxed query from an output query filter
macro_rules! impl_output_filter_to_query {
    ($filter: ty, $table: ident) => {
        impl $filter {
            fn to_query(&self) -> schema::$table::BoxedQuery<'_, Pg> {
                use schema::$table::dsl;
                let mut query = dsl::$table.into_boxed();
                if let Some(other) = self.input_index {
                    query = query.filter(dsl::input_index.eq(other));
                }
                query
            }
        }
    };
}

impl_output_filter_to_query!(VoucherQueryFilter, vouchers);
impl_output_filter_to_query!(NoticeQueryFilter, notices);
impl_output_filter_to_query!(ReportQueryFilter, reports);

/// Implement a paginated query for the given table
macro_rules! impl_paginated_query {
    ($query: ident, $table: ident, $node: ty, $filter: ty) => {
        impl Repository {
            pub fn $query(
                &self,
                first: Option<i32>,
                last: Option<i32>,
                after: Option<String>,
                before: Option<String>,
                filter: $filter,
            ) -> Result<Connection<$node>, Error> {
                let mut conn = self.conn()?;
                let query = filter.to_query().count();
                let count = query
                    .get_result::<i64>(&mut conn)
                    .context(DatabaseSnafu)?;
                let pagination =
                    Pagination::new(first, last, after, before, count as i32)?;
                let nodes = if pagination.limit() > 0 {
                    let query = filter
                        .to_query()
                        .limit(pagination.limit().into())
                        .offset(pagination.offset().into())
                        .order(schema::$table::dsl::$table.primary_key());
                    query.load(&mut conn).context(DatabaseSnafu)?
                } else {
                    vec![]
                };
                Ok(pagination.create_connection(nodes))
            }
        }
    };
}

impl_paginated_query!(get_inputs, inputs, Input, InputQueryFilter);
impl_paginated_query!(get_vouchers, vouchers, Voucher, VoucherQueryFilter);
impl_paginated_query!(get_notices, notices, Notice, NoticeQueryFilter);
impl_paginated_query!(get_reports, reports, Report, ReportQueryFilter);
