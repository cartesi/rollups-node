// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package node

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/claimer"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/prt"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/cartesi/rollups-node/pkg/service"
)

type childServiceStub struct {
	stopped bool
}

func (s *childServiceStub) Alive() bool       { return true }
func (s *childServiceStub) Ready() bool       { return true }
func (s *childServiceStub) Reload() []error   { return nil }
func (s *childServiceStub) Tick() []error     { return nil }
func (s *childServiceStub) Serve() error      { return nil }
func (s *childServiceStub) String() string    { return "test" }
func (s *childServiceStub) Stop(bool) []error { s.stopped = true; return nil }

func TestEnforceSubmitterIsolation(t *testing.T) {
	for _, test := range []struct {
		name           string
		claimerEnabled bool
		prtEnabled     bool
		prtIndex       uint32
		missingService string
		wantCollision  bool
	}{
		{name: "equal addresses", claimerEnabled: true, prtEnabled: true, wantCollision: true},
		{name: "different addresses", claimerEnabled: true, prtEnabled: true, prtIndex: 6},
		{name: "disabled claimer", prtEnabled: true},
		{name: "disabled PRT", claimerEnabled: true},
		{name: "both disabled"},
		{name: "missing claimer", claimerEnabled: true, prtEnabled: true, missingService: claimer.ClaimerConfigKey},
		{name: "missing PRT", claimerEnabled: true, prtEnabled: true, missingService: prt.PrtConfigKey},
	} {
		t.Run(test.name, func(t *testing.T) {
			claimService, prtService := newSubmitterServices(t, test.claimerEnabled, test.prtEnabled, test.prtIndex)
			other := &childServiceStub{}
			children := []service.IService{other}
			if test.missingService != claimer.ClaimerConfigKey {
				children = append(children, claimService)
			}
			if test.missingService != prt.PrtConfigKey {
				children = append(children, prtService)
			}

			err := enforceSubmitterIsolation(children)
			if test.wantCollision {
				address, enabled := claimService.SubmitterAddress()
				require.True(t, enabled)
				require.ErrorContains(t, err, address.String())
			} else {
				require.NoError(t, err)
			}
			require.False(t, other.stopped, "the constructor owns child shutdown")
			require.False(t, claimService.IsStopping())
			require.False(t, prtService.IsStopping())
		})
	}
}

func TestCreateEnforcesSubmitterIsolation(t *testing.T) {
	for _, test := range []struct {
		name           string
		claimerEnabled bool
		prtEnabled     bool
		prtIndex       uint32
		wantCollision  bool
	}{
		{name: "equal addresses", claimerEnabled: true, prtEnabled: true, wantCollision: true},
		{name: "different addresses", claimerEnabled: true, prtEnabled: true, prtIndex: 6},
		{name: "both disabled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, repo := newSubmitterEnvironment(t, test.claimerEnabled, test.prtEnabled, test.prtIndex)
			args := &CreateInfo{
				CreateInfo: service.CreateInfo{Name: "node", Context: t.Context()},
				Config: config.NodeConfig{
					BlockchainId: 31337, BlockchainHttpRequestTimeout: time.Second, AdvancerInputBatchSize: 1,
					BlockchainDefaultBlock:        model.DefaultBlock_Finalized,
					FeatureClaimSubmissionEnabled: test.claimerEnabled || test.prtEnabled,
				},
				PrtClient: client, ClaimerClient: client, ReaderClient: client, Repository: repo,
			}
			node, err := Create(t.Context(), args)
			created := args.Impl.(*Service)
			t.Cleanup(func() {
				created.Stop(true)
				created.Ticker.Stop()
				created.Cancel()
			})
			require.Len(t, created.Children, 5, "all normal child constructors must complete before the guard")
			if test.wantCollision {
				require.Nil(t, node)
				require.ErrorContains(t, err, "claimer and PRT submitter addresses must be different")
			} else {
				require.NoError(t, err)
				require.Same(t, created, node)
			}
			for _, child := range created.Children {
				state := child.(interface{ IsStopping() bool })
				require.Equal(t, test.wantCollision, state.IsStopping(), child.String())
			}
		})
	}
}

