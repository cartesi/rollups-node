// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package remove

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

type ackRepositoryStub struct {
	pending [][]string
	err     error
	calls   int
}

func (s *ackRepositoryStub) GetPendingAcks(
	_ context.Context,
	_ int64,
	_ []string,
) ([]string, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if len(s.pending) == 0 {
		return nil, nil
	}
	current := s.pending[0]
	if len(s.pending) > 1 {
		s.pending = s.pending[1:]
	}
	return current, nil
}

func TestValidateRemoveOptions(t *testing.T) {
	origForce, origWait := forceFlag, waitFlag
	origTimeout, origPoll := waitTimeout, waitPollInterval
	t.Cleanup(func() {
		forceFlag = origForce
		waitFlag = origWait
		waitTimeout = origTimeout
		waitPollInterval = origPoll
	})

	forceFlag = true
	waitFlag = true
	waitTimeout = time.Second
	waitPollInterval = time.Second
	assert.EqualError(t, validateRemoveOptions(), "--wait cannot be used with --force")

	forceFlag = false
	waitFlag = true
	waitTimeout = 0
	assert.EqualError(t, validateRemoveOptions(), "--timeout must be greater than 0")

	waitTimeout = time.Second
	waitPollInterval = 0
	assert.EqualError(t, validateRemoveOptions(), "--poll-interval must be greater than 0")

	waitPollInterval = time.Second
	require.NoError(t, validateRemoveOptions())
}

func TestRequiresBondLossAcknowledgement(t *testing.T) {
	t.Parallel()

	prtApp := &model.Application{
		Name:          "prt-app",
		ConsensusType: model.Consensus_PRT,
	}
	deletedAt := time.Now()
	softDeletedPRTApp := &model.Application{
		Name:          "soft-deleted-prt-app",
		ConsensusType: model.Consensus_PRT,
		DeletedAt:     &deletedAt,
	}
	authorityApp := &model.Application{
		Name:          "authority-app",
		ConsensusType: model.Consensus_Authority,
	}

	assert.False(t, requiresBondLossAcknowledgement(nil, true, true))
	assert.False(t, requiresBondLossAcknowledgement(authorityApp, true, true))
	assert.False(t, requiresBondLossAcknowledgement(prtApp, false, false))
	assert.True(t, requiresBondLossAcknowledgement(prtApp, false, true))
	assert.True(t, requiresBondLossAcknowledgement(prtApp, true, true))
	assert.False(t, requiresBondLossAcknowledgement(softDeletedPRTApp, false, true))
	assert.True(t, requiresBondLossAcknowledgement(softDeletedPRTApp, true, true))
}

func TestWaitForDrainAcksImmediateSuccess(t *testing.T) {
	t.Parallel()

	repo := &ackRepositoryStub{}
	app := &model.Application{
		ID:            42,
		Name:          "echo-dapp",
		ConsensusType: model.Consensus_Authority,
	}
	var out bytes.Buffer

	err := waitForDrainAcks(
		context.Background(),
		repo,
		app,
		100*time.Millisecond,
		10*time.Millisecond,
		&out,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, repo.calls)
	assert.Empty(t, out.String())
}

func TestWaitForDrainAcksPollsUntilComplete(t *testing.T) {
	t.Parallel()

	repo := &ackRepositoryStub{
		pending: [][]string{
			{repository.ServiceAdvancer, repository.ServiceClaimer},
			{repository.ServiceClaimer},
			{},
		},
	}
	app := &model.Application{
		ID:            42,
		Name:          "echo-dapp",
		ConsensusType: model.Consensus_Authority,
	}
	var out bytes.Buffer

	err := waitForDrainAcks(
		context.Background(),
		repo,
		app,
		time.Second,
		10*time.Millisecond,
		&out,
	)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, repo.calls, 3)
	assert.Contains(t, out.String(), "Waiting for drain acknowledgments")
	assert.Contains(t, out.String(), repository.ServiceAdvancer)
}

func TestWaitForDrainAcksTimeout(t *testing.T) {
	t.Parallel()

	repo := &ackRepositoryStub{
		pending: [][]string{
			{repository.ServiceAdvancer},
		},
	}
	app := &model.Application{
		ID:            42,
		Name:          "echo-dapp",
		ConsensusType: model.Consensus_Authority,
	}
	var out bytes.Buffer

	err := waitForDrainAcks(
		context.Background(),
		repo,
		app,
		30*time.Millisecond,
		10*time.Millisecond,
		&out,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out waiting for drain acknowledgments")
	assert.Contains(t, err.Error(), repository.ServiceAdvancer)
}

func TestWaitForDrainAcksPropagatesRepositoryErrors(t *testing.T) {
	t.Parallel()

	repo := &ackRepositoryStub{err: errors.New("db unavailable")}
	app := &model.Application{
		ID:            42,
		Name:          "echo-dapp",
		ConsensusType: model.Consensus_Authority,
	}
	var out bytes.Buffer

	err := waitForDrainAcks(
		context.Background(),
		repo,
		app,
		time.Second,
		10*time.Millisecond,
		&out,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check pending acks")
	assert.Contains(t, err.Error(), "db unavailable")
}
