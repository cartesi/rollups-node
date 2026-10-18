// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRootBondRecoveryWithoutCandidatesDoesNotReadChain(t *testing.T) {
	f := newRecoveryPortFixture(t)
	f.app.ForecloseBlock = 90
	f.repo.recoveries = nil
	call, err := f.plan(t.Context())
	require.NoError(t, err)
	require.Nil(t, call)
	require.Empty(t, f.adapter.reads)
	require.Empty(t, f.backend.calls)
	require.Empty(t, f.backend.published)
}
