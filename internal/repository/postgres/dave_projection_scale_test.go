// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"encoding/binary"
	"math/big"
	"strings"
	"sync"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type queryRecorder struct {
	mu      sync.Mutex
	queries []string
}

func (q *queryRecorder) record(query string) {
	q.mu.Lock()
	q.queries = append(q.queries, query)
	q.mu.Unlock()
}

func (q *queryRecorder) TraceQueryStart(
	ctx context.Context,
	_ *pgx.Conn,
	data pgx.TraceQueryStartData,
) context.Context {
	q.record(data.SQL)
	return ctx
}

func (*queryRecorder) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (*queryRecorder) TraceBatchStart(
	ctx context.Context,
	_ *pgx.Conn,
	_ pgx.TraceBatchStartData,
) context.Context {
	return ctx
}

func (q *queryRecorder) TraceBatchQuery(
	_ context.Context,
	_ *pgx.Conn,
	data pgx.TraceBatchQueryData,
) {
	q.record(data.SQL)
}

func (*queryRecorder) TraceBatchEnd(context.Context, *pgx.Conn, pgx.TraceBatchEndData) {}

func (q *queryRecorder) snapshot() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]string(nil), q.queries...)
}

func (q *queryRecorder) countContaining(substr string) int {
	count := 0
	for _, query := range q.snapshot() {
		if strings.Contains(strings.ToLower(query), strings.ToLower(substr)) {
			count++
		}
	}
	return count
}

// TestProjectDaveOpenInputsScalesWithDelta pins the fix for the per-tick full
// epoch rewrite: re-projecting a large OPEN epoch with no new observed inputs
// must not insert or identity-check any of the already-stored rows.
func TestProjectDaveOpenInputsScalesWithDelta(t *testing.T) {
	const inputCount = 100_000

	r := newEpochPublicationRepository(t).(*PostgresRepository)
	app := repotest.NewApplicationBuilder().WithConsensus(model.Consensus_PRT).
		Create(t.Context(), t, r)
	address := app.IApplicationAddress.Hex()

	makeInput := func(index, block uint64) *model.Input {
		rawData := make([]byte, 8)
		binary.BigEndian.PutUint64(rawData, index)
		return &model.Input{
			Index:           index,
			BlockNumber:     block,
			RawData:         rawData,
			Status:          model.InputCompletionStatus_None,
			TransactionHash: common.BigToHash(new(big.Int).SetUint64(index)),
			LogIndex:        index,
		}
	}
	observed := make([]*model.Input, inputCount)
	for i := range observed {
		observed[i] = makeInput(uint64(i), 90)
	}
	require.NoError(t, r.ObserveDaveInputs(t.Context(), address, 90,
		inputCount, observed))

	open := &model.Epoch{
		Index:                0,
		FirstBlock:           10,
		LastBlock:            90,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: inputCount,
		Status:               model.EpochStatus_Open,
	}
	require.NoError(t, r.ProjectDaveOpenInputs(t.Context(), address, open,
		observed, 90))

	// Re-project the same 100k-row epoch through a repository that records SQL.
	// The tick advanced the head but observed no new input, so the delta is
	// empty and the repository must do bounded work.
	recorder := &queryRecorder{}
	cfg := r.db.Config()
	cfg.MaxConns, cfg.ConnConfig.Tracer = 1, recorder
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	traced := &PostgresRepository{db: pool}

	reprojection := *open
	reprojection.LastBlock = 100
	require.NoError(t, traced.ProjectDaveOpenInputs(t.Context(), address,
		&reprojection, nil, 100))

	require.NotEmpty(t, recorder.snapshot(), "the query recorder must be attached")
	require.Zero(t, recorder.countContaining("insert into public.input"),
		"a no-change re-projection must not insert input rows")
	require.Zero(t, recorder.countContaining("input.index in ("),
		"a no-change re-projection must not identity-check stored rows")
	require.Less(t, len(recorder.snapshot()), 30,
		"work per tick must not scale with the epoch size")

	stored, err := r.GetEpoch(t.Context(), address, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(inputCount), stored.InputIndexUpperBound)
	require.Equal(t, uint64(100), stored.LastBlock)

	// A projection that grows by ten rows writes exactly those ten rows and
	// still does not identity-check the 100k rows below the stored upper bound.
	growth := make([]*model.Input, 10)
	for i := range growth {
		growth[i] = makeInput(inputCount+uint64(i), 105)
	}
	require.NoError(t, r.ObserveDaveInputs(t.Context(), address, 110,
		inputCount+uint64(len(growth)), growth))

	grown := *open
	grown.LastBlock = 110
	grown.InputIndexUpperBound = inputCount + uint64(len(growth))
	require.NoError(t, traced.ProjectDaveOpenInputs(t.Context(), address,
		&grown, growth, 110))

	require.Equal(t, len(growth),
		recorder.countContaining("insert into public.input"))
	require.Zero(t, recorder.countContaining("input.index in ("),
		"a growing projection only writes the delta")

	stored, err = r.GetEpoch(t.Context(), address, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(inputCount+uint64(len(growth))),
		stored.InputIndexUpperBound)
	require.Equal(t, uint64(110), stored.LastBlock)
	pending, err := r.GetInput(t.Context(), address, inputCount+9)
	require.NoError(t, err)
	require.False(t, pending.Unattributed)
}
