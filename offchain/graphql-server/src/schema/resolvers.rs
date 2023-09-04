// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use juniper::{
    graphql_object, DefaultScalarValue, FieldError, FieldResult,
    GraphQLInputObject, GraphQLObject,
};
use std::time::UNIX_EPOCH;

use rollups_data::Repository;
use rollups_data::{
    Connection, Edge, Input, InputQueryFilter, Notice, NoticeQueryFilter,
    OutputEnum, PageInfo as DbPageInfo, Proof, Report, ReportQueryFilter,
    Voucher, VoucherQueryFilter,
};

use super::scalar::RollupsGraphQLScalarValue;

#[derive(Clone)]
pub struct Context {
    repository: Repository,
}

impl Context {
    pub fn new(repository: Repository) -> Self {
        Self { repository }
    }
}

impl juniper::Context for Context {}

pub struct Query;

#[graphql_object(
    context = Context,
    Scalar = RollupsGraphQLScalarValue
    description = "Top level queries"
)]
impl Query {
    #[graphql(description = "Get input based on its identifier")]
    fn input(
        #[graphql(description = "Input index")] index: i32,
    ) -> FieldResult<Input> {
        executor
            .context()
            .repository
            .get_input(index)
            .map_err(convert_error)
    }

    #[graphql(description = "Get voucher based on its index")]
    fn voucher(
        #[graphql(description = "Voucher index in input")] voucher_index: i32,
        #[graphql(description = "Input index")] input_index: i32,
    ) -> FieldResult<Voucher> {
        executor
            .context()
            .repository
            .get_voucher(voucher_index, input_index)
            .map_err(convert_error)
    }

    #[graphql(description = "Get notice based on its index")]
    fn notice(
        #[graphql(description = "Notice index in input")] notice_index: i32,
        #[graphql(description = "Input index")] input_index: i32,
    ) -> FieldResult<Notice> {
        executor
            .context()
            .repository
            .get_notice(notice_index, input_index)
            .map_err(convert_error)
    }

    #[graphql(description = "Get report based on its index")]
    fn report(
        #[graphql(description = "Report index in input")] report_index: i32,
        #[graphql(description = "Input index")] input_index: i32,
    ) -> FieldResult<Report> {
        executor
            .context()
            .repository
            .get_report(report_index, input_index)
            .map_err(convert_error)
    }

    #[graphql(description = "Get inputs with support for pagination")]
    fn inputs(
        #[graphql(
            description = "Get at most the first `n` entries (forward pagination)"
        )]
        first: Option<i32>,
        #[graphql(
            description = "Get at most the last `n` entries (backward pagination)"
        )]
        last: Option<i32>,
        #[graphql(
            description = "Get entries that come after the provided cursor (forward pagination)"
        )]
        after: Option<String>,
        #[graphql(
            description = "Get entries that come before the provided cursor (backward pagination)"
        )]
        before: Option<String>,
        #[graphql(description = "Filter entries to retrieve")] r#where: Option<
            InputFilter,
        >,
    ) -> FieldResult<Connection<Input>> {
        let filter = r#where.map(InputFilter::into).unwrap_or_default();
        executor
            .context()
            .repository
            .get_inputs(first, last, after, before, filter)
            .map_err(convert_error)
    }

    #[graphql(description = "Get vouchers with support for pagination")]
    fn vouchers(
        #[graphql(
            description = "Get at most the first `n` entries (forward pagination)"
        )]
        first: Option<i32>,
        #[graphql(
            description = "Get at most the last `n` entries (backward pagination)"
        )]
        last: Option<i32>,
        #[graphql(
            description = "Get entries that come after the provided cursor (forward pagination)"
        )]
        after: Option<String>,
        #[graphql(
            description = "Get entries that come before the provided cursor (backward pagination)"
        )]
        before: Option<String>,
    ) -> FieldResult<Connection<Voucher>> {
        executor
            .context()
            .repository
            .get_vouchers(first, last, after, before, Default::default())
            .map_err(convert_error)
    }

    #[graphql(description = "Get notices with support for pagination")]
    fn notices(
        #[graphql(
            description = "Get at most the first `n` entries (forward pagination)"
        )]
        first: Option<i32>,
        #[graphql(
            description = "Get at most the last `n` entries (backward pagination)"
        )]
        last: Option<i32>,
        #[graphql(
            description = "Get entries that come after the provided cursor (forward pagination)"
        )]
        after: Option<String>,
        #[graphql(
            description = "Get entries that come before the provided cursor (backward pagination)"
        )]
        before: Option<String>,
    ) -> FieldResult<Connection<Notice>> {
        executor
            .context()
            .repository
            .get_notices(first, last, after, before, Default::default())
            .map_err(convert_error)
    }

    #[graphql(description = "Get reports with support for pagination")]
    fn reports(
        #[graphql(
            description = "Get at most the first `n` entries (forward pagination)"
        )]
        first: Option<i32>,
        #[graphql(
            description = "Get at most the last `n` entries (backward pagination)"
        )]
        last: Option<i32>,
        #[graphql(
            description = "Get entries that come after the provided cursor (forward pagination)"
        )]
        after: Option<String>,
        #[graphql(
            description = "Get entries that come before the provided cursor (backward pagination)"
        )]
        before: Option<String>,
    ) -> FieldResult<Connection<Report>> {
        executor
            .context()
            .repository
            .get_reports(first, last, after, before, Default::default())
            .map_err(convert_error)
    }
}

