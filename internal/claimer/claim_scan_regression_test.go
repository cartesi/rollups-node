// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestIndependentClaimScanOmittedAcceptanceAfterPriorStageIsInconclusive(t *testing.T) {
	chain, api, app := newClaimScanHistory(t)
	before := claimRead{At: txsender.BlockRef{Number: 40, Hash: api.header(40).Hash()},
		Staged: big.NewInt(1), Accepted: new(big.Int)}
	api.logs = nil // the stage is before this range; only the later acceptance is omitted
	api.counterAt = func(block uint64) (*big.Int, *big.Int, error) {
		accepted := new(big.Int)
		if block >= 41 {
			accepted.SetUint64(1)
		}
		return big.NewInt(1), accepted, nil
	}
	batch, err := chain.scanClaims(t.Context(), app, 40, &before, 0)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.ErrorContains(t, err, "staged/accepted logs do not match pinned counters")
	require.Nil(t, batch)
	require.Equal(t, 0, api.claimReads, "the accepted-counter certificate is the only claim-level check in this range")
	require.Len(t, api.filters, 1)
	require.Zero(t, before.Accepted.Sign())
}

func TestIndependentClaimScanFixedHeadStillChecksLocalReadiness(t *testing.T) {
	chain, api, app := newClaimScanHistory(t)
	r := &observationTestRepo{apps: []*model.Application{app}, safe: map[int64]uint64{},
		roots: map[uint64]repository.ClaimRoots{}}
	o := &independentClaimObserver{repository: r, blockchain: chain, logger: chain.logger,
		policy: model.DefaultBlock_Finalized, budget: time.Second}
	_, err := o.observe(t.Context(), 100)
	require.NoError(t, err)
	require.Len(t, o.apps[app.ID].pending, 1)
	require.EqualValues(t, 39, r.safe[app.ID])
	headers, selectors, filters := api.headerReads, len(api.selectors), len(api.filters)
	r.roots[29] = readyRoots(api.machine, common.HexToHash("0xbad"))
	_, err = o.observe(t.Context(), 100)
	require.NoError(t, err)
	require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
	require.EqualValues(t, 100, r.safe[app.ID])
	require.Equal(t, []uint64{29, 29}, r.rootCalls)
	require.Equal(t, headers, api.headerReads, "fixed-head discovery performs no header RPC")
	require.Len(t, api.selectors, selectors, "fixed-head discovery performs no state RPC")
	require.Len(t, api.filters, filters)
}

func TestIndependentClaimScanSameHeightDifferentHashRejectsOldCertificate(t *testing.T) {
	chain, api, app := newClaimScanHistory(t)
	previous := claimRead{At: chain.observed, Staged: big.NewInt(1), Accepted: new(big.Int)}
	replacement := *api.header(100)
	replacement.Extra = []byte("new branch at the same height")
	api.headers[100] = &replacement
	api.byHash[replacement.Hash()] = 100
	chain.observed.Hash = replacement.Hash()
	batch, err := chain.scanClaims(t.Context(), app, 100, &previous, 0)
	require.ErrorIs(t, err, txsender.ErrNonCanonical)
	require.Nil(t, batch)
	require.Empty(t, api.selectors, "reject the cached branch before using its counters")
	require.Empty(t, api.filters)
	require.Positive(t, api.headerReads)
}

func TestIndependentClaimScanDensityUsesBothCounterDeltas(t *testing.T) {
	chain, api, app := newClaimScanHistory(t)
	app.EpochLength = 1
	api.head, api.clock = 1000, big.NewInt(600)
	chain.observed = txsender.BlockRef{Number: 1000, Hash: api.header(1000).Hash()}
	before := claimRead{At: txsender.BlockRef{Number: 400, Hash: api.header(400).Hash()},
		Staged: big.NewInt(150), Accepted: new(big.Int)}
	// The block has 150 new stages and 150 acceptances of older stages. Each
	// delta alone is below the admission target; their total is above it.
	api.counterAt = func(block uint64) (*big.Int, *big.Int, error) {
		if block < 600 {
			return big.NewInt(150), new(big.Int), nil
		}
		return big.NewInt(300), big.NewInt(150), nil
	}
	api.logs = nil
	for index := range 150 {
		api.logs = append(api.logs, watchLog(api, watchStageTopic, big.NewInt(int64(150+index)), 600, uint(index)))
	}
	for index := range 150 {
		api.logs = append(api.logs, watchLog(api, watchAcceptTopic, big.NewInt(int64(index)), 600, uint(150+index)))
	}
	batch, err := chain.scanClaims(t.Context(), app, 400, &before, 0)
	require.NoError(t, err)
	require.NotNil(t, batch)
	require.GreaterOrEqual(t, batch.Read.At.Number, uint64(401))
	require.Less(t, batch.Read.At.Number, uint64(600), "split before the dense block instead of admitting all 300 events")
	require.Empty(t, batch.Claims)
	require.Equal(t, big.NewInt(150), batch.Read.Staged)
	require.Zero(t, batch.Read.Accepted.Sign())
	require.Len(t, api.filters, 1, "count-based admission happens before fetching logs")
	require.Equal(t, batch.Read.At.Number, api.filters[0].ToBlock.ToInt().Uint64())
}

func TestIndependentClaimScanDenseSingleBlockIsReadWhole(t *testing.T) {
	chain, api, app := newClaimScanHistory(t)
	app.EpochLength = 1
	api.head, api.clock = 600, big.NewInt(600)
	chain.observed = txsender.BlockRef{Number: 600, Hash: api.header(600).Hash()}
	before := claimRead{At: txsender.BlockRef{Number: 599, Hash: api.header(599).Hash()},
		Staged: new(big.Int), Accepted: new(big.Int)}
	api.counterAt = func(block uint64) (*big.Int, *big.Int, error) {
		if block < 600 {
			return new(big.Int), new(big.Int), nil
		}
		return big.NewInt(300), new(big.Int), nil
	}
	api.logs = make([]types.Log, 0, 300)
	for index := range 300 {
		api.logs = append(api.logs, watchLog(api, watchStageTopic, big.NewInt(int64(index)), 600, uint(index)))
	}
	batch, err := chain.scanClaims(t.Context(), app, 599, &before, 1)
	require.NoError(t, err)
	require.EqualValues(t, 600, batch.Read.At.Number)
	require.Len(t, batch.Claims, 300, "a block is indivisible; the event target must never truncate it")
	require.Equal(t, big.NewInt(300), batch.Read.Staged)
	require.Equal(t, 300, api.claimReads)
	require.Len(t, api.filters, 1)
	require.EqualValues(t, 600, api.filters[0].FromBlock.ToInt().Uint64())
	require.EqualValues(t, 600, api.filters[0].ToBlock.ToInt().Uint64())
}
