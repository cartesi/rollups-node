// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNodeTelemetryAddresses(t *testing.T) {
	for _, test := range []struct {
		name       string
		configured string
		want       string
	}{
		{name: "default", want: "localhost:10000"},
		{name: "configured port", configured: ":19000", want: "localhost:19000"},
		{name: "configured host", configured: "127.0.0.1:19000", want: "127.0.0.1:19000"},
		{name: "configured IPv6", configured: "[::1]:19000", want: "[::1]:19000"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("CARTESI_NODE_TELEMETRY_ADDRESS", test.configured)
			require.Equal(t, test.want, nodeTelemetryAddress())
			require.Equal(t, []string{
				test.want,
				"localhost:10001", "localhost:10002", "localhost:10003", "localhost:10004", "localhost:10005", "localhost:10006",
				"localhost:10011", "localhost:10012",
			}, multiNodeListenAddresses(), "only the stale standalone endpoint follows the node configuration")
		})
	}
}

func TestMultiNodePreflightConfiguredAddress(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })
	address := listener.Addr().String()
	t.Setenv("CARTESI_NODE_TELEMETRY_ADDRESS", address)

	require.False(t, nodePortAvailable(), "standalone detection must check the configured listener")
	require.EqualError(t, preflightMultiNodePorts(),
		"cannot start multiprocess node: "+address+" is already in use")
}
