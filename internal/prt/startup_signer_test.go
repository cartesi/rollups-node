// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCreateExplainsRequiredPRTSigner(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "submission disabled"
		if enabled {
			name = "submission enabled"
		}
		t.Run(name, func(t *testing.T) {
			viper.Set(config.PRT_AUTH_KIND, "mnemonic_file")
			viper.Set(config.PRT_AUTH_MNEMONIC, "")
			viper.Set(config.PRT_AUTH_MNEMONIC_FILE, filepath.Join(t.TempDir(), "missing-mnemonic"))
			t.Cleanup(func() {
				viper.Set(config.PRT_AUTH_KIND, nil)
				viper.Set(config.PRT_AUTH_MNEMONIC, nil)
				viper.Set(config.PRT_AUTH_MNEMONIC_FILE, nil)
			})
			repo := &prtBlockPolicyCreateRepository{}
			if !enabled {
				repo.On("RegisterSubmitter", mock.Anything, PrtConfigKey, mock.Anything).
					Return(PersistentConfig{
						ChainID: 42, DefaultBlock: model.DefaultBlock_Finalized,
					}, nil).Once()
			}
			client := &ethClientMock{}
			client.On("ChainID", mock.Anything).Return(big.NewInt(42), nil).Once()
			svc, err := Create(t.Context(), &CreateInfo{
				Config: config.PrtConfig{
					PrtPollingInterval: time.Hour, BlockchainId: 42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
					FeatureClaimSubmissionEnabled: enabled, BlockchainHttpRequestTimeout: time.Second,
				},
				Repository: repo, EthClient: client, AdapterFactory: &adapterFactoryMock{},
			})
			if enabled {
				require.Nil(t, svc)
				require.ErrorIs(t, err, os.ErrNotExist, "preserve the underlying credential error")
				require.ErrorContains(t, err, "PRT submission is enabled and requires signer configuration (CARTESI_PRT_AUTH_*)")
				require.ErrorContains(t, err, "the standalone node always starts PRT")
				repo.AssertNotCalled(t, "RegisterSubmitter", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			} else {
				require.NoError(t, err, "reader mode must not require signer credentials")
				require.Nil(t, svc.(*Service).txOptsFactory)
			}
			repo.AssertExpectations(t)
			client.AssertExpectations(t)
		})
	}
}

func TestCreateRegistersResolvedPRTIdentityBeforeDelivery(t *testing.T) {
	signer := common.HexToAddress("0x600")
	repo := &prtBlockPolicyCreateRepository{register: func(
		_ context.Context, key string, requested PersistentConfig,
	) (PersistentConfig, error) {
		require.Equal(t, PrtConfigKey, key)
		require.Equal(t, signer, requested.ResolvedSignerAddress)
		require.True(t, requested.ClaimSubmissionEnabled)
		return requested, nil
	}}
	client := &ethClientMock{}
	client.On("ChainID", mock.Anything).Return(big.NewInt(42), nil).Once()
	svc, err := Create(t.Context(), &CreateInfo{
		Config: prtStartupConfig(true), Repository: repo, EthClient: client, AdapterFactory: &adapterFactoryMock{},
		TransactOptsFactory: ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: signer}),
		DeliveryBackend:     &deliveryBackendFake{},
	})
	require.NoError(t, err)
	s := svc.(*Service)
	require.Equal(t, signer, s.txOptsFactory.From())
	require.NotNil(t, s.delivery.sender)
	require.NoError(t, s.delivery.guardPublication(t.Context()))
	require.False(t, s.Ready(), "Serve verifies the contributing observer before publication")
	client.AssertExpectations(t)
}

func TestCreateAcceptsOptionalFeeCeilingAndRejectsInvalidValue(t *testing.T) {
	for _, value := range []string{"", "100", "invalid"} {
		t.Run(value, func(t *testing.T) {
			cfg := prtStartupConfig(true)
			cfg.BlockchainTxMaxFeePerGas = value
			repo := &prtBlockPolicyCreateRepository{register: func(_ context.Context, _ string,
				requested PersistentConfig) (PersistentConfig, error) {
				return requested, nil
			}}
			client := &ethClientMock{}
			client.On("ChainID", mock.Anything).Return(big.NewInt(42), nil).Once()
			svc, err := Create(t.Context(), &CreateInfo{Config: cfg, Repository: repo, EthClient: client,
				AdapterFactory:      &adapterFactoryMock{},
				TransactOptsFactory: ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: common.HexToAddress("0x600")}),
				DeliveryBackend:     &deliveryBackendFake{}})
			if value == "invalid" {
				require.Nil(t, svc)
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, svc)
			}
		})
	}
}

func TestPRTReadinessRequiresStartupAndRejectsOnlyHardFailures(t *testing.T) {
	f := newDeliveryFixture(t)
	f.d.delivery, f.d.submissionEnabled = f.d, true
	require.False(t, f.d.Ready(), "publication has not started")
	f.d.publicationReady.Store(true)
	f.repo.onProvenance = func(context.Context) error {
		t.Fatal("publication guard must not reread startup registration")
		return nil
	}
	require.NoError(t, f.d.guardPublication(t.Context()))
	require.True(t, f.d.Ready())
	f.d.observationFailures[f.app.ID] = tournamentObservationFailure{failedHeads: tournamentObservationFailureThreshold}
	require.True(t, f.d.Ready(), "one application's failed observation is not a service prerequisite")
	f.d.stopPublication(txsender.ErrFinality)
	require.False(t, f.d.Ready())
	require.ErrorIs(t, f.d.guardPublication(t.Context()), txsender.ErrFinality)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, newDeliveryFixture(t).d.guardPublication(ctx), context.Canceled)
}
