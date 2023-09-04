// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use diesel::deserialize::{self, FromSql, FromSqlRow};
use diesel::pg::{Pg, PgValue};
use diesel::serialize::{self, IsNull, Output, ToSql};
use diesel::{AsExpression, Insertable, Queryable, QueryableByName};
use std::io::Write;

use super::schema::{
    inputs, notices, proofs, reports, sql_types::OutputEnum as SQLOutputEnum,
    vouchers,
};

#[derive(Clone, Debug, Insertable, PartialEq, Queryable, QueryableByName)]
#[diesel(table_name = inputs)]
pub struct Input {
    pub index: i32,
    pub msg_sender: Vec<u8>,
    pub tx_hash: Vec<u8>,
    pub block_number: i64,
    pub timestamp: std::time::SystemTime,
    pub payload: Vec<u8>,
}

#[derive(Clone, Debug, Insertable, PartialEq, Queryable, QueryableByName)]
#[diesel(table_name = notices)]
pub struct Notice {
    pub input_index: i32,
    pub index: i32,
    pub payload: Vec<u8>,
}

#[derive(Clone, Debug, Insertable, PartialEq, Queryable, QueryableByName)]
#[diesel(table_name = vouchers)]
pub struct Voucher {
    pub input_index: i32,
    pub index: i32,
    pub destination: Vec<u8>,
    pub payload: Vec<u8>,
}

#[derive(Clone, Debug, Insertable, PartialEq, Queryable, QueryableByName)]
#[diesel(table_name = reports)]
pub struct Report {
    pub input_index: i32,
    pub index: i32,
    pub payload: Vec<u8>,
}

#[derive(Debug, PartialEq, Eq, Clone, Copy, FromSqlRow, AsExpression)]
#[diesel(sql_type = SQLOutputEnum)]
pub enum OutputEnum {
    Voucher,
    Notice,
}

impl ToSql<SQLOutputEnum, Pg> for OutputEnum {
    fn to_sql<'b>(&'b self, out: &mut Output<'b, '_, Pg>) -> serialize::Result {
        match *self {
            OutputEnum::Voucher => out.write_all(b"voucher")?,
            OutputEnum::Notice => out.write_all(b"notice")?,
        }
        Ok(IsNull::No)
    }
}

impl FromSql<SQLOutputEnum, Pg> for OutputEnum {
    fn from_sql(bytes: PgValue<'_>) -> deserialize::Result<Self> {
        match bytes.as_bytes() {
            b"voucher" => Ok(OutputEnum::Voucher),
            b"notice" => Ok(OutputEnum::Notice),
            _ => Err("Unrecognized enum variant".into()),
        }
    }
}

impl diesel::query_builder::QueryId for SQLOutputEnum {
    type QueryId = SQLOutputEnum;
}

#[derive(Clone, Debug, Insertable, PartialEq, Queryable, QueryableByName)]
#[diesel(table_name = proofs)]
pub struct Proof {
    pub input_index: i32,
    pub output_index: i32,
    pub output_enum: OutputEnum,
    pub validity_input_index_within_epoch: i32,
    pub validity_output_index_within_input: i32,
    pub validity_output_hashes_root_hash: Vec<u8>,
    pub validity_vouchers_epoch_root_hash: Vec<u8>,
    pub validity_notices_epoch_root_hash: Vec<u8>,
    pub validity_machine_state_hash: Vec<u8>,
    pub validity_output_hash_in_output_hashes_siblings: Vec<Option<Vec<u8>>>,
    pub validity_output_hashes_in_epoch_siblings: Vec<Option<Vec<u8>>>,
    pub context: Vec<u8>,
}

#[derive(Debug, Default)]
pub struct InputQueryFilter {
    pub index_greater_than: Option<i32>,
    pub index_lower_than: Option<i32>,
}

macro_rules! decl_output_filter {
    ($name: ident) => {
        #[derive(Debug, Default)]
        pub struct $name {
            pub input_index: Option<i32>,
        }
    };
}

decl_output_filter!(VoucherQueryFilter);
decl_output_filter!(NoticeQueryFilter);
decl_output_filter!(ReportQueryFilter);
