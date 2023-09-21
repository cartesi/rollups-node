-- This file should undo anything in `up.sql`

ALTER TABLE "inputs" DROP "status";

DROP TYPE "CompletionStatus";
