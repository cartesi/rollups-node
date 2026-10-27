// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/stretchr/testify/require"
)

const (
	testRequiredMarchID = uint64(22)
	testOtherMarchID    = uint64(21)
	identityTimeout     = 2 * time.Second
)

func TestLinkedEmulatorIdentityMatchesPins(t *testing.T) {
	identity, err := LinkedEmulatorIdentity()
	require.NoError(t, err)
	require.Equal(t, testRequiredMarchID, identity.HeaderMarchID)
	require.Equal(t, testRequiredMarchID, identity.RuntimeMarchID)
	require.Equal(t, model.UarchPristineStateHash(), identity.UarchPristineStateHash)
	require.NoError(t, ValidateEmulatorMarchID())
	require.NoError(t, ValidateEmulatorComputationHashLimits())
}
