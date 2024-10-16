
-- (c) Cartesi and individual authors (see AUTHORS)
-- SPDX-License-Identifier: Apache-2.0 (see LICENSE)

CREATE SCHEMA IF NOT EXISTS graphql;

CREATE OR REPLACE VIEW graphql."applications" AS
    SELECT
        "contract_address",
        "template_hash",
        "last_processed_block",
        "status"
    FROM
        "application";

CREATE OR REPLACE VIEW graphql."epochs" AS
    SELECT
        "index",
        "application_address",
        "first_block",
        "last_block",
        "transaction_hash",
        "claim_hash",
        "status"
    FROM
        "epoch";

CREATE OR REPLACE VIEW graphql."inputs" AS
    SELECT
        i."index",
        i."status",
        i."block_number",
        i."raw_data",
        i."machine_hash",
        i."outputs_hash",
        i."application_address",
        i."transaction_id",
        e."index" as "epoch_index"
    FROM
        "input" i
    INNER JOIN
        "epoch" e on i."epoch_id" = e."id";

CREATE OR REPLACE VIEW graphql."outputs" AS
    SELECT
        o."index",
        o."raw_data",
        o."output_hashes_siblings",
        o."transaction_hash",
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

CREATE OR REPLACE VIEW graphql."espresso_nonces" AS
    SELECT
        "sender_address",
        "application_address",
        "nonce"
    FROM
        "espresso_nonce";
        

COMMENT ON VIEW graphql."inputs" is
  E'@foreignKey (application_address) references applications(contract_address)|@fieldName applicationByApplicationAddress\n@foreignKey (epoch_index) references epochs(index)|@fieldName epochByEpochIndex';

COMMENT ON VIEW graphql."outputs" is
  E'@foreignKey (input_index) references inputs(index)|@fieldName inputByInputIndex';

COMMENT ON VIEW graphql."reports" is
  E'@foreignKey (input_index) references inputs(index)|@fieldName inputByInputIndex';

COMMENT ON VIEW graphql."epochs" is
  E'@foreignKey (application_address) references applications(contract_address)|@fieldName applicationByApplicationAddress';
  