-- (c) Cartesi and individual authors (see AUTHORS)
-- SPDX-License-Identifier: Apache-2.0 (see LICENSE)

DROP TABLE IF EXISTS "node_config";
DROP TABLE IF EXISTS "snapshot";
DROP TABLE IF EXISTS "report";
DROP TABLE IF EXISTS "output";
DROP TABLE IF EXISTS "input";
DROP TABLE IF EXISTS "epoch";
DROP TABLE IF EXISTS "execution_parameters";
DROP TABLE IF EXISTS "application";

DROP FUNCTION IF EXISTS "f_maxuint64";

DROP TYPE IF EXISTS "InputCompletionStatus";
DROP TYPE IF EXISTS "ApplicationStatus";
DROP TYPE IF EXISTS "DefaultBlock";
DROP TYPE IF EXISTS "EpochStatus";
