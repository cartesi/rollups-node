-- (c) Cartesi and individual authors (see AUTHORS)
-- SPDX-License-Identifier: Apache-2.0 (see LICENSE)

BEGIN;

CREATE DOMAIN "ethereum_address" AS BYTEA CHECK (octet_length(VALUE) = 20);
CREATE DOMAIN "uint64" AS NUMERIC(20, 0) CHECK (VALUE >= 0 AND VALUE <= 18446744073709551615);
CREATE DOMAIN "hash" AS BYTEA CHECK (octet_length(VALUE) = 32);
CREATE DOMAIN "data_availability" AS BYTEA CHECK (octet_length(VALUE) >= 4);

CREATE TYPE "ApplicationState" AS ENUM ('ENABLED', 'DISABLED', 'FAILED', 'INOPERABLE');

CREATE TYPE "InputCompletionStatus" AS ENUM (
    'NONE',
    'ACCEPTED',
    'REJECTED',
    'EXCEPTION',
    'MACHINE_HALTED',
    'OUTPUTS_LIMIT_EXCEEDED',
    'REPORTS_LIMIT_EXCEEDED',
    'CYCLE_LIMIT_EXCEEDED',
    'TIME_LIMIT_EXCEEDED',
    'PAYLOAD_LENGTH_LIMIT_EXCEEDED');

CREATE TYPE "DefaultBlock" AS ENUM ('FINALIZED', 'LATEST', 'PENDING', 'SAFE');

CREATE TYPE "EpochStatus" AS ENUM (
    'OPEN',
    'CLOSED',
    'INPUTS_PROCESSED',
    'CLAIM_COMPUTED',
    'CLAIM_SUBMITTED',
    'CLAIM_ACCEPTED',
    'CLAIM_REJECTED');

CREATE TYPE "SnapshotPolicy" AS ENUM ('NONE', 'EVERY_INPUT', 'EVERY_EPOCH');

CREATE TYPE "Consensus" AS ENUM ('AUTHORITY', 'QUORUM', 'PRT');

CREATE TYPE "MatchDeletionReason" AS ENUM ('STEP', 'TIMEOUT', 'CHILD_TOURNAMENT', 'NOT_DELETED');

CREATE TYPE "WinnerCommitment" AS ENUM ('NONE', 'ONE', 'TWO');

CREATE FUNCTION "update_updated_at_column"()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION check_hash_siblings(arr BYTEA[])
RETURNS BOOLEAN AS $$
DECLARE
    elem BYTEA;
BEGIN
    IF arr IS NULL THEN
        RETURN TRUE; -- NULL array is allowed
    END IF;

    FOREACH elem IN ARRAY arr
    LOOP
        IF octet_length(elem) <> 32 THEN
            RETURN FALSE; -- any element not 32 bytes => fail
        END IF;
    END LOOP;

    RETURN TRUE;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE TABLE "application"
(
    "id" INT GENERATED ALWAYS AS IDENTITY,
    "name" VARCHAR(4096) UNIQUE NOT NULL CHECK ("name" ~ '^[a-z0-9_-]+$'),
    "iapplication_address" ethereum_address UNIQUE NOT NULL,
    "iconsensus_address" ethereum_address NOT NULL,
    "iinputbox_address" ethereum_address NOT NULL,
    "iinputbox_block" uint64 NOT NULL,
    "template_hash" hash NOT NULL,
    "template_uri" VARCHAR(4096) NOT NULL,
    "epoch_length" uint64 NOT NULL,
    "data_availability" data_availability NOT NULL,
    "consensus_type" "Consensus" NOT NULL,
    "state" "ApplicationState" NOT NULL,
    "reason" VARCHAR(4096),
    "last_epoch_check_block" uint64 NOT NULL,
    "last_input_check_block" uint64 NOT NULL,
    "last_output_check_block" uint64 NOT NULL,
    "last_submitted_claim_check_block" uint64 NOT NULL,
    "last_accepted_claim_check_block" uint64 NOT NULL,
    "last_tournament_check_block" uint64 NOT NULL,
    "processed_inputs" uint64 NOT NULL DEFAULT 0,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "reason_required_for_failure_states" CHECK (NOT ("state" IN ('FAILED', 'INOPERABLE') AND ("reason" IS NULL OR LENGTH("reason") = 0))),
    CONSTRAINT "application_pkey" PRIMARY KEY ("id")
);

