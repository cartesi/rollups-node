// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"errors"
	"math/big"
	"net/http/httptest"
	"testing"

	"github.com/cartesi/rollups-node/internal/ethutiltest"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

const testedConsensusAddress = "0x1000000000000000000000000000000000000001"

func newConsensusClient(t *testing.T, backend *ethutiltest.ConsensusRPC) *ethclient.Client {
	t.Helper()
	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", backend))
	t.Cleanup(server.Stop)
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	client, err := ethclient.Dial(httpServer.URL)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return client
}

// TestCustomConsensus covers the deploy call site of the consensus detector. The
// fake answers each supportsInterface query by the interface ID it decodes from
// the call data, so a wrong or reordered probe cannot pass.
func TestCustomConsensus(t *testing.T) {
	tests := []struct {
		name    string
		backend *ethutiltest.ConsensusRPC
		want    model.Consensus
		wantErr string
	}{
		{
			name: "detects an authority",
			backend: &ethutiltest.ConsensusRPC{
				SupportsInterface: ethutiltest.ERC165Only(ethutiltest.IConsensusInterfaceID),
			},
			want: model.Consensus_Authority,
		},
		{
			name: "detects a quorum",
			backend: &ethutiltest.ConsensusRPC{
				SupportsInterface: ethutiltest.ERC165Only(
					ethutiltest.IQuorumInterfaceID, ethutiltest.IConsensusInterfaceID),
				NumOfValidators: big.NewInt(3),
			},
			want: model.Consensus_Quorum,
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
			name: "prt must go through --prt",
			backend: &ethutiltest.ConsensusRPC{
				SupportsInterface: ethutiltest.ERC165Only(ethutiltest.IDataProviderInterfaceID),
			},
			wantErr: "use --prt",
		},
		{
			name: "quorum without validators",
			backend: &ethutiltest.ConsensusRPC{
				SupportsInterface: ethutiltest.ERC165Only(
					ethutiltest.IQuorumInterfaceID, ethutiltest.IConsensusInterfaceID),
				NumOfValidators: big.NewInt(0),
			},
			wantErr: "zero validators",
		},
		{
			name: "consensus version the node cannot operate",
			backend: &ethutiltest.ConsensusRPC{
				SupportsInterface: ethutiltest.ERC165Only(ethutiltest.IConsensusV220InterfaceID),
			},
			wantErr: "unsupported IConsensus contract version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.backend.EpochLength = 10
			tt.backend.ClaimStagingPeriod = 20
			client := newConsensusClient(t, tt.backend)

			address, epochLength, stagingPeriod, consensusType, err := customConsensus(
				t.Context(), client, testedConsensusAddress)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Empty(t, consensusType, "a failed detection or validation must never yield a consensus type")
				require.Equal(t, common.Address{}, address)
				require.Zero(t, epochLength)
				require.Zero(t, stagingPeriod)
				return
			}
			require.NoError(t, err)
			require.Equal(t, common.HexToAddress(testedConsensusAddress), address)
			require.Equal(t, uint64(10), epochLength)
			require.Equal(t, uint64(20), stagingPeriod)
			require.Equal(t, tt.want, consensusType)
		})
	}
}

// TestApplyDeploymentResult protects the removal of the silent Authority default:
// an empty detected type must refuse the deployment, never store Authority.
func TestApplyDeploymentResult(t *testing.T) {
	tests := []struct {
		name    string
		result  ethutil.IApplicationDeploymentResult
		want    model.Consensus
		wantErr string
	}{
		{
			name: "self-hosted deployment is an authority",
			result: &ethutil.SelfhostedApplicationDeploymentResult{
				Deployment: &ethutil.SelfhostedApplicationDeployment{},
			},
			want: model.Consensus_Authority,
		},
		{
			name: "detected authority is kept",
			result: &ethutil.ApplicationDeploymentResult{
				Deployment: &ethutil.ApplicationDeployment{ConsensusType: "AUTHORITY"},
			},
			want: model.Consensus_Authority,
		},
		{
			name: "detected quorum is kept",
			result: &ethutil.ApplicationDeploymentResult{
				Deployment: &ethutil.ApplicationDeployment{ConsensusType: "QUORUM"},
			},
			want: model.Consensus_Quorum,
		},
		{
			name: "missing consensus type is refused",
			result: &ethutil.ApplicationDeploymentResult{
				Deployment: &ethutil.ApplicationDeployment{},
			},
			wantErr: "consensus type of the deployed application was not detected",
		},
		{
			name: "prt deployment is prt",
			result: &ethutil.PRTApplicationDeploymentResult{
				Deployment: &ethutil.PRTApplicationDeployment{},
			},
			want: model.Consensus_PRT,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The application starts without a consensus type, as in runDeployApplication.
			application := model.Application{}
			err := applyDeploymentResult(&application, tt.result)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Empty(t, application.ConsensusType)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, application.ConsensusType)
		})
	}
}
