-- (c) Cartesi and individual authors (see AUTHORS)
-- SPDX-License-Identifier: Apache-2.0 (see LICENSE)

CREATE TYPE "ApplicationStatus" AS ENUM ('RUNNING', 'NOT RUNNING');

CREATE TYPE "InputCompletionStatus" AS ENUM ('NONE', 'ACCEPTED', 'REJECTED', 'EXCEPTION', 'MACHINE_HALTED', 'CYCLE_LIMIT_EXCEEDED', 'TIME_LIMIT_EXCEEDED', 'PAYLOAD_LENGTH_LIMIT_EXCEEDED');

CREATE TYPE "ClaimStatus" AS ENUM ('PENDING', 'SUBMITTED', 'FINALIZED');

CREATE TYPE "DefaultBlock" AS ENUM ('FINALIZED', 'LATEST', 'PENDING', 'SAFE');

CREATE TABLE "application"
(
    "id" SERIAL,
    "contract_address" BYTEA NOT NULL,
    "template_hash" BYTEA NOT NULL,
    "snapshot_uri" VARCHAR(4096) NOT NULL,
    "last_processed_block" NUMERIC(20,0) NOT NULL,
    "status" "ApplicationStatus" NOT NULL,
    "epoch_length" INT NOT NULL,
    CONSTRAINT "application_pkey" PRIMARY KEY ("id"),
    UNIQUE("contract_address")
);

CREATE TABLE "input"
(
    "id" BIGSERIAL,
    "index" NUMERIC(20,0) NOT NULL,
    "raw_data" BYTEA NOT NULL,
    "block_number" NUMERIC(20,0) NOT NULL,
    "status" "InputCompletionStatus" NOT NULL,
    "machine_hash" BYTEA,
    "outputs_hash" BYTEA,
    "application_address" BYTEA NOT NULL,
    CONSTRAINT "input_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "input_application_address_fkey" FOREIGN KEY ("application_address") REFERENCES "application"("contract_address"),
    UNIQUE("index", "application_address")
);

CREATE INDEX "input_idx" ON "input"("block_number");

CREATE TABLE "claim"
(
    "id" BIGSERIAL,
    "index" NUMERIC(20,0) NOT NULL,
    "output_merkle_root_hash" BYTEA NOT NULL,
    "transaction_hash" BYTEA,
    "status" "ClaimStatus" NOT NULL,
    "application_address" BYTEA NOT NULL,
    CONSTRAINT "claim_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "claim_application_address_fkey" FOREIGN KEY ("application_address") REFERENCES "application"("contract_address"),
    UNIQUE("index", "application_address")
);

CREATE TABLE "output"
(
    "id" BIGSERIAL,
    "index" NUMERIC(20,0) NOT NULL,
    "raw_data" BYTEA NOT NULL,
    "hash" BYTEA,
    "output_hashes_siblings" BYTEA[],
    "input_id" BIGINT NOT NULL,
    CONSTRAINT "output_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "output_input_id_fkey" FOREIGN KEY ("input_id") REFERENCES "input"("id")
);

CREATE UNIQUE INDEX "output_idx" ON "output"("index");

CREATE TABLE "report"
(
    "id" BIGSERIAL,
    "index" NUMERIC(20,0) NOT NULL,
    "raw_data" BYTEA NOT NULL,
    "input_id" BIGINT NOT NULL,
    CONSTRAINT "report_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "report_input_id_fkey" FOREIGN KEY ("input_id") REFERENCES "input"("id")
);

CREATE UNIQUE INDEX "report_idx" ON "report"("index");

CREATE TABLE "node_config"
(
    "default_block" "DefaultBlock" NOT NULL,
    "input_box_deployment_block" INT NOT NULL,
    "input_box_address" BYTEA NOT NULL,
    "chain_id" INT NOT NULL,
    "iconsensus_address" BYTEA NOT NULL
);
