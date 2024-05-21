-- (c) Cartesi and individual authors (see AUTHORS)
-- SPDX-License-Identifier: Apache-2.0 (see LICENSE)

CREATE TABLE "epochs"
(
    "start_block" INT NOT NULL,
    "end_block" INT NOT NULL,
    CONSTRAINT "epochs_pkey" PRIMARY KEY ("start_block")
);

CREATE TABLE "node_state"
(
    "most_recently_finalized_block" INT NOT NULL,
    "input_box_deployment_block" INT NOT NULL,
    "epoch_duration" INT NOT NULL,
    "current_epoch" INT NOT NULL,
    CONSTRAINT "node_state_current_epoch_fkey" FOREIGN KEY ("current_epoch") REFERENCES "epochs"("start_block")
);

CREATE TABLE "claims"
(
    "id" INT NOT NULL,
    "epoch" INT NOT NULL,
    "first_input_index" INT NOT NULL,
    "last_input_index" INT NOT NULL,
    "epoch_hash" BYTEA NOT NULL,
    "application_address" BYTEA NOT NULL,
    CONSTRAINT "claims_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "claims_epoch_fkey" FOREIGN KEY ("epoch") REFERENCES "epochs"("start_block")
);

CREATE TABLE "proofs"
(
    "input_index" INT NOT NULL,
    "claim_id" INT NOT NULL,
    "input_index_within_epoch" INT NOT NULL,
    "output_index_within_input" INT NOT NULL,
    "output_hashes_root_hash" BYTEA NOT NULL,
    "output_epoch_root_hash" BYTEA NOT NULL,
    "machine_state_hash" BYTEA NOT NULL,
    "output_hash_in_output_hashes_siblings" BYTEA[] NOT NULL,
    "output_hashes_in_epoch_siblings" BYTEA[] NOT NULL,
    CONSTRAINT "proofs_pkey" PRIMARY KEY ("input_index", "output_index_within_input"),
    CONSTRAINT "proofs_input_index_fkey" FOREIGN KEY ("input_index") REFERENCES "inputs"("index"),
    CONSTRAINT "proofs_output_index_fkey" FOREIGN KEY ("input_index", "output_index_within_input") REFERENCES "outputs"("input_index", "index"),
    CONSTRAINT "proofs_claim_id_fkey" FOREIGN KEY ("claim_id") REFERENCES "claims"("id")
);
