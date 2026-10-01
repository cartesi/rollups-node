// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"bytes"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

func (s *BulkOperationsSuite) TestStoreAdvanceResultLargeEvidenceSet() {
	// Cover the extended protocol's parameter boundary and the machine's row limit.
	for _, rowCount := range []uint64{16384, 65536} {
		s.Run(fmt.Sprintf("Accepted%d", rowCount), func() {
			seed := Seed(s.Ctx, s.T(), s.Repo)
			outputPayloads := make([][]byte, rowCount)
			reportPayloads := make([][]byte, rowCount)
			for i := range outputPayloads {
				outputPayloads[i] = []byte(fmt.Sprintf("output-%d", i))
				reportPayloads[i] = []byte(fmt.Sprintf("report-%d", i))
			}
			s.storeAdvanceResult(seed.App.ID, 0, 0, outputPayloads, reportPayloads)

			outputs, total, err := s.Repo.ListOutputs(s.Ctx, seed.App.Name,
				repository.OutputFilter{}, repository.Pagination{Limit: rowCount}, false)
			s.Require().NoError(err)
			s.Equal(rowCount, total)
			s.Require().Equal(rowCount, uint64(len(outputs)))
			for i, output := range outputs {
				s.Equal(uint64(i), output.Index)
				s.Zero(output.InputIndex)
				s.Equal(outputPayloads[i], output.RawData)
			}

			reports, total, err := s.Repo.ListReports(s.Ctx, seed.App.Name,
				repository.ReportFilter{}, repository.Pagination{Limit: rowCount}, false)
			s.Require().NoError(err)
			s.Equal(rowCount, total)
			s.Require().Equal(rowCount, uint64(len(reports)))
			for i, report := range reports {
				s.Equal(uint64(i), report.Index)
				s.Zero(report.InputIndex)
				s.Equal(reportPayloads[i], report.RawData)
			}
		})
	}
}

func (s *BulkOperationsSuite) TestStoreAdvanceResultEvidencePayloads() {
	s.Run("LargeAndEmpty", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		const payloadSize = 2 * 1024 * 1024
		payloads := [][]byte{bytes.Repeat([]byte("x"), payloadSize), {}, []byte("tail")}
		s.storeAdvanceResult(seed.App.ID, 0, 0, payloads, payloads)

		for i, payload := range payloads {
			output, err := s.Repo.GetOutput(s.Ctx, seed.App.Name, uint64(i))
			s.Require().NoError(err)
			s.Require().NotNil(output)
			s.Equal(payload, output.RawData)
			s.Nil(output.Hash)
			s.Nil(output.OutputHashesSiblings)
			s.Nil(output.ExecutionTransactionHash)
			s.False(output.CreatedAt.IsZero())
			s.True(output.CreatedAt.Equal(output.UpdatedAt))
			report, err := s.Repo.GetReport(s.Ctx, seed.App.Name, uint64(i))
			s.Require().NoError(err)
			s.Require().NotNil(report)
			s.Equal(payload, report.RawData)
			s.False(report.CreatedAt.IsZero())
			s.True(report.CreatedAt.Equal(report.UpdatedAt))
		}
	})
}

func (s *BulkOperationsSuite) TestStoreAdvanceResultRollsBackEvidence() {
	for _, kind := range []string{"output", "report"} {
		s.Run(kind, func() { s.checkEvidenceRollback(kind) })
	}
}

func (s *BulkOperationsSuite) checkEvidenceRollback(kind string) {
	seed := Seed(s.Ctx, s.T(), s.Repo)
	const rowCount = 16384
	payloads := make([][]byte, rowCount)
	for i := range payloads {
		payloads[i] = []byte("valid payload")
	}
	// A late NULL must undo every earlier row in the result transaction.
	payloads[len(payloads)-1] = nil
	beforeEpoch, err := s.Repo.GetEpoch(s.Ctx, seed.App.Name, 0)
	s.Require().NoError(err)
	beforeApp, err := s.Repo.GetApplication(s.Ctx, seed.App.Name)
	s.Require().NoError(err)
	result := &model.AdvanceResult{
		Status:     model.InputCompletionStatus_Accepted,
		Outputs:    [][]byte{[]byte("must roll back")},
		Reports:    payloads,
		StateProof: *DummyStateProof(),
	}
	if kind == "output" {
		result.Outputs, result.Reports = result.Reports, result.Outputs
	}
	err = s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
	var constraint *pgconn.PgError
	s.Require().ErrorAs(err, &constraint)
	s.Equal("23502", constraint.Code)
	s.Equal("raw_data", constraint.ColumnName)
	s.Contains(err.Error(), "failed to copy "+kind+" rows")

	reports, reportCountAfter, err := s.Repo.ListReports(s.Ctx, seed.App.Name,
		repository.ReportFilter{}, repository.Pagination{Limit: 1}, false)
	s.Require().NoError(err)
	s.Empty(reports)
	s.Zero(reportCountAfter)
	outputs, outputCount, err := s.Repo.ListOutputs(s.Ctx, seed.App.Name,
		repository.OutputFilter{}, repository.Pagination{Limit: 1}, false)
	s.Require().NoError(err)
	s.Empty(outputs)
	s.Zero(outputCount)
	input, err := s.Repo.GetInput(s.Ctx, seed.App.Name, 0)
	s.Require().NoError(err)
	s.Equal(model.InputCompletionStatus_None, input.Status)
	s.Nil(input.MachineHash)
	s.Nil(input.TxBufferDataBlock)
	afterEpoch, err := s.Repo.GetEpoch(s.Ctx, seed.App.Name, 0)
	s.Require().NoError(err)
	s.Equal(beforeEpoch, afterEpoch)
	afterApp, err := s.Repo.GetApplication(s.Ctx, seed.App.Name)
	s.Require().NoError(err)
	s.Equal(beforeApp, afterApp)
}
