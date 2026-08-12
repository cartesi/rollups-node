-- (c) Cartesi and individual authors (see AUTHORS)
-- SPDX-License-Identifier: Apache-2.0 (see LICENSE)

BEGIN;

CREATE DOMAIN "ethereum_address" AS BYTEA CHECK (octet_length(VALUE) = 20);
CREATE DOMAIN "uint64" AS NUMERIC(20, 0) CHECK (VALUE >= 0 AND VALUE <= 18446744073709551615);
CREATE DOMAIN "hash" AS BYTEA CHECK (octet_length(VALUE) = 32);
CREATE DOMAIN "data_availability" AS BYTEA CHECK (octet_length(VALUE) >= 4);

CREATE TYPE "ApplicationStatus" AS ENUM ('OK', 'FAILED', 'DIVERGED', 'CORRUPTED');

CREATE TYPE "InputCompletionStatus" AS ENUM (
    'NONE',
    'ACCEPTED',
    'REJECTED',
    'EXCEPTION',
    'MACHINE_HALTED');

CREATE TYPE "DefaultBlock" AS ENUM ('FINALIZED', 'LATEST', 'PENDING', 'SAFE');

CREATE TYPE "EpochStatus" AS ENUM (
    'OPEN',
    'CLOSED',
    'INPUTS_PROCESSED',
    'CLAIM_COMPUTED',
    'CLAIM_SUBMITTED',
    'CLAIM_STAGED',
    'CLAIM_ACCEPTED',
    'CLAIM_REJECTED',
    'CLAIM_FORECLOSED');

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
    -- claim_staging_period is a cache of the on-chain immutable returned by
    -- IConsensus.getClaimStagingPeriod(). On-chain, submitClaim() always
    -- both submits and stages in the same tx (Authority._submitClaim +
    -- _stageClaim, atomic). acceptClaim() is a separate tx that requires
    -- claim.status == STAGED AND block.number - stagingBlockNumber >=
    -- claim_staging_period. With DEFAULT 0, acceptClaim can fire as early
    -- as the block immediately after staging; with N > 0, accept must wait
    -- N blocks past the staging block. A cached value lower than the chain
    -- value causes ClaimStagingPeriodNotOverYet reverts that the claimer
    -- reclassifies as retry-later (see handleAcceptClaimRevert) — one
    -- wasted broadcast per tick until reality catches up; bounded.
    -- The chain is the source of truth; this column is populated once at
    -- register time from getClaimStagingPeriod().
    "claim_staging_period" uint64 NOT NULL DEFAULT 0,
    "withdrawal_guardian" ethereum_address NOT NULL DEFAULT '\x0000000000000000000000000000000000000000',
    "withdrawal_log2_leaves_per_account" SMALLINT NOT NULL DEFAULT 0 CHECK ("withdrawal_log2_leaves_per_account" BETWEEN 0 AND 255),
    "withdrawal_log2_max_num_of_accounts" SMALLINT NOT NULL DEFAULT 0 CHECK ("withdrawal_log2_max_num_of_accounts" BETWEEN 0 AND 255),
    "withdrawal_accounts_drive_start_index" uint64 NOT NULL DEFAULT 0,
    "withdrawal_output_builder" ethereum_address NOT NULL DEFAULT '\x0000000000000000000000000000000000000000',
    "data_availability" data_availability NOT NULL,
    "consensus_type" "Consensus" NOT NULL,
    "enabled" BOOLEAN NOT NULL DEFAULT true,
    "status" "ApplicationStatus" NOT NULL DEFAULT 'OK',
    "reason" VARCHAR(4096),
    "last_epoch_check_block" uint64 NOT NULL,
    "last_input_check_block" uint64 NOT NULL,
    "last_output_check_block" uint64 NOT NULL,
    "last_tournament_check_block" uint64 NOT NULL,
    "last_foreclose_check_block" uint64 NOT NULL DEFAULT 0,
    "last_accounts_drive_proved_check_block" uint64 NOT NULL DEFAULT 0,
    "last_withdrawal_check_block" uint64 NOT NULL DEFAULT 0,
    "processed_inputs" uint64 NOT NULL,
    -- foreclose_block / accounts_drive_proved_block use 0 as the "not yet
    -- observed" sentinel — block 0 is structurally unreachable for the
    -- corresponding events. The companion hash columns are nullable: a Hash
    -- has no natural unreachable value, so NULL is the canonical "not set"
    -- indicator rather than a manufactured zero-hash literal.
    "foreclose_block" uint64 NOT NULL DEFAULT 0,
    "foreclose_transaction" hash,
    "accounts_drive_proved_block" uint64 NOT NULL DEFAULT 0,
    "accounts_drive_proved_transaction" hash,
    "accounts_drive_merkle_root" hash,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "reason_required_for_failure_statuses" CHECK (NOT ("status" IN ('FAILED', 'DIVERGED', 'CORRUPTED') AND ("reason" IS NULL OR LENGTH("reason") = 0))),
    -- The foreclose pair is populated together by the atomic foreclosure
    -- marker+cursor repository write (set-once, first-writer-wins via WHERE
    -- foreclose_block = 0). This CHECK enforces the same invariant at the
    -- schema level: either both unset (block = 0, tx IS NULL) or both set
    -- together. A future code path that wrote one without the other is
    -- rejected at the DB boundary.
    CONSTRAINT "foreclose_block_and_tx_set_together"
        CHECK (("foreclose_block" = 0) = ("foreclose_transaction" IS NULL)),
    -- Same invariant for the drive-proved pair (set together by the atomic
    -- drive-proved marker+cursor repository write); the merkle root is
    -- recorded only when a proved-block is observed, so all three
    -- drive-proved columns are co-set.
    CONSTRAINT "accounts_drive_proved_columns_set_together"
        CHECK (("accounts_drive_proved_block" = 0)
               = ("accounts_drive_proved_transaction" IS NULL)
               AND ("accounts_drive_proved_block" = 0)
               = ("accounts_drive_merkle_root" IS NULL)),
    CONSTRAINT "application_pkey" PRIMARY KEY ("id")
);

