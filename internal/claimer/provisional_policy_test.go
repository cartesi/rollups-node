// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"bytes"
	"log/slog"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCreateWarnsOnceForProvisionalClaimObservation(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Latest, model.DefaultBlock_Safe, model.DefaultBlock_Finalized} {
		t.Run(policy.String(), func(t *testing.T) {
			key, err := crypto.GenerateKey()
			require.NoError(t, err)
			opts, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(42))
			require.NoError(t, err)
			requested := PersistentConfig{
				ChainID: 42, DefaultBlock: policy, ClaimSubmissionEnabled: true, ResolvedSignerAddress: opts.From,
			}
			repo := &claimerCreateRepositoryMock{}
			repo.On("RegisterSubmitter", mock.Anything, ClaimerConfigKey, requested).Return(requested, nil).Once()
			var output bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&output, nil))
			_, err = Create(t.Context(), &CreateInfo{
				Config: config.ClaimerConfig{
					BlockchainId: 42, BlockchainDefaultBlock: policy, ClaimerPollingInterval: time.Second,
					FeatureClaimSubmissionEnabled: true, BlockchainHttpRequestTimeout: time.Second,
				},
				Logger: logger, EthConn: newTestEthClient(t, 42), Repository: repo,
				TransactOptsFactory: ethutil.NewStaticTransactOptsFactory(opts),
			})
			require.NoError(t, err)
			wanted := 1
			if policy == model.DefaultBlock_Finalized {
				wanted = 0
			}
			require.Equal(t, wanted, strings.Count(output.String(), "reorg rollback is unsupported"))
			if wanted != 0 {
				require.Contains(t, output.String(), "default_block="+policy.String())
			}
			repo.AssertExpectations(t)
		})
	}
}