CREATE INDEX "application_data_availability_selector_idx" ON "application"(substring("data_availability" FROM 1 for 4));

CREATE TRIGGER "application_set_updated_at" BEFORE UPDATE ON "application"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE OR REPLACE FUNCTION validate_application_state_transition()
RETURNS TRIGGER AS $$
BEGIN
    -- INOPERABLE is terminal: no state or reason changes allowed
    IF OLD.state = 'INOPERABLE'::"ApplicationState"
       AND (NEW.state <> OLD.state OR NEW.reason IS DISTINCT FROM OLD.reason)
    THEN
        RAISE EXCEPTION 'cannot change state or reason of an INOPERABLE application';
    END IF;

    -- DISABLED cannot transition to FAILED (app must be running to fail)
    IF OLD.state = 'DISABLED'::"ApplicationState" AND NEW.state = 'FAILED'::"ApplicationState" THEN
        RAISE EXCEPTION 'cannot transition from DISABLED to FAILED: application is not running';
    END IF;

    -- Clear stale reason when transitioning to ENABLED or DISABLED
    IF NEW.state IN ('ENABLED'::"ApplicationState", 'DISABLED'::"ApplicationState") THEN
        NEW.reason := NULL;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER "application_validate_state_transition" BEFORE UPDATE ON "application"
FOR EACH ROW EXECUTE FUNCTION validate_application_state_transition();

CREATE TABLE "execution_parameters" (
    "application_id" INT PRIMARY KEY,
    "snapshot_policy" "SnapshotPolicy" NOT NULL DEFAULT 'NONE',
    "advance_inc_cycles" BIGINT NOT NULL CHECK ("advance_inc_cycles" > 0) DEFAULT 4194304, -- 1 << 22
    "advance_max_cycles" BIGINT NOT NULL CHECK ("advance_max_cycles" > 0) DEFAULT 4611686018427387903, -- uint64 max >> 2
    "inspect_inc_cycles" BIGINT NOT NULL CHECK ("inspect_inc_cycles" > 0) DEFAULT 4194304, -- 1 << 22
    "inspect_max_cycles" BIGINT NOT NULL CHECK ("inspect_max_cycles" > 0) DEFAULT 4611686018427387903,
    "advance_inc_deadline" BIGINT NOT NULL CHECK ("advance_inc_deadline" > 0) DEFAULT 10000000000, -- 10s
    "advance_max_deadline" BIGINT NOT NULL CHECK ("advance_max_deadline" > 0) DEFAULT 180000000000, -- 180s
    "inspect_inc_deadline" BIGINT NOT NULL CHECK ("inspect_inc_deadline" > 0) DEFAULT 10000000000, --10s
    "inspect_max_deadline" BIGINT NOT NULL CHECK ("inspect_max_deadline" > 0) DEFAULT 180000000000, -- 180s
    "load_deadline" BIGINT NOT NULL CHECK ("load_deadline" > 0) DEFAULT 300000000000, -- 300s
    "store_deadline" BIGINT NOT NULL CHECK ("store_deadline" > 0) DEFAULT 180000000000, -- 180s
    "fast_deadline" BIGINT NOT NULL CHECK ("fast_deadline" > 0) DEFAULT 5000000000, -- 5s
    "max_concurrent_inspects" INT NOT NULL CHECK ("max_concurrent_inspects" > 0) DEFAULT 10,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "execution_parameters_application_id_fkey" FOREIGN KEY ("application_id") REFERENCES "application"("id") ON DELETE CASCADE
);

