-- (c) Cartesi and individual authors (see AUTHORS)
-- SPDX-License-Identifier: Apache-2.0 (see LICENSE)

BEGIN;

DROP TRIGGER IF EXISTS "state_hashes_set_updated_at" ON "match_advances";
DROP TABLE IF EXISTS "state_hashes";

DROP TRIGGER IF EXISTS "match_advances_set_updated_at" ON "match_advances";
DROP INDEX IF EXISTS "match_advances_block_number_idx";
DROP TABLE IF EXISTS "match_advances";

ALTER TABLE "tournaments" DROP CONSTRAINT "tournaments_parent_match_fkey";

DROP TRIGGER IF EXISTS "matches_set_updated_at" ON "matches";
DROP INDEX IF EXISTS "matches_unique_pair_idx";
DROP TABLE IF EXISTS "matches";

DROP TRIGGER IF EXISTS "commitments_set_updated_at" ON "commitments";
DROP INDEX IF EXISTS "commitments_final_state_idx";
DROP TABLE IF EXISTS "commitments";

DROP TRIGGER IF EXISTS "tournaments_set_updated_at" ON "tournaments";
DROP INDEX IF EXISTS "tournaments_parent_match_nonroot_idx";
DROP INDEX IF EXISTS "unique_root_per_epoch_idx";
DROP TABLE IF EXISTS "tournaments";

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

DROP TRIGGER IF EXISTS "epoch_status_transition_check" ON "epoch";
DROP TRIGGER IF EXISTS "epoch_set_updated_at" ON "epoch";
DROP INDEX IF EXISTS "epoch_status_idx";
DROP INDEX IF EXISTS "epoch_last_block_idx";
DROP TABLE IF EXISTS "epoch";

DROP FUNCTION IF EXISTS "enforce_epoch_status_transition";

DROP TRIGGER IF EXISTS "execution_parameters_set_updated_at" ON "execution_parameters";
DROP TABLE IF EXISTS "execution_parameters";

DROP TRIGGER IF EXISTS "application_set_updated_at" ON "application";
DROP INDEX IF EXISTS "application_data_availability_selector_idx";
DROP TABLE IF EXISTS "application";

DROP FUNCTION IF EXISTS "update_updated_at_column";
DROP FUNCTION IF EXISTS "check_hash_siblings";

DROP TYPE IF EXISTS "WinnerCommitment";
DROP TYPE IF EXISTS "MatchDeletionReason";
DROP TYPE IF EXISTS "Consensus";
DROP TYPE IF EXISTS "SnapshotPolicy";
DROP TYPE IF EXISTS "EpochStatus";
DROP TYPE IF EXISTS "DefaultBlock";
DROP TYPE IF EXISTS "InputCompletionStatus";
DROP TYPE IF EXISTS "ApplicationHealth";
DROP DOMAIN IF EXISTS "data_availability";
DROP DOMAIN IF EXISTS "hash";
DROP DOMAIN IF EXISTS "uint64";
DROP DOMAIN IF EXISTS "ethereum_address";

COMMIT;
