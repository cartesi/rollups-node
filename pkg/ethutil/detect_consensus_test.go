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
	"github.com/ethereum/go-ethereum/crypto"
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

// withERC165 returns the answers of a contract that passes the ERC-165 procedure and
// reports true for the given interface IDs. Every other query answers false.
func withERC165(ids ...[4]byte) map[[4]byte]bool {
	supported := map[[4]byte]bool{erc165InterfaceID: true}
	for _, id := range ids {
		supported[id] = true
	}
	return supported
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
			supported: withERC165(iDataProviderInterfaceID),
			want:      ConsensusPRT,
		},
		{
			name:        "quorum v3",
			supported:   withERC165(iQuorumInterfaceID, iConsensusInterfaceIDv30),
			want:        ConsensusQuorum,
			wantVersion: CurrentIConsensusVersion,
		},
		{
			name:      "quorum legacy",
			supported: withERC165(iQuorumInterfaceID),
			want:      ConsensusQuorum,
		},
		{
			name:        "authority v3",
			supported:   withERC165(iConsensusInterfaceIDv30),
			want:        ConsensusAuthority,
			wantVersion: CurrentIConsensusVersion,
		},
		{
			name:        "authority v2.2.0",
			supported:   withERC165(iConsensusInterfaceIDv220),
			want:        ConsensusAuthority,
			wantVersion: "v2.2.0",
		},
		{
			name:        "authority v2.1.x",
			supported:   withERC165(iConsensusInterfaceIDv21x),
			want:        ConsensusAuthority,
			wantVersion: "v2.1.x",
		},
		{
			name:      "no supported interface",
			supported: withERC165(),
			wantErr:   "does not implement",
		},
		{
			name:    "contract without erc165",
			wantErr: "does not implement ERC-165",
		},
		{
			name:      "fallback answers true to every call",
			supported: withERC165(erc165InvalidInterfaceID, iDataProviderInterfaceID),
			wantErr:   "supportsInterface(0xffffffff) returned true",
		},
		{
			name:    "erc165 probe failure",
			errs:    map[[4]byte]error{erc165InterfaceID: errors.New("provider 503")},
			wantErr: "supportsInterface(ERC-165)",
		},
		{
			name:      "invalid interface id probe failure",
			supported: withERC165(),
			errs:      map[[4]byte]error{erc165InvalidInterfaceID: context.DeadlineExceeded},
			wantErr:   "supportsInterface(0xffffffff)",
		},
		{
			name:      "dave probe failure",
			supported: withERC165(),
			errs:      map[[4]byte]error{iDataProviderInterfaceID: &rpc.HTTPError{StatusCode: 429}},
			wantErr:   "supportsInterface(IDataProvider)",
		},
		{
			name:      "quorum probe failure",
			supported: withERC165(),
			errs:      map[[4]byte]error{iQuorumInterfaceID: errors.New("provider 503")},
			wantErr:   "supportsInterface(IQuorum)",
		},
		{
			name:      "authority probe failure",
			supported: withERC165(),
			errs:      map[[4]byte]error{iConsensusInterfaceIDv30: context.Canceled},
			wantErr:   "supportsInterface(IConsensus)",
		},
		{
			name:      "quorum version probe failure",
			supported: withERC165(iQuorumInterfaceID),
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

func selector(sig string) [4]byte {
	hash := crypto.Keccak256([]byte(sig))
	var id [4]byte
	copy(id[:], hash[:4])
	return id
}

func xorSelectors(sigs ...string) (id [4]byte) {
	for _, sig := range sigs {
		s := selector(sig)
		for i := range id {
			id[i] ^= s[i]
		}
	}
	return id
}

// TestInterfaceIDsMatchSoliditySignatures pins every interface ID to a literal
// value recomputed from the signatures declared in the pinned Solidity sources
// (rollups-contracts v2.1.1, v2.2.0 and v3.0.0-beta.1; dave v3.0.0-beta.1 for
// IDataProvider). A test that only compares the constants with themselves would
// stay green if a constant drifted, and a wrong ID silently selects the wrong
// consensus type on chain.
func TestInterfaceIDsMatchSoliditySignatures(t *testing.T) {
	const getEpochLength = "getEpochLength()"

	// IQuorum declares these seven functions; IConsensus declarations are inherited
	// and do not count towards type(IQuorum).interfaceId.
	iQuorumSignatures := []string{
		"numOfValidators()",
		"validatorId(address)",
		"validatorById(uint256)",
		"numOfValidatorsInFavorOfAnyClaimInEpoch(address,uint256)",
		"isValidatorInFavorOfAnyClaimInEpoch(address,uint256,uint256)",
		"numOfValidatorsInFavorOf(address,uint256,bytes32)",
		"isValidatorInFavorOf(address,uint256,bytes32,uint256)",
	}

	tests := []struct {
		name string
		want [4]byte
		got  [4]byte
		sigs []string
	}{
		{
			name: "IDataProvider",
			want: [4]byte{0x7a, 0x96, 0xf4, 0x80},
			got:  iDataProviderInterfaceID,
			sigs: []string{"provideMerkleRootOfInput(uint256,bytes)"},
		},
		{
			name: "IQuorum",
			want: [4]byte{0x3c, 0x92, 0x5a, 0x62},
			got:  iQuorumInterfaceID,
			sigs: iQuorumSignatures,
		},
		{
			name: "IConsensus v3.0.0-beta.1",
			want: [4]byte{0x13, 0x0c, 0x71, 0xe1},
			got:  iConsensusInterfaceIDv30,
			sigs: []string{
				"submitClaim(address,uint256,bytes32,((bytes32,bytes32[]),(bytes32,bytes32[]),(bytes32,bytes32[])))",
				"acceptClaim(address,uint256,bytes32)",
				getEpochLength,
				"getClaimStagingPeriod()",
				"getNumberOfAcceptedClaims(address)",
				"getNumberOfStagedClaims(address)",
				"getNumberOfSubmittedClaims(address)",
				"getClaim(address,uint256,bytes32)",
			},
		},
		{
			name: "IConsensus v2.2.0",
			want: [4]byte{0x90, 0xb2, 0xf3, 0x46},
			got:  iConsensusInterfaceIDv220,
			sigs: []string{
				"submitClaim(address,uint256,bytes32)",
				getEpochLength,
				"getNumberOfAcceptedClaims()",
				"getNumberOfSubmittedClaims()",
			},
		},
		{
			name: "IConsensus v2.1.1",
			want: [4]byte{0x7e, 0xec, 0xfc, 0xec},
			got:  iConsensusInterfaceIDv21x,
			sigs: []string{
				"submitClaim(address,uint256,bytes32)",
				getEpochLength,
				"getNumberOfAcceptedClaims()",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, xorSelectors(tt.sigs...),
				"XOR of the Solidity signatures (a contracts bump that changes an interface fails here)")
			require.Equal(t, tt.want, tt.got, "package constant")
		})
	}
}

func TestValidateConsensusVersion(t *testing.T) {
	tests := []struct {
		name     string
		detected ConsensusType
		version  string
		wantErr  string
	}{
		{name: "current authority", detected: ConsensusAuthority, version: CurrentIConsensusVersion},
		{name: "current quorum", detected: ConsensusQuorum, version: CurrentIConsensusVersion},
		{name: "prt is exempt", detected: ConsensusPRT},
		{name: "legacy authority", detected: ConsensusAuthority, version: "v2.2.0",
			wantErr: "unsupported IConsensus contract version"},
		{name: "legacy quorum", detected: ConsensusQuorum,
			wantErr: "unsupported IConsensus contract version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConsensusVersion(tt.detected, tt.version)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
