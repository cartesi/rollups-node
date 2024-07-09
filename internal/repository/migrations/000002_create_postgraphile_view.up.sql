
-- (c) Cartesi and individual authors (see AUTHORS)
-- SPDX-License-Identifier: Apache-2.0 (see LICENSE)

CREATE SCHEMA IF NOT EXISTS graphql;

CREATE OR REPLACE VIEW graphql."applications" AS
    SELECT
        "contract_address",
        "template_hash",
        "snapshot_uri",
        "last_processed_block",
        "epoch_length",
        "status"
    FROM
        "application";

CREATE OR REPLACE VIEW graphql."inputs" AS
    SELECT
        "index",
        "status",
        "block_number",
        "raw_data",
        "machine_hash",
        "outputs_hash",
        "application_address"
    FROM
        "input";

CREATE OR REPLACE VIEW graphql."outputs" AS
    SELECT
        o."index",
        o."raw_data",
        o."output_hashes_siblings",
        i."index" as "input_index"
    FROM
        "output" o
    INNER JOIN
        "input" i on o."input_id"=i."id";

CREATE OR REPLACE VIEW graphql."reports" AS
    SELECT
        r."index",
        r."raw_data",
        i."index" as "input_index"
    FROM
        "report" r
    INNER JOIN
        "input" i on r."input_id"=i."id";

CREATE OR REPLACE VIEW graphql."claims" AS
    SELECT
        c."index",
        c."output_merkle_root_hash",
        c."status",
        c."application_address",
        o."index" as "output_index"
    FROM
        "claim" c
    INNER JOIN
        "application" a ON c."application_address"=a."contract_address"
    INNER JOIN
        "input" i ON a."contract_address"=i."application_address"
    INNER JOIN
        "output" o ON i."id"=o."input_id";

COMMENT ON VIEW graphql."inputs" is
  E'@foreignKey (application_address) references applications(contract_address)|@fieldName applicationByApplicationAddress';

COMMENT ON VIEW graphql."outputs" is
  E'@foreignKey (input_index) references inputs(index)|@fieldName inputByInputIndex';

COMMENT ON VIEW graphql."reports" is
  E'@foreignKey (input_index) references inputs(index)|@fieldName inputByInputIndex';

COMMENT ON VIEW graphql."claims" is
  E'@foreignKey (output_index) references outputs(index)|@fieldName outputByOutputIndex\n@foreignKey (application_address) references applications(contract_address)|@fieldName applicationByApplicationAddress';