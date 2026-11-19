// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package register

import (
	"errors"
	"math/big"
	"net/http/httptest"
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/ethutiltest"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

const testConsensusAddress = "0x1000000000000000000000000000000000000001"

// newConsensusEndpoint serves backend as a JSON-RPC endpoint and returns its URL.
func newConsensusEndpoint(t *testing.T, backend *ethutiltest.ConsensusRPC) string {
	t.Helper()
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", backend))
	t.Cleanup(server.Stop)
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	return httpServer.URL
}

func newConsensusClient(t *testing.T, backend *ethutiltest.ConsensusRPC) *ethclient.Client {
	t.Helper()
	client, err := ethclient.Dial(newConsensusEndpoint(t, backend))
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return client
}

// TestDetectConsensusType covers the register call site of the consensus detector.
// The defect this PR fixes was here: a failed probe used to store Authority. Every
// failure below must therefore return no type at all.
func TestDetectConsensusType(t *testing.T) {
	tests := []struct {
		name           string
		backend        *ethutiltest.ConsensusRPC
		want           model.Consensus
		wantErr        string
		wantValidation bool
	}{
		{
			name: "authority",
			backend: &ethutiltest.ConsensusRPC{
				SupportsInterface: ethutiltest.ERC165Only(ethutiltest.IConsensusInterfaceID),
			},
			want: model.Consensus_Authority,
		},
		{
			name: "quorum",
			backend: &ethutiltest.ConsensusRPC{
				SupportsInterface: ethutiltest.ERC165Only(
					ethutiltest.IQuorumInterfaceID, ethutiltest.IConsensusInterfaceID),
				NumOfValidators: big.NewInt(1),
			},
			want: model.Consensus_Quorum,
		},
		{
			name: "prt",
			backend: &ethutiltest.ConsensusRPC{
				SupportsInterface: ethutiltest.ERC165Only(ethutiltest.IDataProviderInterfaceID),
			},
			want: model.Consensus_PRT,
		},
		{
			name: "read error never selects authority",
			backend: &ethutiltest.ConsensusRPC{
				SupportsInterface:    ethutiltest.ERC165Only(),
				SupportsInterfaceErr: map[[4]byte]error{ethutiltest.IQuorumInterfaceID: errors.New("provider 503")},
			},
			wantErr: "supportsInterface(IQuorum)",
		},
		{
			name:    "contract without erc165",
			backend: &ethutiltest.ConsensusRPC{},
			wantErr: "does not implement ERC-165",
		},
		{
			name: "quorum without validators",
			backend: &ethutiltest.ConsensusRPC{
				SupportsInterface: ethutiltest.ERC165Only(
					ethutiltest.IQuorumInterfaceID, ethutiltest.IConsensusInterfaceID),
				NumOfValidators: big.NewInt(0),
			},
			wantErr:        "zero validators",
			wantValidation: true,
		},
		{
			name: "consensus version the node cannot operate",
			backend: &ethutiltest.ConsensusRPC{
				SupportsInterface: ethutiltest.ERC165Only(ethutiltest.IConsensusV220InterfaceID),
			},
			wantErr:        "unsupported IConsensus contract version",
			wantValidation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newConsensusClient(t, tt.backend)

			got, err := detectConsensusType(t.Context(), client, common.HexToAddress(testConsensusAddress))

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Empty(t, got, "a failed detection or validation must never yield a consensus type")
				// Validation failures are reported separately from detection failures.
				var validationErr consensusValidationError
				require.Equal(t, tt.wantValidation, errors.As(err, &validationErr))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestGetConsensusTypeUsesConfiguredEndpoint keeps the dialing path exercised.
func TestGetConsensusTypeUsesConfiguredEndpoint(t *testing.T) {
	config.SetDefaults()
	t.Setenv(config.BLOCKCHAIN_HTTP_ENDPOINT, newConsensusEndpoint(t, &ethutiltest.ConsensusRPC{
		SupportsInterface: ethutiltest.ERC165Only(ethutiltest.IConsensusInterfaceID),
	}))

	got, err := getConsensusType(t.Context(), common.HexToAddress(testConsensusAddress))
	require.NoError(t, err)
	require.Equal(t, model.Consensus_Authority, got)
}