#[graphql_object(
    context = Context,
    Scalar = RollupsGraphQLScalarValue,
    description = "Request submitted to the application to advance its state"
)]
impl Input {
    #[graphql(description = "Input index starting from genesis")]
    fn index(&self) -> i32 {
        self.index
    }

    #[graphql(description = "Address responsible for submitting the input")]
    fn msg_sender(&self) -> String {
        hex_encode(&self.msg_sender)
    }

    #[graphql(
        description = "Timestamp associated with the input submission, as defined by the base layer's block in which it was recorded"
    )]
    fn timestamp(&self) -> i64 {
        match self.timestamp.duration_since(UNIX_EPOCH) {
            Ok(duration) => duration.as_secs() as i64,
            Err(e) => {
                tracing::warn!("failed to parse timestamp ({})", e);
                0
            }
        }
    }

    #[graphql(
        description = "Number of the base layer block in which the input was recorded"
    )]
    fn block_number(&self) -> i64 {
        self.block_number
    }

    #[graphql(
        description = "Input payload in Ethereum hex binary format, starting with '0x'"
    )]
    fn payload(&self) -> String {
        hex_encode(&self.payload)
    }

    #[graphql(
        description = "Get voucher from this particular input given the voucher's index"
    )]
    fn voucher(
        &self,
        #[graphql(description = "Voucher index in input")] index: i32,
    ) -> FieldResult<Voucher> {
        executor
            .context()
            .repository
            .get_voucher(index, self.index)
            .map_err(convert_error)
    }

    #[graphql(
        description = "Get notice from this particular input given the notice's index"
    )]
    fn notice(
        &self,
        #[graphql(description = "Notice index in input")] index: i32,
    ) -> FieldResult<Notice> {
        executor
            .context()
            .repository
            .get_notice(index, self.index)
            .map_err(convert_error)
    }

    #[graphql(
        description = "Get report from this particular input given the report's index"
    )]
    fn report(
        &self,
        #[graphql(description = "Report index in input")] index: i32,
    ) -> FieldResult<Report> {
        executor
            .context()
            .repository
            .get_report(index, self.index)
            .map_err(convert_error)
    }

    #[graphql(
        description = "Get vouchers from this particular input with support for pagination"
    )]
    fn vouchers(
        &self,
        #[graphql(
            description = "Get at most the first `n` entries (forward pagination)"
        )]
        first: Option<i32>,
        #[graphql(
            description = "Get at most the last `n` entries (backward pagination)"
        )]
        last: Option<i32>,
        #[graphql(
            description = "Get entries that come after the provided cursor (forward pagination)"
        )]
        after: Option<String>,
        #[graphql(
            description = "Get entries that come before the provided cursor (backward pagination)"
        )]
        before: Option<String>,
    ) -> FieldResult<Connection<Voucher>> {
        let filter = VoucherQueryFilter {
            input_index: Some(self.index),
        };
        executor
            .context()
            .repository
            .get_vouchers(first, last, after, before, filter)
            .map_err(convert_error)
    }

    #[graphql(
        description = "Get notices from this particular input with support for pagination"
    )]
    fn notices(
        &self,
        #[graphql(
            description = "Get at most the first `n` entries (forward pagination)"
        )]
        first: Option<i32>,
        #[graphql(
            description = "Get at most the last `n` entries (backward pagination)"
        )]
        last: Option<i32>,
        #[graphql(
            description = "Get entries that come after the provided cursor (forward pagination)"
        )]
        after: Option<String>,
        #[graphql(
            description = "Get entries that come before the provided cursor (backward pagination)"
        )]
        before: Option<String>,
    ) -> FieldResult<Connection<Notice>> {
        let filter = NoticeQueryFilter {
            input_index: Some(self.index),
        };
        executor
            .context()
            .repository
            .get_notices(first, last, after, before, filter)
            .map_err(convert_error)
    }

    #[graphql(
        description = "Get reports from this particular input with support for pagination"
    )]
    fn reports(
        &self,
        #[graphql(
            description = "Get at most the first `n` entries (forward pagination)"
        )]
        first: Option<i32>,
        #[graphql(
            description = "Get at most the last `n` entries (backward pagination)"
        )]
        last: Option<i32>,
        #[graphql(
            description = "Get entries that come after the provided cursor (forward pagination)"
        )]
        after: Option<String>,
        #[graphql(
            description = "Get entries that come before the provided cursor (backward pagination)"
        )]
        before: Option<String>,
    ) -> FieldResult<Connection<Report>> {
        let filter = ReportQueryFilter {
            input_index: Some(self.index),
        };
        executor
            .context()
            .repository
            .get_reports(first, last, after, before, filter)
            .map_err(convert_error)
    }
}

