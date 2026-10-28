// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"log/slog"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type publicationRefusal string

func (e publicationRefusal) Error() string { return string(e) }
func (publicationRefusal) ErrorCode() int  { return -32000 }

func TestPRTFreshJoinSelectionUsesEarliestDeadline(t *testing.T) {
	f, _, _ := preparePRTAction(t, joinTournament) // deadline 100+50
	urgent := addDeliveryJoinCandidate(f, 8, 25)   // deadline 100+25
	require.NoError(t, f.d.tick(t.Context()))
	require.Len(t, f.backend.published, 1)
	require.Equal(t, common.BigToAddress(big.NewInt(urgent.ID*10+3)), *f.backend.published[0].To(),
		"the earlier-deadline join must be chosen first")
}

func TestPRTRepeatedPreSendRefusalsRecoverWithoutApplicationPause(t *testing.T) {
	f, call, _ := preparePRTAction(t, joinTournament)
	require.Positive(t, call.Value.Sign())
	f.backend.balance = big.NewInt(1)
	for range 7 {
		clear(f.d.candidateCooldown)
		require.Error(t, f.d.tick(t.Context()))
		require.Empty(t, f.backend.published)
		require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
		require.False(t, f.d.publicationStopped())
	}
	f.backend.balance = nil
	clear(f.d.candidateCooldown)
	require.NoError(t, f.d.tick(t.Context()))
	require.Len(t, f.backend.published, 1, "funding restores publication without operator action")
}

func TestPRTUsefulUntimedActionKeepsNonceAgainstUntimedCandidate(t *testing.T) {
	f, call, _ := preparePRTAction(t, acceptTournament)
	submitPRTAction(t, f, call)
	addSecondStageApplication(f)
	report, err := f.d.sender.PublishNext(t.Context(), f.d)
	require.NoError(t, err)
	require.Equal(t, call.Key, report.Current.Action, "the held acceptance keeps its nonce")
	require.Equal(t, f.backend.raw[0], f.backend.raw[len(f.backend.raw)-1])
}

func TestPRTRefusedPostEndsCandidateSelection(t *testing.T) {
	f, _, _ := preparePRTAction(t, stageTournament)
	addSecondStageApplication(f)
	f.backend.publishError = publicationRefusal("insufficient funds for gas * price + value")
	err := f.d.tick(t.Context())
	require.Error(t, err)
	require.Len(t, f.backend.published, 1, "one POST per pass")
}

func TestPRTProgressWarningsCoverStalledNonceJoinMarginAndFrozenHead(t *testing.T) {
	f := newDeliveryFixture(t)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	join := actionID{ApplicationID: f.app.ID, Application: f.app.IApplicationAddress, Kind: joinTournament,
		Epoch: 3, Deadline: 100 + joinWarningMargin}
	report := func(block, nonce uint64) txsender.Report[actionID] {
		return txsender.Report[actionID]{Block: txsender.BlockRef{Number: block, Hash: common.BigToHash(new(big.Int).SetUint64(block))},
			Nonce: nonce, Current: &txsender.Current[actionID]{Action: join}}
	}
	f.d.recordLiveProgress(report(100, 7), nil, nil)
	require.Contains(t, logs.String(), "level=WARN msg=\"PRT join remains pending near its deadline\"")
	first := logs.String()
	for range 3 {
		f.d.recordLiveProgress(report(100, 7), nil, nil)
	}
	require.Equal(t, first, logs.String(), "near-deadline warnings are rate limited")
	f.d.recordLiveProgress(report(100+stalledNonceBlocks, 7), nil, nil)
	require.Contains(t, logs.String(), "level=WARN msg=\"PRT signer nonce is stalled\"")
	f.d.diagnostics.headSince = time.Now().Add(-2 * frozenLiveHeadInterval)
	f.d.recordLiveProgress(report(100+stalledNonceBlocks, 7), nil, nil)
	require.Contains(t, logs.String(), "level=WARN msg=\"PRT latest block has not advanced; publication may miss a deadline\"")
	before := logs.String()
	for range 3 {
		f.d.recordLiveProgress(report(100+stalledNonceBlocks, 7), nil, nil)
	}
	require.Equal(t, before, logs.String(), "progress warnings are rate limited")
	require.Equal(t, 1, strings.Count(logs.String(), "PRT signer nonce is stalled"))
}

func TestPRTCreateWiresSigningTimeout(t *testing.T) {
	viper.Set(config.PRT_AUTH_KIND, "private_key")
	viper.Set(config.PRT_AUTH_PRIVATE_KEY, "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
	t.Cleanup(func() {
		viper.Set(config.PRT_AUTH_KIND, nil)
		viper.Set(config.PRT_AUTH_PRIVATE_KEY, nil)
	})
	repo := &prtBlockPolicyCreateRepository{}
	repo.On("RegisterSubmitter", mock.Anything, PrtConfigKey, mock.Anything).
		Return(PersistentConfig{ChainID: 42, DefaultBlock: model.DefaultBlock_Finalized, ClaimSubmissionEnabled: true}, nil).Once()
	client := &ethClientMock{}
	client.On("ChainID", mock.Anything).Return(big.NewInt(42), nil).Once()
	cfg := prtStartupConfig(true)
	cfg.BlockchainHttpRequestTimeout = 7 * time.Second
	svc, err := Create(t.Context(), &CreateInfo{Config: cfg, EthClient: client, Repository: repo,
		AdapterFactory: &adapterFactoryMock{}, DeliveryBackend: &deliveryBackendFake{}})
	require.NoError(t, err)
	require.Equal(t, 7*time.Second, svc.(*Service).delivery.deliveryPolicy.SigningTimeout)
}
