// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSetupPersistentConfigUsesSerializedRegistration(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, concurrent := range []bool{false, true} {
			t.Run(fmt.Sprintf("enabled=%t/concurrent=%t", enabled, concurrent), func(t *testing.T) {
				s, _ := newPRTServiceMock()
				repo := &prtBlockPolicyCreateRepository{}
				var signer common.Address
				if enabled {
					signer = common.HexToAddress("0x600")
				}
				requested := PersistentConfig{
					DefaultBlock: model.DefaultBlock_Finalized, ChainID: 42, ClaimSubmissionEnabled: enabled,
					ResolvedSignerAddress: signer,
				}
				stored := requested
				if concurrent {
					stored.ClaimSubmissionEnabled = !enabled
				}
				registrationErr := stored.CheckRequested(requested)
				repo.On("RegisterSubmitter", mock.Anything, PrtConfigKey, requested).
					Return(stored, registrationErr).Once()
				got, err := s.setupPersistentConfig(t.Context(), &config.PrtConfig{
					BlockchainId: 42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
					FeatureClaimSubmissionEnabled: enabled,
				}, repo, signer)
				if concurrent {
					require.Nil(t, got)
					require.ErrorContains(t, err, "claim submission mode mismatch")
				} else {
					require.NoError(t, err)
					require.Equal(t, requested, *got)
				}
				repo.AssertNotCalled(t, "LoadNodeConfigRaw", mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "InitializeNodeConfigRaw", mock.Anything, mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "SaveNodeConfigRaw", mock.Anything, mock.Anything, mock.Anything)
				repo.AssertExpectations(t)
			})
		}
	}
}

func TestCreateRejectsInvalidRegisteredConfig(t *testing.T) {
	for _, test := range []struct {
		name             string
		raw              string
		requestedEnabled bool
		want             string
	}{
		{"chain mismatch", `{"ChainID":2,"DefaultBlock":"FINALIZED","ClaimSubmissionEnabled":true,
            "ResolvedSignerAddress":"0x0000000000000000000000000000000000000600"}`, true,
			"chain ID mismatch: database=2, configured=42"},
		{"policy mismatch", `{"ChainID":42,"DefaultBlock":"LATEST","ClaimSubmissionEnabled":true,
            "ResolvedSignerAddress":"0x0000000000000000000000000000000000000600"}`, true,
			"observation policy mismatch: database=LATEST, configured=FINALIZED"},
		{"enable submission", `{"ChainID":42,"DefaultBlock":"FINALIZED","ClaimSubmissionEnabled":false}`, true,
			"claim submission mode mismatch: database=false, configured=true"},
		{"disable submission", `{"ChainID":42,"DefaultBlock":"FINALIZED","ClaimSubmissionEnabled":true,
            "ResolvedSignerAddress":"0x0000000000000000000000000000000000000600"}`, false,
			"claim submission mode mismatch: database=true, configured=false"},
		{"missing mode", `{"ChainID":42,"DefaultBlock":"FINALIZED"}`, true, "non-null ClaimSubmissionEnabled"},
		{"null mode", `{"ChainID":42,"DefaultBlock":"FINALIZED","ClaimSubmissionEnabled":null}`, true,
			"non-null ClaimSubmissionEnabled"},
		{"null config", `null`, true, "non-null DefaultBlock and ChainID"},
		{"invalid policy", `{"ChainID":42,"DefaultBlock":"invalid","ClaimSubmissionEnabled":true,
            "ResolvedSignerAddress":"0x0000000000000000000000000000000000000600"}`, true, "invalid DefaultBlock"},
	} {
		t.Run(test.name, func(t *testing.T) {
			signer := common.HexToAddress("0x600")
			registered := false
			repo := &prtBlockPolicyCreateRepository{register: func(
				_ context.Context, key string, requested PersistentConfig,
			) (PersistentConfig, error) {
				registered = true
				require.Equal(t, PrtConfigKey, key)
				if test.requestedEnabled {
					require.Equal(t, signer, requested.ResolvedSignerAddress)
				} else {
					require.Zero(t, requested.ResolvedSignerAddress)
				}
				var stored PersistentConfig
				if err := json.Unmarshal([]byte(test.raw), &stored); err != nil {
					return stored, err
				}
				return stored, stored.CheckRequested(requested)
			}}
			client := &ethClientMock{}
			client.On("ChainID", mock.Anything).Return(big.NewInt(42), nil).Once()
			info := &CreateInfo{
				Config: config.PrtConfig{
					PrtPollingInterval: time.Hour,
					BlockchainId:       42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
					FeatureClaimSubmissionEnabled: test.requestedEnabled, BlockchainHttpRequestTimeout: time.Second,
				},
				EthClient: client, Repository: repo,
				TransactOptsFactory: ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: signer}),
			}
			svc, err := Create(t.Context(), info)
			require.Nil(t, svc)
			require.ErrorContains(t, err, test.want)
			require.True(t, registered)
			repo.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}
