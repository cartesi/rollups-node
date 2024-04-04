// Code generated by internal/node/config/generate.
// DO NOT EDIT.

// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

type (
	Duration = time.Duration
	LogLevel = slog.Level
)

// ------------------------------------------------------------------------------------------------
// Auth Kind
// ------------------------------------------------------------------------------------------------

type AuthKind uint8

const (
	AuthKindPrivateKeyVar AuthKind = iota
	AuthKindPrivateKeyFile
	AuthKindMnemonicVar
	AuthKindMnemonicFile
	AuthKindAWS
)

// ------------------------------------------------------------------------------------------------
// Parsing functions
// ------------------------------------------------------------------------------------------------

func toInt64FromString(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func toUint64FromString(s string) (uint64, error) {
	value, err := strconv.ParseUint(s, 10, 64)
	return value, err
}

func toStringFromString(s string) (string, error) {
	return s, nil
}

func toDurationFromSeconds(s string) (time.Duration, error) {
	return time.ParseDuration(s + "s")
}

func toLogLevelFromString(s string) (LogLevel, error) {
	var m = map[string]LogLevel{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	if v, ok := m[s]; ok {
		return v, nil
	} else {
		var zeroValue LogLevel
		return zeroValue, fmt.Errorf("invalid log level '%s'", s)
	}
}

func toAuthKindFromString(s string) (AuthKind, error) {
	var m = map[string]AuthKind{
		"private_key":      AuthKindPrivateKeyVar,
		"private_key_file": AuthKindPrivateKeyFile,
		"mnemonic":         AuthKindMnemonicVar,
		"mnemonic_file":    AuthKindMnemonicFile,
		"aws":              AuthKindAWS,
	}
	if v, ok := m[s]; ok {
		return v, nil
	} else {
		var zeroValue AuthKind
		return zeroValue, fmt.Errorf("invalid auth kind '%s'", s)
	}
}

// Aliases to be used by the generated functions.
var (
	toBool     = strconv.ParseBool
	toInt      = strconv.Atoi
	toInt64    = toInt64FromString
	toUint64   = toUint64FromString
	toString   = toStringFromString
	toDuration = toDurationFromSeconds
	toLogLevel = toLogLevelFromString
	toAuthKind = toAuthKindFromString
)

// ------------------------------------------------------------------------------------------------
// Getters
// ------------------------------------------------------------------------------------------------

func getAuthAwsKmsKeyId() string {
	s, ok := os.LookupEnv("CARTESI_AUTH_AWS_KMS_KEY_ID")
	if !ok {
		panic("missing env var CARTESI_AUTH_AWS_KMS_KEY_ID")
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_AUTH_AWS_KMS_KEY_ID: %v", err))
	}
	return val
}

func getAuthAwsKmsRegion() string {
	s, ok := os.LookupEnv("CARTESI_AUTH_AWS_KMS_REGION")
	if !ok {
		panic("missing env var CARTESI_AUTH_AWS_KMS_REGION")
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_AUTH_AWS_KMS_REGION: %v", err))
	}
	return val
}

func getAuthKind() AuthKind {
	s, ok := os.LookupEnv("CARTESI_AUTH_KIND")
	if !ok {
		s = "mnemonic"
	}
	val, err := toAuthKind(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_AUTH_KIND: %v", err))
	}
	return val
}

func getAuthMnemonic() string {
	s, ok := os.LookupEnv("CARTESI_AUTH_MNEMONIC")
	if !ok {
		panic("missing env var CARTESI_AUTH_MNEMONIC")
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_AUTH_MNEMONIC: %v", err))
	}
	return val
}

func getAuthMnemonicAccountIndex() int {
	s, ok := os.LookupEnv("CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX")
	if !ok {
		s = "0"
	}
	val, err := toInt(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_AUTH_MNEMONIC_ACCOUNT_INDEX: %v", err))
	}
	return val
}

func getAuthMnemonicFile() string {
	s, ok := os.LookupEnv("CARTESI_AUTH_MNEMONIC_FILE")
	if !ok {
		panic("missing env var CARTESI_AUTH_MNEMONIC_FILE")
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_AUTH_MNEMONIC_FILE: %v", err))
	}
	return val
}

func getAuthPrivateKey() string {
	s, ok := os.LookupEnv("CARTESI_AUTH_PRIVATE_KEY")
	if !ok {
		panic("missing env var CARTESI_AUTH_PRIVATE_KEY")
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_AUTH_PRIVATE_KEY: %v", err))
	}
	return val
}

func getAuthPrivateKeyFile() string {
	s, ok := os.LookupEnv("CARTESI_AUTH_PRIVATE_KEY_FILE")
	if !ok {
		panic("missing env var CARTESI_AUTH_PRIVATE_KEY_FILE")
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_AUTH_PRIVATE_KEY_FILE: %v", err))
	}
	return val
}

func getBlockchainBlockTimeout() int {
	s, ok := os.LookupEnv("CARTESI_BLOCKCHAIN_BLOCK_TIMEOUT")
	if !ok {
		s = "60"
	}
	val, err := toInt(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_BLOCKCHAIN_BLOCK_TIMEOUT: %v", err))
	}
	return val
}

func getBlockchainFinalityOffset() int {
	s, ok := os.LookupEnv("CARTESI_BLOCKCHAIN_FINALITY_OFFSET")
	if !ok {
		s = "10"
	}
	val, err := toInt(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_BLOCKCHAIN_FINALITY_OFFSET: %v", err))
	}
	return val
}

func getBlockchainHttpEndpoint() string {
	s, ok := os.LookupEnv("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT")
	if !ok {
		panic("missing env var CARTESI_BLOCKCHAIN_HTTP_ENDPOINT")
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_BLOCKCHAIN_HTTP_ENDPOINT: %v", err))
	}
	return val
}

func getBlockchainId() uint64 {
	s, ok := os.LookupEnv("CARTESI_BLOCKCHAIN_ID")
	if !ok {
		panic("missing env var CARTESI_BLOCKCHAIN_ID")
	}
	val, err := toUint64(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_BLOCKCHAIN_ID: %v", err))
	}
	return val
}

func getBlockchainIsLegacy() bool {
	s, ok := os.LookupEnv("CARTESI_BLOCKCHAIN_IS_LEGACY")
	if !ok {
		s = "false"
	}
	val, err := toBool(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_BLOCKCHAIN_IS_LEGACY: %v", err))
	}
	return val
}

func getBlockchainWsEndpoint() string {
	s, ok := os.LookupEnv("CARTESI_BLOCKCHAIN_WS_ENDPOINT")
	if !ok {
		panic("missing env var CARTESI_BLOCKCHAIN_WS_ENDPOINT")
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_BLOCKCHAIN_WS_ENDPOINT: %v", err))
	}
	return val
}

func getContractsApplicationAddress() string {
	s, ok := os.LookupEnv("CARTESI_CONTRACTS_APPLICATION_ADDRESS")
	if !ok {
		panic("missing env var CARTESI_CONTRACTS_APPLICATION_ADDRESS")
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_CONTRACTS_APPLICATION_ADDRESS: %v", err))
	}
	return val
}

func getContractsApplicationDeploymentBlockNumber() int64 {
	s, ok := os.LookupEnv("CARTESI_CONTRACTS_APPLICATION_DEPLOYMENT_BLOCK_NUMBER")
	if !ok {
		panic("missing env var CARTESI_CONTRACTS_APPLICATION_DEPLOYMENT_BLOCK_NUMBER")
	}
	val, err := toInt64(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_CONTRACTS_APPLICATION_DEPLOYMENT_BLOCK_NUMBER: %v", err))
	}
	return val
}

func getContractsAuthorityAddress() string {
	s, ok := os.LookupEnv("CARTESI_CONTRACTS_AUTHORITY_ADDRESS")
	if !ok {
		panic("missing env var CARTESI_CONTRACTS_AUTHORITY_ADDRESS")
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_CONTRACTS_AUTHORITY_ADDRESS: %v", err))
	}
	return val
}

func getContractsHistoryAddress() string {
	s, ok := os.LookupEnv("CARTESI_CONTRACTS_HISTORY_ADDRESS")
	if !ok {
		panic("missing env var CARTESI_CONTRACTS_HISTORY_ADDRESS")
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_CONTRACTS_HISTORY_ADDRESS: %v", err))
	}
	return val
}

func getContractsInputBoxAddress() string {
	s, ok := os.LookupEnv("CARTESI_CONTRACTS_INPUT_BOX_ADDRESS")
	if !ok {
		panic("missing env var CARTESI_CONTRACTS_INPUT_BOX_ADDRESS")
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_CONTRACTS_INPUT_BOX_ADDRESS: %v", err))
	}
	return val
}

func getContractsInputBoxDeploymentBlockNumber() int64 {
	s, ok := os.LookupEnv("CARTESI_CONTRACTS_INPUT_BOX_DEPLOYMENT_BLOCK_NUMBER")
	if !ok {
		panic("missing env var CARTESI_CONTRACTS_INPUT_BOX_DEPLOYMENT_BLOCK_NUMBER")
	}
	val, err := toInt64(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_CONTRACTS_INPUT_BOX_DEPLOYMENT_BLOCK_NUMBER: %v", err))
	}
	return val
}

func getExperimentalServerManagerBypassLog() bool {
	s, ok := os.LookupEnv("CARTESI_EXPERIMENTAL_SERVER_MANAGER_BYPASS_LOG")
	if !ok {
		s = "false"
	}
	val, err := toBool(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_EXPERIMENTAL_SERVER_MANAGER_BYPASS_LOG: %v", err))
	}
	return val
}

func getExperimentalSunodoValidatorEnabled() bool {
	s, ok := os.LookupEnv("CARTESI_EXPERIMENTAL_SUNODO_VALIDATOR_ENABLED")
	if !ok {
		s = "false"
	}
	val, err := toBool(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_EXPERIMENTAL_SUNODO_VALIDATOR_ENABLED: %v", err))
	}
	return val
}

func getExperimentalSunodoValidatorRedisEndpoint() string {
	s, ok := os.LookupEnv("CARTESI_EXPERIMENTAL_SUNODO_VALIDATOR_REDIS_ENDPOINT")
	if !ok {
		panic("missing env var CARTESI_EXPERIMENTAL_SUNODO_VALIDATOR_REDIS_ENDPOINT")
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_EXPERIMENTAL_SUNODO_VALIDATOR_REDIS_ENDPOINT: %v", err))
	}
	return val
}

func getFeatureDisableClaimer() bool {
	s, ok := os.LookupEnv("CARTESI_FEATURE_DISABLE_CLAIMER")
	if !ok {
		s = "false"
	}
	val, err := toBool(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_FEATURE_DISABLE_CLAIMER: %v", err))
	}
	return val
}

func getFeatureDisableMachineHashCheck() bool {
	s, ok := os.LookupEnv("CARTESI_FEATURE_DISABLE_MACHINE_HASH_CHECK")
	if !ok {
		s = "false"
	}
	val, err := toBool(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_FEATURE_DISABLE_MACHINE_HASH_CHECK: %v", err))
	}
	return val
}

func getHttpAddress() string {
	s, ok := os.LookupEnv("CARTESI_HTTP_ADDRESS")
	if !ok {
		s = "127.0.0.1"
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_HTTP_ADDRESS: %v", err))
	}
	return val
}

func getHttpPort() int {
	s, ok := os.LookupEnv("CARTESI_HTTP_PORT")
	if !ok {
		s = "10000"
	}
	val, err := toInt(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_HTTP_PORT: %v", err))
	}
	return val
}

func getLogLevel() LogLevel {
	s, ok := os.LookupEnv("CARTESI_LOG_LEVEL")
	if !ok {
		s = "info"
	}
	val, err := toLogLevel(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_LOG_LEVEL: %v", err))
	}
	return val
}

func getLogPretty() bool {
	s, ok := os.LookupEnv("CARTESI_LOG_PRETTY")
	if !ok {
		s = "false"
	}
	val, err := toBool(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_LOG_PRETTY: %v", err))
	}
	return val
}

func getPostgresEndpoint() string {
	s, ok := os.LookupEnv("CARTESI_POSTGRES_ENDPOINT")
	if !ok {
		s = ""
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_POSTGRES_ENDPOINT: %v", err))
	}
	return val
}

func getEpochDuration() Duration {
	s, ok := os.LookupEnv("CARTESI_EPOCH_DURATION")
	if !ok {
		s = "86400"
	}
	val, err := toDuration(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_EPOCH_DURATION: %v", err))
	}
	return val
}

func getSnapshotDir() string {
	s, ok := os.LookupEnv("CARTESI_SNAPSHOT_DIR")
	if !ok {
		panic("missing env var CARTESI_SNAPSHOT_DIR")
	}
	val, err := toString(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CARTESI_SNAPSHOT_DIR: %v", err))
	}
	return val
}