CREATE TRIGGER "execution_parameters_set_updated_at" BEFORE UPDATE ON "execution_parameters"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE "epoch"
(
    "application_id" int4 NOT NULL,
    "index" uint64 NOT NULL,
    "first_block" uint64 NOT NULL,
    "last_block" uint64 NOT NULL,
    "input_index_lower_bound" uint64 NOT NULL,
    "input_index_upper_bound" uint64 NOT NULL,
    "machine_hash" hash,
    "outputs_merkle_root" hash,
    "outputs_merkle_proof" BYTEA[],
    "commitment" hash,
    "commitment_proof" BYTEA[],
    "tournament_address" ethereum_address,
    "claim_submitted_transaction_hash" hash,
    "claim_accepted_transaction_hash" hash,
    "claim_transaction_hash" hash,
    "status" "EpochStatus" NOT NULL,
    "virtual_index" uint64 NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "epoch_pkey" PRIMARY KEY ("application_id", "index"),
    CONSTRAINT "epoch_application_id_virtual_index_unique" UNIQUE ("application_id", "virtual_index"),
    CONSTRAINT "epoch_application_id_fkey" FOREIGN KEY ("application_id") REFERENCES "application"("id") ON DELETE CASCADE,
    CONSTRAINT "epoch_block_bounds_check" CHECK ("first_block" <= "last_block"),
    CONSTRAINT "epoch_input_bounds_check" CHECK ("input_index_lower_bound" <= "input_index_upper_bound")
);

CREATE INDEX "epoch_last_block_idx" ON "epoch"("application_id", "last_block");
CREATE INDEX "epoch_status_idx" ON "epoch"("application_id", "status");

CREATE TRIGGER "epoch_set_updated_at" BEFORE UPDATE ON "epoch"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Enforce valid epoch status transitions.
-- The state machine is:
--   OPEN → CLOSED → INPUTS_PROCESSED → CLAIM_COMPUTED
--   CLAIM_COMPUTED → CLAIM_SUBMITTED → CLAIM_ACCEPTED
--   CLAIM_COMPUTED → CLAIM_ACCEPTED  (PRT skips SUBMITTED; also valid when
--                                     syncing from scratch and the claim was
--                                     already accepted, or in reader-only mode
--                                     with tx submission disabled)
--   CLAIM_COMPUTED  → CLAIM_REJECTED (claim rejected on-chain before the node
--                                     submits, e.g. a conflicting claim was
--                                     already accepted)
--   CLAIM_SUBMITTED → CLAIM_REJECTED
-- Any other transition (including backwards) is rejected.
-- Same-status updates are allowed (idempotent no-ops).
--
-- When transitioning to CLAIM_COMPUTED, the trigger also verifies that
-- required proof fields are populated:
--   All apps:          machine_hash, outputs_merkle_root, outputs_merkle_proof
--   PRT (DaveConsensus): additionally commitment, commitment_proof
CREATE FUNCTION enforce_epoch_status_transition() RETURNS trigger AS $$
DECLARE
    valid_transitions text[][] := ARRAY[
        ARRAY['OPEN',             'CLOSED'],
        ARRAY['CLOSED',           'INPUTS_PROCESSED'],
        ARRAY['INPUTS_PROCESSED', 'CLAIM_COMPUTED'],
        ARRAY['CLAIM_COMPUTED',   'CLAIM_SUBMITTED'],
        ARRAY['CLAIM_COMPUTED',   'CLAIM_ACCEPTED'],
        ARRAY['CLAIM_COMPUTED',   'CLAIM_REJECTED'],
        ARRAY['CLAIM_SUBMITTED',  'CLAIM_ACCEPTED'],
        ARRAY['CLAIM_SUBMITTED',  'CLAIM_REJECTED']
    ];
    is_valid boolean := false;
    app_consensus text;
