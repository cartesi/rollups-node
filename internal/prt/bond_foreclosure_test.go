// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRootBondRecoveryWithoutCandidatesDoesNotReadChain(t *testing.T) {
	app := prtForeclosedApp(1, 10)
	factory := &adapterFactoryMock{}
	service := newRootBondTestService(common.HexToAddress("0x600"), factory)

	require.NoError(t, service.recoverRootBonds(t.Context(), app, 20))
	require.Empty(t, service.rootBondRecoveries)
	factory.AssertNotCalled(t, "CreateDaveConsensusAdapter", mock.Anything)
	factory.AssertNotCalled(t, "CreateTournamentAdapter", mock.Anything)
}
