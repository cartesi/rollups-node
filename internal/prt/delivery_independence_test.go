// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"errors"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"log/slog"
	"strings"
	"testing"
)

// A successful sibling cannot clear an incomplete application observation pass.
func deliveryRegressionObserveHeads(t *testing.T, withHealthySibling bool, heads int) string {
	f := newObserverCheckpointFixture(t)
	var output bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&output, nil))
	stuck, healthy := checkpointEpoch(0, "0x100"), checkpointEpoch(1, "0x200")
	failure := errors.New("provider cannot serve this tournament's logs")
	epochs := []*model.Epoch{stuck}
	if withHealthySibling {
		epochs = append(epochs, healthy)
	}
	for i := range heads {
		head := uint64(100 + i)
		f.consensus.On("TournamentLevelCount", mock.MatchedBy(resultCallOptsAtBlock(head))).Return(uint64(2), nil).Once()
		f.tournament(stuck, *stuck.TournamentAddress, 0, 2, 0, head, &TournamentEvents{}, failure)
		if withHealthySibling {
			f.tournament(healthy, *healthy.TournamentAddress, 0, 2, 0, head, &TournamentEvents{}, nil)
			f.repo.On("StoreTournamentInterval", mock.Anything, f.app.ID,
				mock.MatchedBy(func(b []*repository.TournamentEventBatch) bool {
					return len(b) == 1 && b[0].Tournament.Address == *healthy.TournamentAddress
				}), head).Return(nil).Once()
		}
		err := f.s.observeTournamentState(t.Context(), f.app, epochs, f.consensus, observerFacts(head))
		require.ErrorIs(t, err, failure)
	}
	return output.String()
}

const deliveryRegressionStallMessage = "PRT tournament observation is stalled"

func TestDeliveryStallAlarmControlSingleFailingTournament(t *testing.T) {
	logs := deliveryRegressionObserveHeads(t, false, 6)
	require.Contains(t, logs, deliveryRegressionStallMessage, "control: the alarm fires for a lone stuck tournament")
}

func TestDeliveryStallAlarmFiresDespiteHealthySibling(t *testing.T) {
	logs := deliveryRegressionObserveHeads(t, true, 6)
	require.Contains(t, logs, deliveryRegressionStallMessage,
		"a tournament that failed at six increasing heads must raise the stall alarm; got %d failure records cleared",
		strings.Count(logs, "level=ERROR"))
}

// deliveryRegressionHeldStage publishes a held stage through the real sender, then moves the
// protected live head to 111 (same protocol view), as in
// TestHeldConsensusSurvivesIncompleteObservationAndSpendingPause.

// Held maintenance is independent of the observer's consecutive failures.

// Replica skew is uncertainty, not proof that the held action must be cancelled.
