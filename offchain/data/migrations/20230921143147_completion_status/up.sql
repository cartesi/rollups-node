-- Your SQL goes here

CREATE TYPE "CompletionStatus" AS ENUM (
    'Unprocessed',
    'Accepted',
    'Rejected',
    'Exception',
    'MachineHalted',
    'CycleLimitExceeded',
    'TimeLimitExceeded',
    'PayloadLengthLimitExceeded'
);

ALTER TABLE "inputs" ADD "status" "CompletionStatus" NOT NULL DEFAULT 'Unprocessed';
