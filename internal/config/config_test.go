// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"os"
	"os/exec"
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
}

// ------------------------------------------------------------------------------------------------

func (suite *EnvSuite) TestRead() {
	require := suite.Require()

	// test specific setup
	cache.values = make(map[string]string)
	cacheLen := len(cache.values)
	require.Equal(cacheLen, len(cache.values))

	name, value := FOO, foo
	{ // not initialized
		s, ok := read(name)
		require.Equal("", s)
		require.False(ok)
		require.Equal(cacheLen, len(cache.values))
	}
	{ // initialized
		os.Setenv(name, value)
		s, ok := read(name)
		require.True(ok)
		require.Equal(value, s)
		require.Equal(cacheLen+1, len(cache.values))
	}
	{ // cached
		os.Setenv(name, "another foo")
		s, ok := read(name)
		require.True(ok)
		require.Equal(value, s)
		require.Equal(cacheLen+1, len(cache.values))
	}
	{ // empty string
		os.Setenv(BAZ, "")
		s, ok := read(BAZ)
		require.True(ok)
		require.Equal("", s)
		require.Equal(cacheLen+2, len(cache.values))
	}
}

func (suite *EnvSuite) TestGetOptional() {
	require := suite.Require()
	{ // not set | not cached | no default
		v := getOptional[int](FOO, "", false, toInt)
		require.Nil(v)
	}
	{ // not set | not cached | has default
		v := getOptional[int](FOO, "10", true, toInt)
		require.NotNil(v)
		require.Equal(10, *v)
	}
	{ // not set | cached     | no default
		v := getOptional[int](FOO, "", false, toInt)
		require.NotNil(v)
		require.Equal(10, *v)
	}
	{ // set     | cached     | has default
		os.Setenv(FOO, foo)
		v := getOptional[int](FOO, "20", true, toInt)
		require.NotNil(v)
		require.Equal(10, *v)
	}
	{ // set     | not cached | no default
		os.Setenv(BAR, bar)
		v := getOptional[string](BAR, "", false, toString)
		require.NotNil(v)
		require.Equal(bar, *v)
	}
}

func (suite *EnvSuite) TestGet() {
	os.Setenv(FOO, foo)
	v := get[string](FOO, "", false, toString)
	require.Equal(suite.T(), foo, *v)
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
		get[string](FOO, "", false, toString)
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