BEGIN
    IF OLD.status = NEW.status THEN
        RETURN NEW;
    END IF;
    FOR i IN 1..array_length(valid_transitions, 1) LOOP
        IF OLD.status::text = valid_transitions[i][1]
           AND NEW.status::text = valid_transitions[i][2] THEN
            is_valid := true;
            EXIT;
        END IF;
    END LOOP;
    IF NOT is_valid THEN
        RAISE EXCEPTION 'invalid epoch status transition: % -> %',
            OLD.status, NEW.status;
    END IF;

    -- Enforce required fields when entering CLAIM_COMPUTED.
    IF NEW.status::text = 'CLAIM_COMPUTED' THEN
        IF NEW.machine_hash IS NULL
           OR NEW.outputs_merkle_root IS NULL
           OR NEW.outputs_merkle_proof IS NULL THEN
            RAISE EXCEPTION
                'CLAIM_COMPUTED requires machine_hash, outputs_merkle_root, '
                'and outputs_merkle_proof to be non-null';
        END IF;

        SELECT a.consensus_type::text INTO app_consensus
          FROM application a
         WHERE a.id = NEW.application_id;

        IF app_consensus = 'PRT' THEN
            IF NEW.commitment IS NULL
               OR NEW.commitment_proof IS NULL THEN
                RAISE EXCEPTION
                    'CLAIM_COMPUTED for PRT apps requires commitment '
                    'and commitment_proof to be non-null';
            END IF;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER "epoch_status_transition_check"
    BEFORE UPDATE OF "status" ON "epoch"
    FOR EACH ROW
    EXECUTE FUNCTION enforce_epoch_status_transition();

CREATE TABLE "input"
(
    "epoch_application_id" int4 NOT NULL,
    "epoch_index" uint64 NOT NULL,
    "index" uint64 NOT NULL,
    "block_number" uint64 NOT NULL,
    "raw_data" BYTEA NOT NULL,
    "status" "InputCompletionStatus" NOT NULL,
    "machine_hash" hash,
    "outputs_hash" hash,
    "transaction_reference" hash,
    "snapshot_uri" VARCHAR(4096),
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "input_pkey" PRIMARY KEY ("epoch_application_id", "index"),
    CONSTRAINT "input_epoch_index_unique" UNIQUE ("epoch_application_id", "epoch_index", "index"),
    CONSTRAINT "input_application_id_tx_reference_unique" UNIQUE ("epoch_application_id", "transaction_reference"),
    CONSTRAINT "input_epoch_id_fkey" FOREIGN KEY ("epoch_application_id", "epoch_index") REFERENCES "epoch"("application_id", "index") ON DELETE CASCADE
);

CREATE INDEX "input_block_number_idx" ON "input"("epoch_application_id", "block_number");
CREATE INDEX "input_status_idx" ON "input"("epoch_application_id", "status");

CREATE INDEX "input_sender_idx" ON "input" ("epoch_application_id", substring("raw_data" FROM 81 FOR 20));

CREATE TRIGGER "input_set_updated_at" BEFORE UPDATE ON "input"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE "output"
(
    "input_epoch_application_id" int4 NOT NULL,
    "input_index" uint64 NOT NULL,
    "index" uint64 NOT NULL,
    "raw_data" BYTEA NOT NULL,
    "hash" hash,
    "output_hashes_siblings" BYTEA[],
    "execution_transaction_hash" hash,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "output_pkey" PRIMARY KEY ("input_epoch_application_id", "index"),
    CONSTRAINT "output_input_id_fkey" FOREIGN KEY ("input_epoch_application_id",  "input_index") REFERENCES "input"("epoch_application_id", "index") ON DELETE CASCADE,
    CONSTRAINT "output_hashes_siblings_length_check" CHECK (check_hash_siblings("output_hashes_siblings"))
);

CREATE INDEX "output_raw_data_type_idx" ON "output" ("input_epoch_application_id", substring("raw_data" FROM 1 FOR 4));

CREATE INDEX "output_raw_data_address_idx" ON "output" ("input_epoch_application_id", substring("raw_data" FROM 17 FOR 20))
WHERE SUBSTRING("raw_data" FROM 1 FOR 4) IN (
    E'\\x10321e8b',  -- DelegateCallVoucher
    E'\\x237a816f'   -- Voucher
);

