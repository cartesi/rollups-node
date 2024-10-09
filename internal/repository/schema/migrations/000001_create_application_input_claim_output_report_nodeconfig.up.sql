-- (c) Cartesi and individual authors (see AUTHORS)
-- SPDX-License-Identifier: Apache-2.0 (see LICENSE)

CREATE TYPE "ApplicationStatus" AS ENUM ('RUNNING', 'NOT RUNNING');

CREATE TYPE "InputCompletionStatus" AS ENUM (
    'NONE',
    'ACCEPTED',
    'REJECTED',
    'EXCEPTION',
    'MACHINE_HALTED',
    'CYCLE_LIMIT_EXCEEDED',
    'TIME_LIMIT_EXCEEDED',
    'PAYLOAD_LENGTH_LIMIT_EXCEEDED');

CREATE TYPE "DefaultBlock" AS ENUM ('FINALIZED', 'LATEST', 'PENDING', 'SAFE');

CREATE TYPE "EpochStatus" AS ENUM (
    'OPEN',
    'CLOSED',
    'PROCESSED_ALL_INPUTS',
    'CLAIM_COMPUTED',
    'CLAIM_SUBMITTED',
    'CLAIM_ACCEPTED',
    'CLAIM_REJECTED');

CREATE FUNCTION "f_maxuint64"()
    RETURNS NUMERIC(20,0)
    LANGUAGE sql IMMUTABLE PARALLEL SAFE AS
        'SELECT 18446744073709551615';

CREATE TABLE "application"
(
    "id" SERIAL,
    "contract_address" BYTEA NOT NULL,
    "template_hash" BYTEA NOT NULL,
    "template_uri" VARCHAR(4096) NOT NULL,
    "last_processed_block" NUMERIC(20,0) NOT NULL CHECK ("last_processed_block" >= 0 AND "last_processed_block" <= f_maxuint64()),
    "status" "ApplicationStatus" NOT NULL,
    "iconsensus_address" BYTEA NOT NULL,
    "last_claim_check_block" NUMERIC(20,0) NOT NULL CHECK ("last_claim_check_block" >= 0 AND "last_claim_check_block" <= f_maxuint64()),
    "last_output_check_block" NUMERIC(20,0) NOT NULL CHECK ("last_output_check_block" >= 0 AND "last_output_check_block" <= f_maxuint64()),
    CONSTRAINT "application_pkey" PRIMARY KEY ("id"),
    UNIQUE("contract_address")
);


CREATE TABLE "execution_parameters" (
    "application_id" INT PRIMARY KEY,
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
    CONSTRAINT "application_id_fkey" FOREIGN KEY ("application_id") REFERENCES "application"("id")
);

CREATE TABLE "epoch"
(
    "id" BIGSERIAL,
    "application_address" BYTEA NOT NULL,
    "index" BIGINT NOT NULL,
    "first_block" NUMERIC(20,0) NOT NULL CHECK ("first_block" >= 0 AND "first_block" <= f_maxuint64()),
    "last_block" NUMERIC(20,0) NOT NULL CHECK ("last_block" >= 0 AND "last_block" <= f_maxuint64()),
    "claim_hash" BYTEA,
    "transaction_hash" BYTEA,
    "status" "EpochStatus" NOT NULL,
    CONSTRAINT "epoch_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "epoch_application_address_fkey" FOREIGN KEY ("application_address") REFERENCES "application"("contract_address"),
    UNIQUE ("index","application_address")
);

CREATE INDEX "epoch_idx" ON "epoch"("index");
CREATE INDEX "epoch_last_block_idx" ON "epoch"("last_block");

CREATE TABLE "input"
(
    "id" BIGSERIAL,
    "index" NUMERIC(20,0) NOT NULL CHECK ("index" >= 0 AND "index" <= f_maxuint64()),
    "raw_data" BYTEA NOT NULL,
    "block_number" NUMERIC(20,0) NOT NULL CHECK ("block_number" >= 0 AND "block_number" <= f_maxuint64()),
    "status" "InputCompletionStatus" NOT NULL,
    "machine_hash" BYTEA,
    "outputs_hash" BYTEA,
    "application_address" BYTEA NOT NULL,
    "epoch_id" BIGINT NOT NULL,
    CONSTRAINT "input_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "input_application_address_fkey" FOREIGN KEY ("application_address") REFERENCES "application"("contract_address"),
    CONSTRAINT "input_epoch_fkey" FOREIGN KEY ("epoch_id") REFERENCES "epoch"("id"),
    UNIQUE("index", "application_address")
);

CREATE INDEX "input_idx" ON "input"("block_number");

CREATE TABLE "output"
(
    "id" BIGSERIAL,
    "index" NUMERIC(20,0) NOT NULL CHECK ("index" >= 0 AND "index" <= f_maxuint64()),
    "raw_data" BYTEA NOT NULL,
    "hash" BYTEA,
    "output_hashes_siblings" BYTEA[],
    "input_id" BIGINT NOT NULL,
    "transaction_hash" BYTEA,
    CONSTRAINT "output_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "output_input_id_fkey" FOREIGN KEY ("input_id") REFERENCES "input"("id")
);

CREATE INDEX "output_idx" ON "output"("index");

CREATE TABLE "report"
(
    "id" BIGSERIAL,
    "index" NUMERIC(20,0) NOT NULL CHECK ("index" >= 0 AND "index" <= f_maxuint64()),
    "raw_data" BYTEA NOT NULL,
    "input_id" BIGINT NOT NULL,
    CONSTRAINT "report_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "report_input_id_fkey" FOREIGN KEY ("input_id") REFERENCES "input"("id")
);

CREATE INDEX "report_idx" ON "report"("index");

CREATE TABLE "snapshot"
(
    "id" BIGSERIAL,
    "input_id" BIGINT NOT NULL,
    "application_address" BYTEA NOT NULL,
    "uri" VARCHAR(4096) NOT NULL,
    CONSTRAINT "snapshot_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "snapshot_input_id_fkey" FOREIGN KEY ("input_id") REFERENCES "input"("id"),
    CONSTRAINT "snapshot_application_address_fkey" FOREIGN KEY ("application_address") REFERENCES "application"("contract_address"),
    UNIQUE("input_id")
);

CREATE TABLE "node_config"
(
    "default_block" "DefaultBlock" NOT NULL,
    "input_box_deployment_block" INT NOT NULL,
    "input_box_address" BYTEA NOT NULL,
    "chain_id" INT NOT NULL
);


