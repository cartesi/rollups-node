// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package integration

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessRSS(t *testing.T) {
	if _, err := os.Stat("/proc/self/status"); err != nil {
		t.Skip("RSS measurement requires Linux procfs")
	}
	rss, err := processRSS(os.Getpid())
	require.NoError(t, err)
	require.Positive(t, rss)
	_, err = processRSS(-1)
	require.Error(t, err, "missing processes must fail measurement")
}