#[graphql_object(
    context = Context,
    Scalar = RollupsGraphQLScalarValue,
    description = "Representation of a transaction that can be carried out on the base layer blockchain, such as a transfer of assets"
)]
impl Voucher {
    #[graphql(
        description = "Voucher index within the context of the input that produced it"
    )]
    fn index(&self) -> i32 {
        self.index
    }

    #[graphql(description = "Input whose processing produced the voucher")]
    fn input(&self) -> FieldResult<Input> {
        executor
            .context()
            .repository
            .get_input(self.input_index)
            .map_err(convert_error)
    }

    #[graphql(
        description = "Transaction destination address in Ethereum hex binary format (20 bytes), starting with '0x'"
    )]
    fn destination(&self) -> String {
        hex_encode(&self.destination)
    }

    #[graphql(
        description = "Transaction payload in Ethereum hex binary format, starting with '0x'"
    )]
    fn payload(&self) -> String {
        hex_encode(&self.payload)
    }

    #[graphql(
        description = "Proof object that allows this voucher to be validated and executed on the base layer blockchain"
    )]
    fn proof(&self) -> FieldResult<Option<Proof>> {
        executor
            .context()
            .repository
            .get_proof(self.input_index, self.index, OutputEnum::Voucher)
            .map_err(convert_error)
    }
}

#[graphql_object(
    context = Context,
    Scalar = RollupsGraphQLScalarValue,
    description = "Informational statement that can be validated in the base layer blockchain"
)]
impl Notice {
    #[graphql(
        description = "Notice index within the context of the input that produced it"
    )]
    fn index(&self) -> i32 {
        self.index
    }

    #[graphql(description = "Input whose processing produced the notice")]
    fn input(&self) -> FieldResult<Input> {
        executor
            .context()
            .repository
            .get_input(self.input_index)
            .map_err(convert_error)
    }

    #[graphql(
        description = "Notice data as a payload in Ethereum hex binary format, starting with '0x'"
    )]
    fn payload(&self) -> String {
        hex_encode(&self.payload)
    }

    #[graphql(
        description = "Proof object that allows this notice to be validated by the base layer blockchain"
    )]
    fn proof(&self) -> FieldResult<Option<Proof>> {
        executor
            .context()
            .repository
            .get_proof(self.input_index, self.index, OutputEnum::Notice)
            .map_err(convert_error)
    }
}