CREATE INDEX "application_data_availability_selector_idx" ON "application"(substring("data_availability" FROM 1 for 4));
-- Supports ListApplications(ForeclosureRecorded = true), used by the claimer's
-- listEnabledForeclosedNonPRTApps once per tick. The filtered set is small
-- (foreclosed apps), so a partial index keyed on foreclose_block > 0 keeps
-- the scan an index-only seek even as the application table grows.
CREATE INDEX "application_foreclosed_idx" ON "application"("id")
    WHERE "foreclose_block" > 0;

CREATE TRIGGER "application_set_updated_at" BEFORE UPDATE ON "application"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE OR REPLACE FUNCTION validate_application_status_transition()
RETURNS TRIGGER AS $$
BEGIN
    -- DIVERGED and CORRUPTED are terminal: once the node detects a consensus
    -- disagreement or local corruption, neither status nor reason may change.
    -- Observation continues independently (it is gated on enabled, not status).
    IF OLD.status IN ('DIVERGED'::"ApplicationStatus", 'CORRUPTED'::"ApplicationStatus")
       AND (NEW.status <> OLD.status OR NEW.reason IS DISTINCT FROM OLD.reason)
    THEN
        RAISE EXCEPTION 'cannot change status or reason of a terminal (%) application', OLD.status;
    END IF;

    -- Foreclosure is one-way and lives in foreclose_block, not in status: a
    -- status write must never clear the foreclosure marker.
    IF OLD.foreclose_block <> 0 AND NEW.foreclose_block = 0 THEN
        RAISE EXCEPTION 'cannot clear foreclose_block of a foreclosed application';
    END IF;

    -- Clear a stale failure reason when health returns to OK.
    IF NEW.status = 'OK'::"ApplicationStatus" THEN
        NEW.reason := NULL;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER "application_validate_status_transition" BEFORE UPDATE ON "application"
FOR EACH ROW EXECUTE FUNCTION validate_application_status_transition();