CREATE TRIGGER "output_set_updated_at" BEFORE UPDATE ON "output"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE "report"
(
    "input_epoch_application_id" int4 NOT NULL,
    "input_index" uint64 NOT NULL,
    "index" uint64 NOT NULL,
    "raw_data" BYTEA NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "report_pkey" PRIMARY KEY ("input_epoch_application_id", "index"),
    CONSTRAINT "report_input_id_fkey" FOREIGN KEY ("input_epoch_application_id", "input_index") REFERENCES "input"("epoch_application_id", "index") ON DELETE CASCADE
);

CREATE TRIGGER "report_set_updated_at" BEFORE UPDATE ON "report"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE "node_config"
(
    "key" VARCHAR(255) PRIMARY KEY,
    "value" jsonb NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER "config_set_updated_at" BEFORE UPDATE ON "node_config"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE "tournaments"
(
    "application_id" INT4 NOT NULL,
    "epoch_index" uint64 NOT NULL,
    "address" ethereum_address NOT NULL,
    "parent_tournament_address" ethereum_address,
    "parent_match_id_hash" hash,
    "max_level" INT NOT NULL CHECK("max_level" >= 0),
    "level" INT NOT NULL CHECK("level" >= 0),
    "log2step" INT NOT NULL CHECK("log2step" >= 0),
    "height" INT NOT NULL CHECK("height" >= 0),
    "winner_commitment" hash,
    "final_state_hash" hash,
    "finished_at_block" uint64 DEFAULT 0,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "tournaments_pkey" PRIMARY KEY ("application_id","epoch_index","address"),
    CONSTRAINT "tournaments_epoch_fkey"    FOREIGN KEY ("application_id","epoch_index")
      REFERENCES "epoch"("application_id","index")
      ON DELETE CASCADE,
    CONSTRAINT "chk_tournament_root_parent"
      CHECK (
        ("level" = 0 AND "parent_tournament_address" IS NULL AND "parent_match_id_hash" IS NULL)
        OR
        ("level" > 0 AND "parent_tournament_address" IS NOT NULL AND "parent_match_id_hash" IS NOT NULL)
      ),
    CONSTRAINT "tournaments_max_level_gte_level_check" CHECK ("max_level" >= "level")
);

CREATE UNIQUE INDEX "unique_root_per_epoch_idx"
  ON "tournaments"("application_id","epoch_index")
  WHERE "level" = 0;

CREATE INDEX "tournaments_parent_match_nonroot_idx"
  ON "tournaments"("application_id","epoch_index","parent_tournament_address","parent_match_id_hash")
  WHERE "level" > 0;

CREATE TRIGGER "tournaments_set_updated_at"
BEFORE UPDATE ON "tournaments"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE "commitments"
(
    "application_id" INT4 NOT NULL,
    "epoch_index" uint64 NOT NULL,
    "tournament_address" ethereum_address NOT NULL,
    "commitment" hash NOT NULL,
    "final_state_hash" hash NOT NULL,
    "submitter_address" ethereum_address NOT NULL,
    "block_number" uint64 NOT NULL,
    "tx_hash" hash NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "commitments_pkey"
      PRIMARY KEY ("application_id","epoch_index","tournament_address","commitment"),
    CONSTRAINT "commitments_tournament_fkey"
      FOREIGN KEY ("application_id","epoch_index","tournament_address")
      REFERENCES "tournaments"("application_id","epoch_index","address")
      ON DELETE CASCADE
);

CREATE INDEX "commitments_final_state_idx"
  ON "commitments"("final_state_hash");

CREATE TRIGGER "commitments_set_updated_at"
BEFORE UPDATE ON "commitments"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE "matches"
(
    "application_id" INT4 NOT NULL,
    "epoch_index" uint64 NOT NULL,
    "tournament_address" ethereum_address NOT NULL,
    "id_hash" hash NOT NULL,
    "commitment_one" hash NOT NULL,
    "commitment_two" hash NOT NULL,
    "left_of_two" hash NOT NULL,
    "block_number" uint64 NOT NULL,
    "tx_hash" hash NOT NULL,
    "winner" "WinnerCommitment" NOT NULL,
    "deletion_reason" "MatchDeletionReason" NOT NULL,
    "deletion_block_number" uint64 DEFAULT 0,
    "deletion_tx_hash" hash,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "matches_pkey"
      PRIMARY KEY ("application_id","epoch_index","tournament_address","id_hash"),

    CONSTRAINT "matches_tournament_fkey"
      FOREIGN KEY ("application_id","epoch_index","tournament_address")
      REFERENCES "tournaments"("application_id","epoch_index","address")
      ON DELETE CASCADE,

    CONSTRAINT "matches_one_commitment_fkey"
      FOREIGN KEY ("application_id","epoch_index","tournament_address","commitment_one")
      REFERENCES "commitments"("application_id","epoch_index","tournament_address","commitment")
      ON DELETE CASCADE,

    CONSTRAINT "matches_two_commitment_fkey"
      FOREIGN KEY ("application_id","epoch_index","tournament_address","commitment_two")
      REFERENCES "commitments"("application_id","epoch_index","tournament_address","commitment")
      ON DELETE CASCADE
);

CREATE UNIQUE INDEX "matches_unique_pair_idx"
  ON "matches"("application_id","epoch_index","tournament_address","commitment_one","commitment_two");

CREATE TRIGGER "matches_set_updated_at"
BEFORE UPDATE ON "matches"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add foreign key from tournaments to matches (parent match)
ALTER TABLE "tournaments"
  ADD CONSTRAINT "tournaments_parent_match_fkey"
  FOREIGN KEY ("application_id","epoch_index","parent_tournament_address","parent_match_id_hash")
  REFERENCES "matches"("application_id","epoch_index","tournament_address","id_hash")
  ON DELETE CASCADE;

CREATE TABLE "match_advances"
(
    "application_id" INT4 NOT NULL,
    "epoch_index" uint64 NOT NULL,
    "tournament_address" ethereum_address NOT NULL,
    "id_hash" hash NOT NULL,   -- keccak256(abi.encode(one,two))
    "other_parent" hash NOT NULL,
    "left_node" hash NOT NULL,
    "block_number" uint64 NOT NULL,
    "tx_hash" hash NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "match_advances_pkey"
      PRIMARY KEY ("application_id","epoch_index","tournament_address","id_hash","other_parent"),

    CONSTRAINT "match_advances_matches_fkey"
      FOREIGN KEY ("application_id","epoch_index","tournament_address","id_hash")
      REFERENCES "matches"("application_id","epoch_index","tournament_address","id_hash")
      ON DELETE CASCADE
);

CREATE INDEX "match_advances_block_number_idx"
  ON "match_advances"("application_id","epoch_index","tournament_address","id_hash","block_number");

CREATE TRIGGER "match_advances_set_updated_at"
BEFORE UPDATE ON "match_advances"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE "state_hashes"
(
    "input_epoch_application_id" int4 NOT NULL,
    "epoch_index" uint64 NOT NULL,
    "input_index" uint64 NOT NULL,
    "index" uint64 NOT NULL,
    "machine_hash" hash NOT NULL,
    "repetitions" INT8 NOT NULL CHECK ("repetitions" > 0),
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "state_hashes_pkey" PRIMARY KEY ("input_epoch_application_id", "epoch_index", "index"),
    CONSTRAINT "state_hashes_input_id_fkey" FOREIGN KEY ("input_epoch_application_id", "epoch_index", "input_index") REFERENCES "input"("epoch_application_id", "epoch_index", "index") ON DELETE CASCADE
);

CREATE TRIGGER "state_hashes_set_updated_at" BEFORE UPDATE ON "state_hashes"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMIT;

