// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestEnv(t *testing.T) {
	suite.Run(t, new(EnvSuite))
}

type EnvSuite struct {
	suite.Suite
}

func (suite *EnvSuite) TearDownTest() {
	os.Unsetenv(FOO)
	os.Unsetenv(BAR)
	os.Unsetenv(BAZ)
	cache.values = make(map[string]string)
	logInit()
}

// ------------------------------------------------------------------------------------------------

func (suite *EnvSuite) TestRead() {
	require := suite.Require()

	// test specific setup
	cache.values = make(map[string]string)
	cacheLen := len(cache.values)
	require.Equal(cacheLen, len(cache.values))

	// mocking the logger
	var buffer bytes.Buffer
	configLogger = log.New(&buffer, "", 0)

	name, value := FOO, foo

	{ // not initialized
		s, ok := read(name, false)
		require.Equal("", s)
		require.False(ok)
		require.Equal(cacheLen, len(cache.values))
		require.Zero(len(getMockedLog(buffer)))
	}
	{ // initialized
		os.Setenv(name, value)
		s, ok := read(name, false)
		require.True(ok)
		require.Equal(value, s)
		require.Equal(cacheLen+1, len(cache.values))
		suite.T().Log(buffer.String())
		require.Equal(1, len(getMockedLog(buffer)))
	}
	{ // cached
		os.Setenv(name, "another foo")
		s, ok := read(name, false)
		require.True(ok)
		require.Equal(value, s)
		require.Equal(cacheLen+1, len(cache.values))
		require.Equal(1, len(getMockedLog(buffer)))
	}
	{ // redacted
		os.Setenv(BAR, bar)
		s, ok := read(BAR, true)
		require.True(ok)
		require.Equal(bar, s)
		require.Equal(cacheLen+2, len(cache.values))
		require.Equal(1, len(getMockedLog(buffer)))
	}
	{ // empty string
		os.Setenv(BAZ, "")
		s, ok := read(BAZ, false)
		require.True(ok)
		require.Equal("", s)
		require.Equal(cacheLen+3, len(cache.values))
		require.Equal(2, len(getMockedLog(buffer)))
	}
}

func (suite *EnvSuite) TestGetOptional() {
	require := suite.Require()
	{ // not set | not cached | no default
		_, ok := getOptional[int](FOO, "", false, true, toInt)
		require.False(ok)
	}
	{ // not set | not cached | has default
		v, ok := getOptional[int](FOO, "10", true, true, toInt)
		require.True(ok)
		require.Equal(10, v)
	}
	{ // not set | cached     | no default
		v, ok := getOptional[int](FOO, "", false, true, toInt)
		require.True(ok)
		require.Equal(10, v)
	}
	{ // set     | cached     | has default
		os.Setenv(FOO, foo)
		v, ok := getOptional[int](FOO, "20", true, true, toInt)
		require.True(ok)
		require.Equal(10, v)
	}
	{ // set     | not cached | no default
		os.Setenv(BAR, bar)
		v, ok := getOptional[string](BAR, "", false, true, toString)
		require.True(ok)
		require.Equal(bar, v)
	}
}

func (suite *EnvSuite) TestGet() {
	os.Setenv(FOO, foo)
	v := get[string](FOO, "", false, true, toString)
	require.Equal(suite.T(), foo, v)
}

func (suite *EnvSuite) TestLogInit() {
	require.Equal(suite.T(), 2, len(cache.values))
}

// ------------------------------------------------------------------------------------------------
// Individual Tests
// ------------------------------------------------------------------------------------------------

func TestParse(t *testing.T) {
	v := parse("true", toBool)
	require.True(t, v)
}

func TestParseFail(t *testing.T) {
	requireExit(t, "TestParseFail", func() {
		parse("not int", toInt)
	})
}

func TestGetFail(t *testing.T) {
	os.Unsetenv(FOO)
	requireExit(t, "TestGetFail", func() {
		get[string](FOO, "", false, true, toString)
	})
}

// ------------------------------------------------------------------------------------------------
// Auxiliary
// ------------------------------------------------------------------------------------------------

var (
	FOO = "FOO"
	BAR = "BAR"
	BAZ = "BAZ"

	foo = "foo"
	bar = "bar"
)

// For testing code that terminates with os.Exit(1).
func requireExit(t *testing.T, name string, test func()) {
	if os.Getenv("IS_TEST") == "1" {
		test()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run="+name)
	cmd.Env = append(os.Environ(), "IS_TEST=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	}
	t.Fatalf("ran with err %v, want exit(1)", err)
}

func getMockedLog(buffer bytes.Buffer) []string {
	return strings.Split(buffer.String(), "\n")[1:]
}