#[graphql_object(
    context = Context,
    Scalar = RollupsGraphQLScalarValue,
    description = "Application log or diagnostic information"
)]
impl Report {
    #[graphql(
        description = "Report index within the context of the input that produced it"
    )]
    fn index(&self) -> i32 {
        self.index
    }

    #[graphql(description = "Input whose processing produced the report")]
    fn input(&self) -> FieldResult<Input> {
        executor
            .context()
            .repository
            .get_input(self.input_index)
            .map_err(convert_error)
    }

    #[graphql(
        description = "Report data as a payload in Ethereum hex binary format, starting with '0x'"
    )]
    fn payload(&self) -> String {
        hex_encode(&self.payload)
    }
}

#[graphql_object(
    context = Context,
    Scalar = RollupsGraphQLScalarValue,
    description = "Data that can be used as proof to validate notices and execute vouchers on the base layer blockchain"
)]
impl Proof {
    #[graphql(description = "Validity proof for an output")]
    fn validity(&self) -> OutputValidityProof {
        OutputValidityProof {
            input_index_within_epoch: self.validity_input_index_within_epoch,
            output_index_within_input: self.validity_output_index_within_input,
            output_hashes_root_hash: hex_encode(
                &self.validity_output_hashes_root_hash,
            ),
            vouchers_epoch_root_hash: hex_encode(
                &self.validity_vouchers_epoch_root_hash,
            ),
            notices_epoch_root_hash: hex_encode(
                &self.validity_notices_epoch_root_hash,
            ),
            machine_state_hash: hex_encode(&self.validity_machine_state_hash),
            output_hash_in_output_hashes_siblings: self
                .validity_output_hash_in_output_hashes_siblings
                .iter()
                .map(|hash| hex_encode(hash.as_ref().unwrap_or(&vec![])))
                .collect(),
            output_hashes_in_epoch_siblings: self
                .validity_output_hashes_in_epoch_siblings
                .iter()
                .map(|hash| hex_encode(hash.as_ref().unwrap_or(&vec![])))
                .collect(),
        }
    }

    #[graphql(
        description = "Data that allows the validity proof to be contextualized within submitted claims, given as a payload in Ethereum hex binary format, starting with '0x'"
    )]
    fn context(&self) -> String {
        hex_encode(&self.context)
    }
}

#[derive(GraphQLObject, Debug, Clone)]
#[graphql(
    description = "Validity proof for an output"
    scalar = RollupsGraphQLScalarValue,
)]
struct OutputValidityProof {
    #[graphql(
        description = "Local input index within the context of the related epoch"
    )]
    pub input_index_within_epoch: i32,

    #[graphql(
        description = "Output index within the context of the input that produced it"
    )]
    pub output_index_within_input: i32,

    #[graphql(
        description = "Merkle root of all output hashes of the related input, given in Ethereum hex binary format (32 bytes), starting with '0x'"
    )]
    pub output_hashes_root_hash: String,

    #[graphql(
        description = "Merkle root of all voucher hashes of the related epoch, given in Ethereum hex binary format (32 bytes), starting with '0x'"
    )]
    pub vouchers_epoch_root_hash: String,

    #[graphql(
        description = "Merkle root of all notice hashes of the related epoch, given in Ethereum hex binary format (32 bytes), starting with '0x'"
    )]
    pub notices_epoch_root_hash: String,

    #[graphql(
        description = "Hash of the machine state claimed for the related epoch, given in Ethereum hex binary format (32 bytes), starting with '0x'"
    )]
    pub machine_state_hash: String,

    #[graphql(
        description = "Proof that this output hash is in the output-hashes merkle tree. This array of siblings is bottom-up ordered (from the leaf to the root). Each hash is given in Ethereum hex binary format (32 bytes), starting with '0x'."
    )]
    pub output_hash_in_output_hashes_siblings: Vec<String>,

    #[graphql(
        description = "Proof that this output-hashes root hash is in epoch's output merkle tree. This array of siblings is bottom-up ordered (from the leaf to the root). Each hash is given in Ethereum hex binary format (32 bytes), starting with '0x'."
    )]
    pub output_hashes_in_epoch_siblings: Vec<String>,
}

