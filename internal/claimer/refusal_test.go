// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestClaimerHardCapabilityFailureAffectsReadinessWithoutAHealthWrite(t *testing.T) {
	s, _, blockchain := newServiceMock(t)
	s.submissionEnabled = false
	blockchain.On("getDefaultBlockNumber", mock.Anything).Return((*big.Int)(nil), txsender.ErrUnsupported).Once()
	require.True(t, s.Ready())
	progress, err := s.Tick(t.Context())
	require.False(t, progress)
	require.ErrorIs(t, err, txsender.ErrUnsupported)
	require.True(t, s.Ready(), "a retryable unsupported read is not a permanent prerequisite fault")
	blockchain.AssertExpectations(t)
}
