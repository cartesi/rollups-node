// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ConfigTestSuite struct {
	suite.Suite
}

func (s *ConfigTestSuite) SetupSuite() {
	os.Setenv("CARTESI_BLOCKCHAIN_ID", "31337")
	os.Setenv("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT", "http://localhost:8545")
	os.Setenv("CARTESI_BLOCKCHAIN_WS_ENDPOINT", "ws://localhost:8545")
	os.Setenv("CARTESI_CONTRACTS_APPLICATION_ADDRESS", "0x")
	os.Setenv("CARTESI_CONTRACTS_HISTORY_ADDRESS", "0x")
	os.Setenv("CARTESI_CONTRACTS_AUTHORITY_ADDRESS", "0x")
	os.Setenv("CARTESI_CONTRACTS_INPUT_BOX_ADDRESS", "0x")
	os.Setenv("CARTESI_CONTRACTS_INPUT_BOX_DEPLOYMENT_BLOCK_NUMBER", "0")
	os.Setenv("CARTESI_SNAPSHOT_DIR", "/tmp")
}

func TestConfigTest(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}

func (s *ConfigTestSuite) TestAuthIsNotSetWhenClaimerIsDisabled() {
	os.Setenv("CARTESI_FEATURE_DISABLE_CLAIMER", "true")
	c := FromEnv()
	assert.Nil(s.T(), c.Auth)
}

func (s *ConfigTestSuite) TestExperimentalSunodoValidatorRedisEndpointIsRedacted() {
	enableSunodoValidatorMode()
	c := FromEnv()
	assert.Equal(s.T(), "[REDACTED]", c.ExperimentalSunodoValidatorRedisEndpoint.String())
}

func enableSunodoValidatorMode() {
	os.Setenv("CARTESI_EXPERIMENTAL_SUNODO_VALIDATOR_ENABLED", "true")
	os.Setenv("CARTESI_EXPERIMENTAL_SUNODO_VALIDATOR_REDIS_ENDPOINT",
		"redis://username:p@ssw0rd@hostname:9999")
}
