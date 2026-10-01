// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

func seedReportInputs(ctx context.Context, t *testing.T, repo repository.Repository) *model.Application {
	t.Helper()
	const (
		inputCount = 3
		epochEnd   = 9
		scanHead   = epochEnd + 1
	)
	app := NewApplicationBuilder().Create(ctx, t, repo)
	epoch := NewEpochBuilder(app.ID).WithStatus(model.EpochStatus_Closed).
		WithBlocks(0, epochEnd).WithInputBounds(0, inputCount).Build()
	inputs := make([]*model.Input, inputCount)
	for i := range inputs {
		inputs[i] = NewInputBuilder().WithIndex(uint64(i)).WithBlockNumber(uint64(i) + 1).Build()
	}
	require.NoError(t, repo.CreateEpochsAndInputs(ctx, app.Name,
		map[*model.Epoch][]*model.Input{epoch: inputs}, scanHead))
	return app
}

func (s *BulkOperationsSuite) TestStoreAdvanceResultReportSequence() {
	for _, firstStatus := range []model.InputCompletionStatus{
		model.InputCompletionStatus_Accepted,
		model.InputCompletionStatus_Rejected,
	} {
		s.Run(firstStatus.String()+"/REJECTED/ACCEPTED", func() {
			app := seedReportInputs(s.Ctx, s.T(), s.Repo)
			statuses := []model.InputCompletionStatus{
				firstStatus, model.InputCompletionStatus_Rejected, model.InputCompletionStatus_Accepted,
			}
			for i, status := range statuses {
				StoreAdvanceResult(s.Ctx, s.T(), s.Repo, app.ID, 0, uint64(i), status,
					nil, [][]byte{[]byte(fmt.Sprintf("report-%d", i))})
			}
			reports, total, err := s.Repo.ListReports(s.Ctx, app.Name,
				repository.ReportFilter{}, repository.Pagination{Limit: uint64(len(statuses))}, false)
			s.Require().NoError(err)
			s.Equal(uint64(len(statuses)), total)
			s.Require().Len(reports, len(statuses))
			for i, report := range reports {
				s.Equal(uint64(i), report.Index)
				s.Equal(uint64(i), report.InputIndex)
				s.Zero(report.EpochIndex)
				s.Equal([]byte(fmt.Sprintf("report-%d", i)), report.RawData)
			}
		})
	}
}

func (s *BulkOperationsSuite) TestStoreAdvanceResultLargeRejectedReportSet() {
	// Cover the extended protocol's parameter boundary and the machine's report limit.
	for _, count := range []uint64{16384, 65536} {
		s.Run(fmt.Sprintf("%d", count), func() {
			seed := Seed(s.Ctx, s.T(), s.Repo)
			payloads := make([][]byte, count)
			for i := range payloads {
				payloads[i] = []byte(fmt.Sprintf("rejected-report-%d", i))
			}
			StoreAdvanceResult(s.Ctx, s.T(), s.Repo, seed.App.ID, 0, 0,
				model.InputCompletionStatus_Rejected, nil, payloads)
			reports, total, err := s.Repo.ListReports(s.Ctx, seed.App.Name,
				repository.ReportFilter{}, repository.Pagination{Limit: count}, false)
			s.Require().NoError(err)
			s.Equal(count, total)
			s.Require().Equal(count, uint64(len(reports)))
			for i, report := range reports {
				s.Equal(uint64(i), report.Index)
				s.Zero(report.InputIndex)
				s.Equal(payloads[i], report.RawData)
			}
		})
	}
}

func (s *BulkOperationsSuite) TestStoreAdvanceResultReportsBeyondBindMessageLimit() {
	if testing.Short() {
		s.T().Skip("transfers more than 1 GiB of report data")
	}
	s.Run("Rejected", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		const (
			reportCount = 520
			payloadSize = 2 * 1024 * 1024
		)
		// Share the source payload and read back samples to bound test memory.
		payload := bytes.Repeat([]byte("x"), payloadSize)
		reports := make([][]byte, reportCount)
		for i := range reports {
			reports[i] = payload
		}
		StoreAdvanceResult(s.Ctx, s.T(), s.Repo, seed.App.ID, 0, 0,
			model.InputCompletionStatus_Rejected, nil, reports)
		page, total, err := s.Repo.ListReports(s.Ctx, seed.App.Name,
			repository.ReportFilter{}, repository.Pagination{Limit: 1}, false)
		s.Require().NoError(err)
		s.Equal(uint64(reportCount), total)
		s.Require().Len(page, 1)
		for _, index := range []uint64{0, reportCount / 2, reportCount - 1} {
			report, err := s.Repo.GetReport(s.Ctx, seed.App.Name, index)
			s.Require().NoError(err)
			s.Require().NotNil(report)
			s.Equal(index, report.Index)
			s.Zero(report.InputIndex)
			s.Equal(payload, report.RawData)
		}
		input, err := s.Repo.GetInput(s.Ctx, seed.App.Name, 0)
		s.Require().NoError(err)
		s.Equal(model.InputCompletionStatus_Rejected, input.Status)
		app, err := s.Repo.GetApplication(s.Ctx, seed.App.Name)
		s.Require().NoError(err)
		s.Equal(uint64(1), app.ProcessedInputs)
	})
}