CREATE TABLE "execution_parameters" (
    "application_id" INT PRIMARY KEY,
    "snapshot_policy" "SnapshotPolicy" NOT NULL DEFAULT 'NONE',
    "advance_inc_cycles" BIGINT NOT NULL CHECK ("advance_inc_cycles" > 0) DEFAULT 4194304, -- 1 << 22
    "advance_max_cycles" BIGINT NOT NULL CHECK ("advance_max_cycles" >= 0 AND "advance_max_cycles" <= 281474976710655) DEFAULT 0, -- 0 uses the machine's fixed (1 << 48) - 1 cycle span
    "inspect_inc_cycles" BIGINT NOT NULL CHECK ("inspect_inc_cycles" > 0) DEFAULT 4194304, -- 1 << 22
    "inspect_max_cycles" BIGINT NOT NULL CHECK ("inspect_max_cycles" >= 0 AND "inspect_max_cycles" <= 281474976710655) DEFAULT 0, -- 0 uses the machine's fixed (1 << 48) - 1 cycle span
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
    "claim_transaction_hash" hash,
    "status" "EpochStatus" NOT NULL,
    "staged_at_block" uint64,
    "virtual_index" uint64 NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "epoch_pkey" PRIMARY KEY ("application_id", "index"),
    CONSTRAINT "epoch_application_id_virtual_index_unique" UNIQUE ("application_id", "virtual_index"),
    CONSTRAINT "epoch_application_id_fkey" FOREIGN KEY ("application_id") REFERENCES "application"("id") ON DELETE CASCADE,
    CONSTRAINT "epoch_block_bounds_check" CHECK ("first_block" <= "last_block"),
    CONSTRAINT "epoch_input_bounds_check" CHECK ("input_index_lower_bound" <= "input_index_upper_bound"),
    -- staged_at_block is set when an epoch is staged on chain and is then
    -- kept historically — same lifetime convention as claim_transaction_hash.
    -- We only enforce the forward direction: if you're in CLAIM_STAGED you
    -- must have a staging block. After transitioning out to CLAIM_ACCEPTED,
    -- the column is retained as audit info.
    CONSTRAINT "epoch_staged_requires_block" CHECK ("status" <> 'CLAIM_STAGED' OR "staged_at_block" IS NOT NULL)
);

CREATE INDEX "epoch_last_block_idx" ON "epoch"("application_id", "last_block");
CREATE INDEX "epoch_status_idx" ON "epoch"("application_id", "status");
-- Supports HasUnreconciledClaimsBeforeBlock: the broad Authority/Quorum drain
-- gate scans epoch rows for (application_id, first_block <= $foreclose_block)
-- restricted to the set of actionable pre-terminal statuses. A partial index keyed on
-- (application_id, first_block) with the same status predicate keeps the
-- scan an index-only lookup; the bare epoch_status_idx covers the status
-- filter but adds a per-row comparison on first_block.
--
-- The status list below MUST match unreconciledEpochStatuses in
-- internal/repository/postgres/epoch.go. If they drift, the query stops
-- matching this partial index and silently degrades to a full table scan.
CREATE INDEX "epoch_unreconciled_idx" ON "epoch"("application_id", "first_block")
    WHERE "status" IN ('OPEN','CLOSED','INPUTS_PROCESSED',
                       'CLAIM_COMPUTED','CLAIM_SUBMITTED','CLAIM_STAGED');

