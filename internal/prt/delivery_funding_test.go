// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	joinFundingMessage = "PRT signer cannot fund tournament join; fund the signer now; join must be included before the closing block"
	// The fixture charges a 50 wei bond and quotes 3 wei per gas. Its estimate
	// is 21,000 gas; the prepared join adds 150,000 gas.
	joinMinimumFundsWei  = 63_050
	joinPreparedFundsWei = 513_050
)

func fundingErrors(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["level"] == "ERROR" {
			records = append(records, record)
		}
	}
	return records
}

func TestPRTJoinFundingRefusalsReportActualSenderAmounts(t *testing.T) {
	for _, test := range []struct {
		name      string
		balance   int64
		required  string
		available string
	}{
		{"minimum before estimation", 0, "63050", "0"},
		{"minimum fits but prepared join does not", joinMinimumFundsWei, "513050", "63050"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, _, _ := preparePRTAction(t, joinTournament)
			var logs bytes.Buffer
			f.d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			f.backend.balance = big.NewInt(test.balance)
			err := f.d.tick(t.Context())
			require.ErrorIs(t, err, txsender.ErrLimit)
			var limit *txsender.LimitError
			require.ErrorAs(t, err, &limit)
			records := fundingErrors(t, &logs)
			require.Len(t, records, 1)
			record := records[0]
			require.Equal(t, joinFundingMessage, record["msg"])
			require.Equal(t, strings.ToLower(f.app.IApplicationAddress.Hex()), record["address"])
			require.Equal(t, float64(f.epoch.Index), record["epoch"])
			require.Equal(t, strings.ToLower(f.epoch.TournamentAddress.Hex()), record["tournament"])
			require.Equal(t, float64(150), record["join_closes_at_block"])
			require.Equal(t, strings.ToLower(f.d.txOptsFactory.From().Hex()), record["signer"])
			require.Equal(t, "join tournament", record["action"])
			require.Equal(t, test.required, record["required_wei"])
			require.Equal(t, test.available, record["available_wei"])
			require.Equal(t, limit.Required.String(), record["required_wei"])
			require.Equal(t, limit.Available.String(), record["available_wei"])
			errorText, ok := record["error"].(string)
			require.True(t, ok, "the funding error must contain a string description")
			require.Equal(t, test.balance == 0, strings.Contains(errorText, "minimum funding before gas estimation"))
			require.NotContains(t, logs.String(), "PRT transaction could not publish", "the funding ERROR replaces the generic WARN")
			require.Empty(t, f.backend.published)
			require.Empty(t, f.repo.windows)
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			require.False(t, f.d.publicationStopped())
			f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestPRTJoinFundingErrorRepeatsUntilSignerIsFunded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, _, _ := preparePRTAction(t, joinTournament)
		var logs bytes.Buffer
		f.d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
		f.backend.balance = new(big.Int)
		for tick := range 2 {
			require.ErrorIs(t, f.d.tick(t.Context()), txsender.ErrLimit)
			require.Len(t, fundingErrors(t, &logs), tick+1, "each real refusal needs an ERROR before the warning interval expires")
			time.Sleep(f.d.pollInterval)
		}
		f.backend.balance.SetInt64(joinPreparedFundsWei)
		require.NoError(t, f.d.tick(t.Context()))
		require.Len(t, fundingErrors(t, &logs), 2, "funding must stop the ERROR")
		require.Len(t, f.backend.published, 1, "the exact prepared budget must permit the join")
		require.Equal(t, big.NewInt(50), f.backend.published[0].Value())
		require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	})
}

func TestPRTFundedJoinDoesNotReportFundingError(t *testing.T) {
	for _, balance := range []int64{joinPreparedFundsWei, joinPreparedFundsWei + 1} {
		f, _, _ := preparePRTAction(t, joinTournament)
		var logs bytes.Buffer
		f.d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
		f.backend.balance = big.NewInt(balance)
		require.NoError(t, f.d.tick(t.Context()))
		require.Empty(t, fundingErrors(t, &logs))
		require.Len(t, f.backend.published, 1)
	}
}

func TestPRTFundingErrorDoesNotMislabelOtherRefusals(t *testing.T) {
	for _, test := range []struct {
		name      string
		kind      actionKind
		resource  string
		canceling bool
	}{
		{"base fee", joinTournament, "base fee", false},
		{"gas limit", joinTournament, "transaction gas", false},
		{"stage funds", stageTournament, "signer balance", false},
		{"cancellation funds", joinTournament, "signer balance", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, call, _ := preparePRTAction(t, test.kind)
			var logs bytes.Buffer
			f.d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			err := errors.Join(txsender.ErrLimit, &txsender.LimitError{
				Limit: test.resource, Required: big.NewInt(2), Available: big.NewInt(1),
			})
			f.d.warnPublicationRefusal(call.Key, txsender.Report[actionID]{Canceling: test.canceling}, err)
			require.Empty(t, fundingErrors(t, &logs))
			require.Contains(t, logs.String(), "PRT transaction could not publish")
		})
	}
}

