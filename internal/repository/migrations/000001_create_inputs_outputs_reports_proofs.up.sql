-- (c) Cartesi and individual authors (see AUTHORS)
-- SPDX-License-Identifier: Apache-2.0 (see LICENSE)

CREATE TYPE "CompletionStatus" AS ENUM ('UNPROCESSED', 'ACCEPTED', 'REJECTED', 'EXCEPTION', 'MACHINE_HALTED', 'CYCLE_LIMIT_EXCEEDED', 'TIME_LIMIT_EXCEEDED', 'PAYLOAD_LENGTH_LIMIT_EXCEEDED');

CREATE TABLE "inputs"
(
    "index" INT NOT NULL,
    "blob" BYTEA NOT NULL,
    "status" "CompletionStatus" NOT NULL,
    CONSTRAINT "inputs_pkey" PRIMARY KEY ("index")
);

CREATE TABLE "outputs"
(
    "input_index" INT NOT NULL,
    "index" INT NOT NULL,
    "blob" BYTEA NOT NULL,
    CONSTRAINT "outputs_pkey" PRIMARY KEY ("input_index", "index"),
    CONSTRAINT "outputs_input_index_fkey" FOREIGN KEY ("input_index") REFERENCES "inputs"("index")
);

CREATE TABLE "reports"
(
    "input_index" INT NOT NULL,
    "index" INT NOT NULL,
    "blob" BYTEA NOT NULL,
    CONSTRAINT "reports_pkey" PRIMARY KEY ("input_index", "index"),
    CONSTRAINT "reports_input_index_fkey" FOREIGN KEY ("input_index") REFERENCES "inputs"("index")
);

CREATE TABLE "proofs"
(
    "input_index" INT NOT NULL,
    "output_index" INT NOT NULL,
    "epoch_first_input_index" INT NOT NULL,
    "epoch_last_input_index" INT NOT NULL,
    "validity_input_index_within_epoch" INT NOT NULL,
    "validity_output_index_within_input" INT NOT NULL,
    "validity_output_hashes_root_hash" BYTEA NOT NULL,
    "validity_outputs_epoch_root_hash" BYTEA NOT NULL,
    "validity_machine_state_hash" BYTEA NOT NULL,
    "validity_output_hash_in_output_hashes_siblings" BYTEA[] NOT NULL,
    "validity_output_hashes_in_epoch_siblings" BYTEA[] NOT NULL,
    CONSTRAINT "proofs_pkey" PRIMARY KEY ("input_index", "output_index"),
    CONSTRAINT "proofs_input_index_fkey" FOREIGN KEY ("input_index") REFERENCES "inputs"("index"),
    CONSTRAINT "proofs_output_index_fkey" FOREIGN KEY ("input_index", "output_index") REFERENCES "outputs"("input_index", "index")
);