CREATE TRIGGER "epoch_set_updated_at" BEFORE UPDATE ON "epoch"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Enforce valid epoch status transitions.
-- The state machine is:
--   OPEN → CLOSED → INPUTS_PROCESSED → CLAIM_COMPUTED
--   CLAIM_COMPUTED → CLAIM_SUBMITTED → CLAIM_STAGED → CLAIM_ACCEPTED      (v3 normal)
--   CLAIM_COMPUTED → CLAIM_STAGED                                         (restart recovery: chain at STAGED before we submitted)
--   CLAIM_COMPUTED → CLAIM_ACCEPTED                                       (PRT skips SUBMITTED; also valid in deep reader-mode catch-up)
--   CLAIM_COMPUTED  → CLAIM_REJECTED                                      (conflicting Quorum claim staged/accepted before we submitted)
--   CLAIM_SUBMITTED → CLAIM_REJECTED                                      (we submitted, then a different Quorum claim was staged/accepted)
--   OPEN            → CLAIM_FORECLOSED                                    (guardian foreclosed before epoch could finish)
--   CLOSED          → CLAIM_FORECLOSED                                    (guardian foreclosed before inputs/proofs could finish)
--   INPUTS_PROCESSED → CLAIM_FORECLOSED                                   (guardian foreclosed before claim could be computed)
--   CLAIM_COMPUTED  → CLAIM_FORECLOSED                                    (guardian foreclosed before claim could progress)
--   CLAIM_SUBMITTED → CLAIM_FORECLOSED                                    (guardian foreclosed before claim could stage)
--   CLAIM_STAGED    → CLAIM_FORECLOSED                                    (guardian foreclosed before claim could be accepted)
--   CLAIM_STAGED    → CLAIM_ACCEPTED                                      (normal acceptance path)
-- Any other transition (including backwards) is rejected.
-- Same-status updates are allowed (idempotent no-ops).
--
-- When transitioning to CLAIM_COMPUTED, the trigger also verifies that
-- required proof fields are populated:
--   All apps:          machine_hash, outputs_merkle_root, outputs_merkle_proof
--   PRT (DaveConsensus): additionally commitment, commitment_proof
--
-- CLAIM_STAGED is NEVER valid for PRT apps (PRT settles via tournaments,
-- not the staging flow). The trigger rejects this regardless of which
-- transition led to it.
CREATE FUNCTION enforce_epoch_status_transition() RETURNS trigger AS $$
DECLARE
    valid_transitions text[][] := ARRAY[
        ARRAY['OPEN',             'CLOSED'],
        ARRAY['OPEN',             'CLAIM_FORECLOSED'],
        ARRAY['CLOSED',           'INPUTS_PROCESSED'],
        ARRAY['CLOSED',           'CLAIM_FORECLOSED'],
        ARRAY['INPUTS_PROCESSED', 'CLAIM_COMPUTED'],
        ARRAY['INPUTS_PROCESSED', 'CLAIM_FORECLOSED'],
        ARRAY['CLAIM_COMPUTED',   'CLAIM_SUBMITTED'],
        ARRAY['CLAIM_COMPUTED',   'CLAIM_STAGED'],
        ARRAY['CLAIM_COMPUTED',   'CLAIM_ACCEPTED'],
        ARRAY['CLAIM_COMPUTED',   'CLAIM_REJECTED'],
        ARRAY['CLAIM_COMPUTED',   'CLAIM_FORECLOSED'],
        ARRAY['CLAIM_SUBMITTED',  'CLAIM_STAGED'],
        ARRAY['CLAIM_SUBMITTED',  'CLAIM_ACCEPTED'],
        ARRAY['CLAIM_SUBMITTED',  'CLAIM_REJECTED'],
        ARRAY['CLAIM_SUBMITTED',  'CLAIM_FORECLOSED'],
        ARRAY['CLAIM_STAGED',     'CLAIM_FORECLOSED'],
        ARRAY['CLAIM_STAGED',     'CLAIM_ACCEPTED']
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

    -- Enforce CLAIM_STAGED is never valid for PRT consensus, and that
    -- staged_at_block is set when entering CLAIM_STAGED. The
    -- staged_requires_block table CHECK constraint also enforces the latter;
    -- this trigger gives a clearer error message on the state-machine path.
    IF NEW.status::text = 'CLAIM_STAGED' THEN
        IF NEW.staged_at_block IS NULL THEN
            RAISE EXCEPTION
                'CLAIM_STAGED requires staged_at_block to be non-null';
        END IF;

        SELECT a.consensus_type::text INTO app_consensus
          FROM application a
         WHERE a.id = NEW.application_id;

        IF app_consensus = 'PRT' THEN
            RAISE EXCEPTION
                'CLAIM_STAGED is not valid for PRT consensus '
                '(PRT settles via tournaments, not the staging flow)';
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
    "exception_data" BYTEA,
    "machine_hash" hash,
    "outputs_hash" hash,
    "transaction_hash" hash NOT NULL,
    "log_index" uint64 NOT NULL,
    "snapshot_uri" VARCHAR(4096),
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "input_exception_data_check" CHECK (
        ("status" = 'EXCEPTION' AND "exception_data" IS NOT NULL)
        OR
        ("status" <> 'EXCEPTION' AND "exception_data" IS NULL)
    ),
    CONSTRAINT "input_pkey" PRIMARY KEY ("epoch_application_id", "index"),
    CONSTRAINT "input_epoch_index_unique" UNIQUE ("epoch_application_id", "epoch_index", "index"),
    CONSTRAINT "input_application_id_tx_hash_log_index_unique" UNIQUE ("epoch_application_id", "transaction_hash", "log_index"),
    CONSTRAINT "input_completed_hashes_check" CHECK (
        ("status" = 'NONE' AND "machine_hash" IS NULL AND "outputs_hash" IS NULL)
        OR
        ("status" <> 'NONE' AND "machine_hash" IS NOT NULL AND "outputs_hash" IS NOT NULL)
    ),
    CONSTRAINT "input_epoch_id_fkey" FOREIGN KEY ("epoch_application_id", "epoch_index") REFERENCES "epoch"("application_id", "index") ON DELETE CASCADE
);

CREATE INDEX "input_block_number_idx" ON "input"("epoch_application_id", "block_number");
CREATE INDEX "input_status_idx" ON "input"("epoch_application_id", "status");
CREATE INDEX "input_unprocessed_idx" ON "input"("epoch_application_id", "epoch_index", "index")
    WHERE "status" = 'NONE';