#[derive(Debug, Clone, GraphQLInputObject)]
#[graphql(scalar = RollupsGraphQLScalarValue)]
/// Filter object to restrict results depending on input properties
pub struct InputFilter {
    /// Filter only inputs with index lower than a given value
    pub index_lower_than: Option<i32>,

    /// Filter only inputs with index greater than a given value
    pub index_greater_than: Option<i32>,
}

impl From<InputFilter> for InputQueryFilter {
    fn from(filter: InputFilter) -> InputQueryFilter {
        InputQueryFilter {
            index_lower_than: filter.index_lower_than,
            index_greater_than: filter.index_greater_than,
        }
    }
}

#[derive(Debug, Clone, GraphQLObject)]
/// Page metadata for the cursor-based Connection pagination pattern
struct PageInfo {
    /// Cursor pointing to the first entry of the page
    start_cursor: Option<String>,

    /// Cursor pointing to the last entry of the page
    end_cursor: Option<String>,

    /// Indicates if there are additional entries after the end curs
    has_next_page: bool,

    /// Indicates if there are additional entries before the start curs
    has_previous_page: bool,
}

impl From<&DbPageInfo> for PageInfo {
    fn from(page_info: &DbPageInfo) -> PageInfo {
        PageInfo {
            start_cursor: page_info
                .start_cursor
                .as_ref()
                .map(|cursor| cursor.encode()),
            end_cursor: page_info
                .end_cursor
                .as_ref()
                .map(|cursor| cursor.encode()),
            has_next_page: page_info.has_next_page,
            has_previous_page: page_info.has_previous_page,
        }
    }
}

/// Implement the Connection and Edge objects
macro_rules! impl_connection {
    ($connection_name: literal, $edge_name: literal, $node: ty) => {
        #[graphql_object(
                                            name = $connection_name,
                                            context = Context,
                                            Scalar = RollupsGraphQLScalarValue,
                                            description = "Pagination result"
                                        )]
        impl Connection<$node> {
            #[graphql(
                description = "Total number of entries that match the query"
            )]
            fn total_count(&self) -> i32 {
                self.total_count
            }

            #[graphql(
                description = "Pagination entries returned for the current page"
            )]
            fn edges(&self) -> &Vec<Edge<$node>> {
                &self.edges
            }

            #[graphql(description = "Pagination metadata")]
            fn page_info(&self) -> PageInfo {
                (&self.page_info).into()
            }
        }

        #[graphql_object(
                                            name = $edge_name
                                            context = Context,
                                            Scalar = RollupsGraphQLScalarValue,
                                            description = "Pagination entry"
                                        )]
        impl Edge<$node> {
            #[graphql(description = "Node instance")]
            fn node(&self) -> &$node {
                &self.node
            }

            #[graphql(description = "Pagination cursor")]
            fn cursor(&self) -> String {
                self.cursor.encode()
            }
        }
    };
}

impl_connection!("InputConnection", "InputEdge", Input);
impl_connection!("VoucherConnection", "VoucherEdge", Voucher);
impl_connection!("NoticeConnection", "NoticeEdge", Notice);
impl_connection!("ReportConnection", "ReportEdge", Report);

fn convert_error(e: rollups_data::Error) -> FieldError<DefaultScalarValue> {
    tracing::warn!("Got error during query: {:?}", e);
    e.into()
}

pub fn hex_encode(data: &[u8]) -> String {
    format!("0x{}", hex::encode(data))
}
