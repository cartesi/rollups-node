// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package httpclient

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	for _, raw := range []string{"http://localhost:8545", "https://u:p@example.com/path?q=key", "http://[::1]:8545/rpc"} {
		u, err := Validate(raw)
		require.NoError(t, err)
		require.NotEmpty(t, u.Hostname())
	}
	for _, raw := range []string{
		"", "SECRET.sock", "wss://host/SECRET", "ws://host/SECRET", "ipc://host/SECRET",
		"https:///SECRET", "http://:8545/SECRET", "http:SECRET", "http://host/%SECRET", "http://host:SECRET",
	} {
		_, err := Validate(raw)
		require.Error(t, err, raw)
		require.NotContains(t, err.Error(), "SECRET")
	}
}