CREATE INDEX "input_sender_idx" ON "input" ("epoch_application_id", substring("raw_data" FROM 81 FOR 20));

-- The input table has a hot update pattern: status changes from NONE to a
-- terminal value on every processed input. Lower the autovacuum threshold so
-- dead tuples (and the shrinking partial index) are reclaimed promptly.
ALTER TABLE "input" SET (autovacuum_vacuum_scale_factor = 0.02);

CREATE FUNCTION enforce_input_completion_immutability()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status <> 'NONE' AND (
        NEW.epoch_application_id IS DISTINCT FROM OLD.epoch_application_id
        OR NEW.epoch_index IS DISTINCT FROM OLD.epoch_index
        OR NEW.index IS DISTINCT FROM OLD.index
        OR NEW.raw_data IS DISTINCT FROM OLD.raw_data
        OR NEW.status IS DISTINCT FROM OLD.status
        OR NEW.exception_data IS DISTINCT FROM OLD.exception_data
        OR NEW.machine_hash IS DISTINCT FROM OLD.machine_hash
        OR NEW.outputs_hash IS DISTINCT FROM OLD.outputs_hash
    ) THEN
        RAISE EXCEPTION
            'completed input result is immutable';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER "input_completion_immutability_check"
    BEFORE UPDATE OF "epoch_application_id", "epoch_index", "index", "raw_data",
        "status", "exception_data", "machine_hash", "outputs_hash" ON "input"
    FOR EACH ROW
    EXECUTE FUNCTION enforce_input_completion_immutability();

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

CREATE INDEX "output_input_index_idx" ON "output" ("input_epoch_application_id", "input_index", "index");

CREATE INDEX "output_raw_data_address_idx" ON "output" ("input_epoch_application_id", substring("raw_data" FROM 17 FOR 20))
WHERE SUBSTRING("raw_data" FROM 1 FOR 4) IN (
    E'\\x10321e8b',  -- DelegateCallVoucher
    E'\\x237a816f'   -- Voucher
);

-- Serves GetNumberOfPendingExecutableOutputs and pending-voucher list queries
-- without scanning the application's full output history.
CREATE INDEX "output_pending_voucher_idx" ON "output" ("input_epoch_application_id")
WHERE "execution_transaction_hash" IS NULL AND SUBSTRING("raw_data" FROM 1 FOR 4) IN (
    E'\\x10321e8b',  -- DelegateCallVoucher
    E'\\x237a816f'   -- Voucher
);

CREATE TRIGGER "output_set_updated_at" BEFORE UPDATE ON "output"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Withdrawal rows are append-only for the lifetime of an application: they are
-- inserted as the evmreader scans Withdrawal events and are never updated or
-- deleted (except by ON DELETE CASCADE when the whole application is removed).
-- The post-foreclosure withdrawal scanner relies on this — it uses the row
-- COUNT(*) (GetNumberOfWithdrawals) as the previous on-chain withdrawal counter
-- to seed its monotonic search. Deleting a row would make that count disagree
-- with the chain counter and trip the corruption guard. Do not add an UPDATE or
-- DELETE path for individual withdrawal rows.
CREATE TABLE "withdrawal"
(
    "application_id" INT NOT NULL,
    "account_index" uint64 NOT NULL,
    "account" BYTEA NOT NULL,
    "output" BYTEA NOT NULL,
    "block_number" uint64 NOT NULL,
    "transaction_hash" hash NOT NULL,
    "log_index" INT NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT "withdrawal_pkey" PRIMARY KEY ("application_id", "account_index"),
    CONSTRAINT "withdrawal_application_id_fkey" FOREIGN KEY ("application_id") REFERENCES "application"("id") ON DELETE CASCADE
);

CREATE INDEX "withdrawal_block_number_idx" ON "withdrawal" ("application_id", "block_number");

CREATE TRIGGER "withdrawal_set_updated_at" BEFORE UPDATE ON "withdrawal"
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

CREATE INDEX "report_input_index_idx" ON "report" ("input_epoch_application_id", "input_index", "index");

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

CREATE INDEX "state_hashes_input_index_idx" ON "state_hashes" ("input_epoch_application_id", "input_index", "index");

CREATE TRIGGER "state_hashes_set_updated_at" BEFORE UPDATE ON "state_hashes"
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMIT;
