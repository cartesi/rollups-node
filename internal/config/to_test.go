// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToBool(t *testing.T) {
	require := require.New(t)
	{ // true
		v, err := toBool("true")
		require.Nil(err)
		require.True(v)
	}
	{ // false
		v, err := toBool("false")
		require.Nil(err)
		require.False(v)
	}
	{ // fail
		_, err := toBool("not bool")
		require.NotNil(err)
	}
}

func TestToInt(t *testing.T) {
	require := require.New(t)
	{ // ok
		v, err := toInt("10")
		require.Nil(err)
		require.Equal(int(10), v)
	}
	{ // fail
		_, err := toInt("not int")
		require.NotNil(err)
	}
}

func TestToInt64(t *testing.T) {
	require := require.New(t)
	{ // ok
		v, err := toInt64("10")
		require.Nil(err)
		require.Equal(int64(10), v)
	}
	{ // fail
		_, err := toInt64("not int64")
		require.NotNil(err)
	}
}

func TestToString(t *testing.T) {
	s := "nugget"
	v := parse(s, toString)
	require.Equal(t, s, v)
}

func TestToDuration(t *testing.T) {
	require := require.New(t)
	{ // ok
		v, err := toDuration("60")
		require.Nil(err)
		require.Equal(float64(60), v.Seconds())
	}
	{ // fail
		_, err := toDuration("not duration")
		require.NotNil(err)
	}
}

func TestToLogLevel(t *testing.T) {
	require := require.New(t)
	{ // info
		v, err := toLogLevel("debug")
		require.Nil(err)
		require.Equal(LogLevelDebug, v)
	}
	{ // debug
		v, err := toLogLevel("info")
		require.Nil(err)
		require.Equal(LogLevelInfo, v)
	}
	{ // warning
		v, err := toLogLevel("warning")
		require.Nil(err)
		require.Equal(LogLevelWarning, v)
	}
	{ // error (not an error, but LogLevelError)
		v, err := toLogLevel("error")
		require.Nil(err)
		require.Equal(LogLevelError, v)
	}
	{ // fail
		_, err := toLogLevel("not log level")
		require.NotNil(err)
	}
}
