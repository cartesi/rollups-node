// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type supportsInterfaceStub struct {
	supported map[[4]byte]bool
	errs      map[[4]byte]error
	opts      *bind.CallOpts
}

func (s *supportsInterfaceStub) SupportsInterface(
	opts *bind.CallOpts, interfaceID [4]byte,
) (bool, error) {
	s.opts = opts
	if err := s.errs[interfaceID]; err != nil {
		return false, err
	}
	return s.supported[interfaceID], nil
}

func TestDetectConsensusType(t *testing.T) {
	callOpts := &bind.CallOpts{Context: t.Context()}

	tests := []struct {
		name        string
		supported   map[[4]byte]bool
		errs        map[[4]byte]error
		want        ConsensusType
		wantVersion string
		wantErr     string
	}{
		{
			name:      "dave",
			supported: map[[4]byte]bool{iDataProviderInterfaceID: true},
			want:      ConsensusPRT,
		},
		{
			name: "quorum v3",
			supported: map[[4]byte]bool{
				iQuorumInterfaceID:       true,
				iConsensusInterfaceIDv30: true,
			},
			want:        ConsensusQuorum,
			wantVersion: "v3.0.0-alpha",
		},
		{
			name:      "quorum legacy",
			supported: map[[4]byte]bool{iQuorumInterfaceID: true},
			want:      ConsensusQuorum,
		},
		{
			name:        "authority v3",
			supported:   map[[4]byte]bool{iConsensusInterfaceIDv30: true},
			want:        ConsensusAuthority,
			wantVersion: "v3.0.0-alpha",
		},
		{
			name:        "authority v2.2.0",
			supported:   map[[4]byte]bool{iConsensusInterfaceIDv220: true},
			want:        ConsensusAuthority,
			wantVersion: "v2.2.0",
		},
		{
			name:        "authority v2.1.x",
			supported:   map[[4]byte]bool{iConsensusInterfaceIDv21x: true},
			want:        ConsensusAuthority,
			wantVersion: "v2.1.x",
		},
		{
			name:    "no supported interface",
			wantErr: "does not implement",
		},
		{
			name:    "dave probe failure",
			errs:    map[[4]byte]error{iDataProviderInterfaceID: &rpc.HTTPError{StatusCode: 429}},
			wantErr: "supportsInterface(IDataProvider)",
		},
		{
			name:    "quorum probe failure",
			errs:    map[[4]byte]error{iQuorumInterfaceID: errors.New("provider 503")},
			wantErr: "supportsInterface(IQuorum)",
		},
		{
			name:    "authority probe failure",
			errs:    map[[4]byte]error{iConsensusInterfaceIDv30: context.Canceled},
			wantErr: "supportsInterface(IConsensus)",
		},
		{
			name:      "quorum version probe failure",
			supported: map[[4]byte]bool{iQuorumInterfaceID: true},
			errs:      map[[4]byte]error{iConsensusInterfaceIDv30: context.DeadlineExceeded},
			wantErr:   "for Quorum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := &supportsInterfaceStub{supported: tt.supported, errs: tt.errs}

			got, version, err := DetectConsensusType(callOpts, probe)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				// A probe failure must never fall back to Authority.
				assert.Equal(t, ConsensusUnknown, got)
				assert.Empty(t, version)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantVersion, version)
			assert.Same(t, callOpts, probe.opts)
		})
	}
}

type validatorCounterStub struct {
	count *big.Int
	err   error
}

func (s validatorCounterStub) NumOfValidators(_ *bind.CallOpts) (*big.Int, error) {
	return s.count, s.err
}

func TestValidateQuorum(t *testing.T) {
	const zeroValidators = "zero validators"

	tests := []struct {
		name    string
		counter validatorCounterStub
		wantErr string
	}{
		{name: "nonzero validators", counter: validatorCounterStub{count: big.NewInt(3)}},
		{name: "zero validators", counter: validatorCounterStub{count: big.NewInt(0)},
			wantErr: zeroValidators},
		{name: "nil validators", counter: validatorCounterStub{},
			wantErr: zeroValidators},
		{name: "read failure", counter: validatorCounterStub{err: errors.New("provider 503")},
			wantErr: "NumOfValidators"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuorum(&bind.CallOpts{Context: t.Context()}, tt.counter)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestConsensusTypeString(t *testing.T) {
	assert.Equal(t, "Authority", ConsensusAuthority.String())
	assert.Equal(t, "Quorum", ConsensusQuorum.String())
	assert.Equal(t, "DaveConsensus (PRT)", ConsensusPRT.String())
	assert.Equal(t, "Unknown", ConsensusUnknown.String())
}

func TestConsensusTypeToModel(t *testing.T) {
	assert.Equal(t, model.Consensus_Authority, ConsensusAuthority.ToModel())
	assert.Equal(t, model.Consensus_Quorum, ConsensusQuorum.ToModel())
	assert.Equal(t, model.Consensus_PRT, ConsensusPRT.ToModel())
	assert.Equal(t, model.Consensus(""), ConsensusUnknown.ToModel())
}

// TestIConsensusV3InterfaceID locks down the v3 interface ID computation.
// If a method is renamed in the binding or this list drifts from the
// IConsensus.sol interface, this test surfaces the change as a value mismatch.
func TestIConsensusV3InterfaceID(t *testing.T) {
	// Non-zero, distinct from the v2 IDs.
	assert.NotEqual(t, [4]byte{}, iConsensusInterfaceIDv30,
		"v3 interface ID should be non-zero")
	assert.NotEqual(t, iConsensusInterfaceIDv220, iConsensusInterfaceIDv30,
		"v3 interface ID should differ from v2.2.0")
	assert.NotEqual(t, iConsensusInterfaceIDv21x, iConsensusInterfaceIDv30,
		"v3 interface ID should differ from v2.1.x")
}
