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
