// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package register

import (
	"errors"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/stretchr/testify/require"
)

type quorumConsensusProbeStub struct {
	numOfValidators *big.Int
	err             error
	called          bool
}

func (p *quorumConsensusProbeStub) NumOfValidators(_ *bind.CallOpts) (*big.Int, error) {
	p.called = true
	return p.numOfValidators, p.err
}

func TestConsensusTypeFromQuorumProbe_PRTSkipsProbe(t *testing.T) {
	probe := &quorumConsensusProbeStub{
		numOfValidators: big.NewInt(3),
	}

	consensusType, err := consensusTypeFromQuorumProbe(true, probe)

	require.NoError(t, err)
	require.Equal(t, model.Consensus_PRT, consensusType)
	require.False(t, probe.called)
}

func TestConsensusTypeFromQuorumProbe_AuthorityWhenProbeFails(t *testing.T) {
	probe := &quorumConsensusProbeStub{
		err: errors.New("execution reverted"),
	}

	consensusType, err := consensusTypeFromQuorumProbe(false, probe)

	require.NoError(t, err)
	require.Equal(t, model.Consensus_Authority, consensusType)
	require.True(t, probe.called)
}

func TestConsensusTypeFromQuorumProbe_QuorumWhenValidatorsExist(t *testing.T) {
	probe := &quorumConsensusProbeStub{
		numOfValidators: big.NewInt(3),
	}

	consensusType, err := consensusTypeFromQuorumProbe(false, probe)

	require.NoError(t, err)
	require.Equal(t, model.Consensus_Quorum, consensusType)
	require.True(t, probe.called)
}

func TestConsensusTypeFromQuorumProbe_RejectsZeroValidatorQuorum(t *testing.T) {
	probe := &quorumConsensusProbeStub{
		numOfValidators: big.NewInt(0),
	}

	_, err := consensusTypeFromQuorumProbe(false, probe)

	require.ErrorContains(t, err, "zero validators")
	require.True(t, probe.called)
}
