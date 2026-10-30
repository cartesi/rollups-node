// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"errors"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSetupPersistentConfigUsesAtomicRegistration(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		signer := common.Address{}
		if enabled {
			signer = common.HexToAddress("0x1")
		}
		requested := PersistentConfig{
			DefaultBlock: model.DefaultBlock_Finalized, ChainID: 42, ClaimSubmissionEnabled: enabled, ResolvedSignerAddress: signer,
		}
		for _, registrationErr := range []error{nil, errors.New("concurrent config differs")} {
			s, _, _ := newServiceMock(t)
			repo := &claimerCreateRepositoryMock{}
			repo.On("RegisterSubmitter", mock.Anything, ClaimerConfigKey, requested).Return(requested, registrationErr).Once()
			got, err := setupPersistentConfig(t.Context(), s.Logger, repo, &config.ClaimerConfig{
				BlockchainId: 42, BlockchainDefaultBlock: model.DefaultBlock_Finalized, FeatureClaimSubmissionEnabled: enabled,
			}, signer)
			if registrationErr == nil {
				require.NoError(t, err)
				require.Equal(t, requested, *got)
			} else {
				require.ErrorIs(t, err, registrationErr)
				require.Nil(t, got)
			}
			repo.AssertExpectations(t)
		}
	}
}

func TestCreateRegistersInjectedPublicSignerBeforePublication(t *testing.T) {
	viper.Set(config.AUTH_KIND, "invalid")
	t.Cleanup(func() { viper.Set(config.AUTH_KIND, nil) })
	signer := common.HexToAddress("0xa1")
	factory := ethutil.NewStaticTransactOptsFactory(&bind.TransactOpts{From: signer})
	requested := PersistentConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: 42, ClaimSubmissionEnabled: true, ResolvedSignerAddress: signer,
	}
	repo := &claimerCreateRepositoryMock{}
	registrationErr := errors.New("saved observer policy differs")
	repo.On("RegisterSubmitter", mock.Anything, ClaimerConfigKey, requested).
		Return(PersistentConfig{}, registrationErr).Once()
	svc, err := Create(t.Context(), &CreateInfo{
		Config: config.ClaimerConfig{
			ClaimerPollingInterval: time.Hour, BlockchainId: 42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
			FeatureClaimSubmissionEnabled: true, BlockchainHttpRequestTimeout: time.Second,
		},
		EthConn: newTestEthClient(t, 42), Repository: repo, TransactOptsFactory: factory,
	})
	require.Nil(t, svc)
	require.ErrorIs(t, err, registrationErr, "the injected identity is used without resolving auth again")
	repo.AssertExpectations(t)
}

func TestCreateRejectsUnresolvedSignerBeforeRegistration(t *testing.T) {
	viper.Set(config.AUTH_KIND, "invalid")
	t.Cleanup(func() { viper.Set(config.AUTH_KIND, nil) })
	repo := &claimerCreateRepositoryMock{}
	svc, err := Create(t.Context(), &CreateInfo{Config: config.ClaimerConfig{
		ClaimerPollingInterval: time.Hour, BlockchainId: 42, BlockchainDefaultBlock: model.DefaultBlock_Finalized,
		FeatureClaimSubmissionEnabled: true, BlockchainHttpRequestTimeout: time.Second,
	}, EthConn: newTestEthClient(t, 42), Repository: repo})
	require.Nil(t, svc)
	require.ErrorContains(t, err, "getting transaction options")
	repo.AssertNotCalled(t, "RegisterSubmitter", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestCreateReaderUsesRegisteredPolicy(t *testing.T) {
	persisted := PersistentConfig{DefaultBlock: model.DefaultBlock_Latest, ChainID: 42, ClaimSubmissionEnabled: false}
	repo := &claimerCreateRepositoryMock{}
	repo.On("RegisterSubmitter", mock.Anything, ClaimerConfigKey, persisted).Return(persisted, nil).Once()
	svc, err := Create(t.Context(), &CreateInfo{Config: config.ClaimerConfig{
		ClaimerPollingInterval: time.Hour, BlockchainId: 42, BlockchainDefaultBlock: model.DefaultBlock_Latest,
	}, EthConn: newTestEthClient(t, 42), Repository: repo})
	require.NoError(t, err)
	impl := svc.(*Service)
	require.False(t, impl.submissionEnabled)
	require.Equal(t, model.DefaultBlock_Latest, impl.blockchain.(*claimerBlockchain).defaultBlock)
	require.True(t, impl.Ready())
	require.Nil(t, impl.publisher)
	require.NotNil(t, impl.claimObserver, "reader mode must observe claims independently of local publication")
	require.Equal(t, model.DefaultBlock_Latest, impl.claimObserver.policy)
	repo.AssertExpectations(t)
}
