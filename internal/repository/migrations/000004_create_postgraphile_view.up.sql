-- (c) Cartesi and individual authors (see AUTHORS)
-- SPDX-License-Identifier: Apache-2.0 (see LICENSE)

CREATE SCHEMA IF NOT EXISTS graphql;

CREATE OR REPLACE VIEW graphql."inputs" AS
    SELECT
        "index",
        "status",
        "block_number",
        "blob"
    FROM
        "inputs";

CREATE OR REPLACE VIEW graphql."outputs" AS
    SELECT
        "index",
        "input_index",
        "blob"
    FROM
        "outputs";

CREATE OR REPLACE VIEW graphql."reports" AS
    SELECT
        "input_index",
        "index",
        "blob"
    FROM
        "reports";

CREATE OR REPLACE VIEW graphql."proofs" AS
    SELECT
        "input_index",
        "input_index_within_epoch",
        "output_index_within_input",
        "output_hashes_root_hash",
        "output_epoch_root_hash",
        "machine_state_hash",
        "output_hash_in_output_hashes_siblings",
        "output_hashes_in_epoch_siblings"
    FROM
        "proofs";

COMMENT ON VIEW graphql."outputs" is
  E'@foreignKey (input_index) references inputs(index)|@fieldName inputByInputIndex';

COMMENT ON VIEW graphql."reports" is
  E'@foreignKey (input_index) references inputs(index)|@fieldName inputByInputIndex';

COMMENT ON VIEW graphql."proofs" is
  E'@foreignKey (input_index) references inputs(index)|@fieldName inputByInputIndex\n@foreignKey (input_index, output_index_within_input) references outputs(input_index,index)|@fieldName proofsByInputIndexAndOutputIndexWithinInput';