type submitterConfigRepository struct {
	repository.Repository
	values map[string][]byte
}

func (r *submitterConfigRepository) LoadNodeConfigRaw(
	_ context.Context, key string,
) ([]byte, time.Time, time.Time, error) {
	value, ok := r.values[key]
	if !ok {
		return nil, time.Time{}, time.Time{}, fmt.Errorf("unexpected node config key %q", key)
	}
	return value, time.Time{}, time.Time{}, nil
}

type submitterChainRPC struct{}

//nolint:revive // ChainId preserves the eth_chainId JSON-RPC method name.
func (*submitterChainRPC) ChainId(context.Context) *hexutil.Big {
	return (*hexutil.Big)(big.NewInt(31337))
}

func newSubmitterEnvironment(
	t *testing.T, claimerEnabled, prtEnabled bool, prtIndex uint32,
) (*ethclient.Client, *submitterConfigRepository) {
	t.Helper()
	viper.Reset()
	viper.AutomaticEnv()
	config.SetDefaults()
	t.Cleanup(func() {
		viper.Reset()
		viper.AutomaticEnv()
		config.SetDefaults()
	})
	viper.Set(config.AUTH_KIND, "mnemonic")
	viper.Set(config.AUTH_MNEMONIC, ethutil.FoundryMnemonic)
	viper.Set(config.AUTH_MNEMONIC_ACCOUNT_INDEX, 0)
	viper.Set(config.PRT_AUTH_KIND, "mnemonic")
	viper.Set(config.PRT_AUTH_MNEMONIC, ethutil.FoundryMnemonic)
	viper.Set(config.PRT_AUTH_MNEMONIC_ACCOUNT_INDEX, prtIndex)

	server := rpc.NewServer()
	require.NoError(t, server.RegisterName("eth", &submitterChainRPC{}))
	t.Cleanup(server.Stop)
	client := ethclient.NewClient(rpc.DialInProc(server))
	t.Cleanup(client.Close)
	storedConfig := func(enabled bool) []byte {
		value, err := json.Marshal(claimer.PersistentConfig{
			DefaultBlock: model.DefaultBlock_Finalized, ClaimSubmissionEnabled: enabled, ChainID: 31337,
		})
		require.NoError(t, err)
		return value
	}
	repo := &submitterConfigRepository{values: map[string][]byte{
		claimer.ClaimerConfigKey: storedConfig(claimerEnabled),
		prt.PrtConfigKey:         storedConfig(prtEnabled),
	}}
	readerConfig, err := json.Marshal(evmreader.PersistentConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: 31337,
	})
	require.NoError(t, err)
	repo.values[evmreader.EvmReaderConfigKey] = readerConfig
	return client, repo
}

// Use the normal constructors so the test covers concrete service discovery,
// persisted enablement, and signer construction without access to private fields.
func newSubmitterServices(t *testing.T, claimerEnabled, prtEnabled bool, prtIndex uint32) (*claimer.Service, *prt.Service) {
	t.Helper()
	client, repo := newSubmitterEnvironment(t, claimerEnabled, prtEnabled, prtIndex)
	claimService, err := claimer.Create(t.Context(), &claimer.CreateInfo{
		CreateInfo: service.CreateInfo{Name: "claimer", Context: t.Context()},
		Config: config.ClaimerConfig{
			BlockchainId: 31337, BlockchainHttpRequestTimeout: time.Second,
			BlockchainDefaultBlock: model.DefaultBlock_Finalized, FeatureClaimSubmissionEnabled: claimerEnabled,
		},
		EthConn: client, Repository: repo,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		claimService.Ticker.Stop()
		claimService.Cancel()
	})
	prtService, err := prt.Create(t.Context(), &prt.CreateInfo{
		CreateInfo: service.CreateInfo{Name: "prt", Context: t.Context()},
		Config: config.PrtConfig{
			BlockchainId: 31337, BlockchainHttpRequestTimeout: time.Second,
			BlockchainDefaultBlock: model.DefaultBlock_Finalized, FeatureClaimSubmissionEnabled: prtEnabled,
		},
		EthClient: client, Repository: repo,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		prtService.Ticker.Stop()
		prtService.Cancel()
	})
	return claimService, prtService
}
