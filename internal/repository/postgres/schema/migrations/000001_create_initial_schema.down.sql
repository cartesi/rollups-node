-- (c) Cartesi and individual authors (see AUTHORS)
-- SPDX-License-Identifier: Apache-2.0 (see LICENSE)

DROP TRIGGER IF EXISTS "node_config_set_updated_at" ON "node_config";
DROP TABLE IF EXISTS "node_config";

DROP TRIGGER IF EXISTS "report_set_updated_at" ON "report";
DROP TABLE IF EXISTS "report";

DROP TRIGGER IF EXISTS "output_set_updated_at" ON "output";
DROP INDEX IF EXISTS "output_raw_data_address_idx";
DROP INDEX IF EXISTS "output_raw_data_type_idx";
DROP TABLE IF EXISTS "output";

DROP TRIGGER IF EXISTS "input_set_updated_at" ON "input";
DROP INDEX IF EXISTS "input_sender_idx";
DROP INDEX IF EXISTS "input_status_idx";
DROP INDEX IF EXISTS "input_block_number_idx";
DROP TABLE IF EXISTS "input";

DROP TRIGGER IF EXISTS "epoch_set_updated_at" ON "epoch";
DROP INDEX IF EXISTS "epoch_status_idx";
DROP INDEX IF EXISTS "epoch_last_block_idx";
DROP TABLE IF EXISTS "epoch";

DROP TRIGGER IF EXISTS "execution_parameters_set_updated_at" ON "execution_parameters";
DROP TABLE IF EXISTS "execution_parameters";

DROP TRIGGER IF EXISTS "application_set_updated_at" ON "application";
DROP TABLE IF EXISTS "application";

DROP FUNCTION IF EXISTS "update_updated_at_column";
DROP FUNCTION IF EXISTS "check_hash_siblings";

DROP TYPE IF EXISTS "SnapshotPolicy";
DROP TYPE IF EXISTS "EpochStatus";
DROP TYPE IF EXISTS "DefaultBlock";
DROP TYPE IF EXISTS "InputCompletionStatus";
DROP TYPE IF EXISTS "ApplicationState";
DROP DOMAIN IF EXISTS "hash";
DROP DOMAIN IF EXISTS "uint64";
DROP DOMAIN IF EXISTS "ethereum_address";