func TestPRTFirstPublisherTickChecksFundsWhenAdmissionIsReady(t *testing.T) {
	for _, admitted := range []bool{false, true} {
		name := "awaits deployment admission"
		if admitted {
			name = "eligible join checks funds immediately"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f, _, _ := preparePRTAction(t, joinTournament)
				var logs bytes.Buffer
				f.d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
				f.d.delivery, f.d.submissionEnabled, f.d.pollInterval = f.d, true, time.Hour
				f.backend.balance = new(big.Int)
				if !admitted {
					f.d.deployments = deploymentCache{}
				}
				observer := &startupTickFixture{fn: func(ctx context.Context) (bool, error) {
					<-ctx.Done()
					return false, nil
				}}
				require.NoError(t, service.InitTickServiceTemplate(&f.d.TickServiceTemplate, &service.TickServiceConfigs{
					BaseConfigs: service.BaseConfigs{Logger: f.d.Logger}, PollInterval: time.Hour,
				}, observer))
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				done := make(chan error, 1)
				go func() { done <- f.d.Serve(ctx) }()
				synctest.Wait()
				require.NotEmpty(t, f.backend.headerReads, "the first publisher tick runs without waiting an hour")
				if admitted {
					require.Len(t, fundingErrors(t, &logs), 1)
				} else {
					require.Empty(t, fundingErrors(t, &logs), "no funding verdict before a join can be planned")
				}
				require.Empty(t, f.backend.published)
				cancel()
				require.ErrorIs(t, <-done, context.Canceled)
			})
		})
	}
}

func TestPRTJoinFundingErrorCoversBalanceCappedEstimation(t *testing.T) {
	const estimate = 100_000
	const preparedFunds = 750_050 // (100,000 + 150,000) gas * 3 wei/gas + 50 wei bond.
	for _, legacy := range []bool{false, true} {
		for _, balance := range []int64{joinMinimumFundsWei, 250_000, 300_049, 300_050, preparedFunds - 1, preparedFunds} {
			t.Run(fmt.Sprintf("legacy=%t/balance=%d", legacy, balance), func(t *testing.T) {
				f, _, _ := preparePRTAction(t, joinTournament)
				var logs bytes.Buffer
				f.d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
				f.backend.balance = big.NewInt(balance)
				backend := useBalanceCappedEstimator(t, f, estimate, legacy)
				err := f.d.tick(t.Context())
				require.NotEmpty(t, backend.estimates)
				if balance < preparedFunds {
					require.ErrorIs(t, err, txsender.ErrLimit)
					records := fundingErrors(t, &logs)
					require.Len(t, records, 1)
					require.Equal(t, "750050", records[0]["required_wei"])
					require.Equal(t, fmt.Sprint(balance), records[0]["available_wei"])
					require.Empty(t, f.backend.published)
				} else {
					require.NoError(t, err)
					require.Empty(t, fundingErrors(t, &logs))
					require.Len(t, f.backend.published, 1)
				}
			})
		}
	}
}

func TestPRTHeldJoinFundingRefusalAndAffordableReuse(t *testing.T) {
	for _, test := range []struct {
		name         string
		baseFee, tip int64
		refused      bool
	}{
		{"retained bytes remain affordable", 2, 1, false},
		{"replacement exceeds balance", 1, 2, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, call, view := preparePRTAction(t, joinTournament)
			var logs bytes.Buffer
			f.d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			backend := useBalanceCappedEstimator(t, f, intrinsicGasFloor, false)
			submitPRTAction(t, f, call)
			f.backend.balance = big.NewInt(joinPreparedFundsWei)
			backend.tip = big.NewInt(test.tip)
			updatePRTView(f, 111, view)
			f.backend.headers[111].BaseFee.SetInt64(test.baseFee)
			f.block.Hash = f.backend.headers[111].Hash()
			f.consensus.ExpectedCalls = nil
			f.snapshot(view)
			err := f.d.tick(t.Context())
			if test.refused {
				require.ErrorIs(t, err, txsender.ErrLimit)
				records := fundingErrors(t, &logs)
				require.Len(t, records, 1, "a retained transaction must not suppress the current funding refusal")
				require.Equal(t, "684050", records[0]["required_wei"])
				require.Equal(t, "513050", records[0]["available_wei"])
				require.Len(t, f.backend.published, 1)
			} else {
				require.NoError(t, err)
				require.Empty(t, fundingErrors(t, &logs))
				require.Len(t, f.backend.published, 2)
				require.Equal(t, f.backend.raw[0], f.backend.raw[1])
			}
			require.Equal(t, common.Big1, f.backend.published[0].GasTipCap())
		})
	}
}
