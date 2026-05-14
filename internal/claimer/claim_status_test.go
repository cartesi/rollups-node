// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateEpochStagedFromClaimStatus_NilStagingBlock_ReturnsError(t *testing.T) {
	m, r, _ := newServiceMock()
	defer r.AssertExpectations(t)

	app := makeApplication()
	epoch := makeComputedEpoch(app, 1)
	claim := makeClaimStatus(claimStatusStaged, epoch, 0)

	_, err := m.updateEpochStagedFromClaimStatus(app, epoch, claim, "test")

	require.Error(t, err)
	require.Contains(t, err.Error(), "nil staging block")
}
