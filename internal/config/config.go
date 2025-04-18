// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// The config package manages the node configuration, which comes from environment variables.
// The sub-package generate specifies these environment variables.
package config

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cartesi/rollups-node/internal/model"

	"github.com/ethereum/go-ethereum/common"
)

// Redacted is a wrapper that redacts a given field from the logs.
type Redacted[T any] struct {
	Value T
}

func (r Redacted[T]) String() string {
	return "[REDACTED]"
}

type (
	URL            = *url.URL
	Duration       = time.Duration
	LogLevel       = slog.Level
	DefaultBlock   = model.DefaultBlock
	RedactedString = Redacted[string]
	RedactedUint   = Redacted[uint32]
	Address        = common.Address
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
// JSON-RPC Machine log level
// ------------------------------------------------------------------------------------------------

// MachineLogLevel represents the verbosity level for machine logs
type MachineLogLevel string

const (
	MachineLogLevelTrace MachineLogLevel = "trace"
	MachineLogLevelDebug MachineLogLevel = "debug"
	MachineLogLevelInfo  MachineLogLevel = "info"
	MachineLogLevelWarn  MachineLogLevel = "warn"
	MachineLogLevelError MachineLogLevel = "error"
	MachineLogLevelFatal MachineLogLevel = "fatal"
)

// ------------------------------------------------------------------------------------------------
// Parsing functions
// ------------------------------------------------------------------------------------------------

func ToUint64FromString(s string) (uint64, error) {
	value, err := strconv.ParseUint(s, 10, 64)
	return value, err
}

func ToUint64FromDecimalOrHexString(s string) (uint64, error) {
	if len(s) >= 2 && (strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X")) {
		return strconv.ParseUint(s[2:], 16, 64)
	}
	return ToUint64FromString(s)
}

func ToStringFromString(s string) (string, error) {
	return s, nil
}

func ToDurationFromSeconds(s string) (time.Duration, error) {
	return time.ParseDuration(s + "s")
}

func ToLogLevelFromString(s string) (LogLevel, error) {
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

func ToMachineLogLevelFromString(s string) (MachineLogLevel, error) {
	var m = map[string]MachineLogLevel{
		string(MachineLogLevelTrace): MachineLogLevelTrace,
		string(MachineLogLevelDebug): MachineLogLevelDebug,
		string(MachineLogLevelInfo):  MachineLogLevelInfo,
		string(MachineLogLevelWarn):  MachineLogLevelWarn,
		string(MachineLogLevelError): MachineLogLevelError,
		string(MachineLogLevelFatal): MachineLogLevelFatal,
	}
	if v, ok := m[s]; ok {
		return v, nil
	}
	return "", fmt.Errorf("invalid remote machine log level")
}

func ToAddressFromString(s string) (Address, error) {
	if len(s) < 3 || (!strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X")) {
		return Address{}, fmt.Errorf("invalid address '%s'", s)
	}
	s = s[2:]
	b, err := hex.DecodeString(s)
	if err != nil {
		return Address{}, err
	}
	return common.BytesToAddress(b), nil
}

func ToApplicationNameFromString(s string) (string, error) {
	if s == "" {
		return "", fmt.Errorf("application name cannot be empty")
	}
	validNamePattern := regexp.MustCompile(`^[a-z0-9_-]+$`)
	if !validNamePattern.MatchString(s) {
		return "", fmt.Errorf("invalid application name '%s': must contain only lowercase letters, numbers, underscores, and hyphens", s)
	}
	return s, nil
}

func ToApplicationNameOrAddressFromString(s string) (string, error) {
	if s == "" {
		return "", fmt.Errorf("application name or address cannot be empty")
	}
	if len(s) >= 3 && (strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X")) {
		_, err := ToAddressFromString(s)
		if err != nil {
			return "", fmt.Errorf("invalid Ethereum address '%s'", s)
		}
		return s, nil
	}
	return ToApplicationNameFromString(s)
}

func ToDefaultBlockFromString(s string) (DefaultBlock, error) {
	var m = map[string]DefaultBlock{
		"latest":    model.DefaultBlock_Latest,
		"pending":   model.DefaultBlock_Pending,
		"safe":      model.DefaultBlock_Safe,
		"finalized": model.DefaultBlock_Finalized,
	}
	if v, ok := m[s]; ok {
		return v, nil
	} else {
		var zeroValue DefaultBlock
		return zeroValue, fmt.Errorf("invalid default block '%s'", s)
	}
}

func ToAuthKindFromString(s string) (AuthKind, error) {
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

func ToRedactedStringFromString(s string) (RedactedString, error) {
	return RedactedString{s}, nil
}

func ToRedactedUint32FromString(s string) (RedactedUint, error) {
	value, err := strconv.ParseUint(s, 10, 32)
	return RedactedUint{uint32(value)}, err
}

func ToURLFromString(s string) (URL, error) {
	result, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid URL [Redacted]")
	}
	return result, nil
}

// Aliases to be used by the generated functions.
var (
	toBool            = strconv.ParseBool
	toUint64          = ToUint64FromString
	toString          = ToStringFromString
	toDuration        = ToDurationFromSeconds
	toLogLevel        = ToLogLevelFromString
	toAuthKind        = ToAuthKindFromString
	toDefaultBlock    = ToDefaultBlockFromString
	toRedactedString  = ToRedactedStringFromString
	toRedactedUint    = ToRedactedUint32FromString
	toURL             = ToURLFromString
	toMachineLogLevel = ToMachineLogLevelFromString
	toAddress         = ToAddressFromString
)

var (
	notDefinedbool            = func() bool { return false }
	notDefineduint64          = func() uint64 { return 0 }
	notDefinedstring          = func() string { return "" }
	notDefinedDuration        = func() time.Duration { return 0 }
	notDefinedLogLevel        = func() slog.Level { return slog.LevelInfo }
	notDefinedAuthKind        = func() AuthKind { return AuthKindMnemonicVar }
	notDefinedDefaultBlock    = func() model.DefaultBlock { return model.DefaultBlock_Finalized }
	notDefinedRedactedString  = func() RedactedString { return RedactedString{""} }
	notDefinedRedactedUint    = func() RedactedUint { return RedactedUint{0} }
	notDefinedURL             = func() URL { return &url.URL{} }
	notDefinedMachineLogLevel = func() MachineLogLevel { return MachineLogLevelInfo }
	notDefinedAddress         = func() Address { return common.Address{} }
)
