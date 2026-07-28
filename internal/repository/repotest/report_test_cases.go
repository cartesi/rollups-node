// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

type ReportSuite struct {
	BaseSuite
}

func NewReportSuite(factory RepositoryFactory) *ReportSuite {
	return &ReportSuite{BaseSuite: BaseSuite{factory: factory}}
}

func (s *ReportSuite) TestGetReport() {
	s.Run("ExistingReport", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			nil, [][]byte{[]byte("report-payload")})

		got, err := s.Repo.GetReport(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(uint64(0), got.Index)
		s.Equal([]byte("report-payload"), got.RawData)
	})

	s.Run("NotFound", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		got, err := s.Repo.GetReport(s.Ctx, app.IApplicationAddress.String(), 99)
		s.Require().NoError(err)
		s.Nil(got)
	})
}

func (s *ReportSuite) TestListReports() {
	s.Run("EmptyResult", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)
		reports, total, err := s.Repo.ListReports(
			s.Ctx, app.IApplicationAddress.String(),
			repository.ReportFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(reports)
		s.Equal(uint64(0), total)
	})

	s.Run("ReturnsAllReports", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			nil, [][]byte{[]byte("r0"), []byte("r1"), []byte("r2")})

		reports, total, err := s.Repo.ListReports(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.ReportFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(reports, 3)
		s.Equal(uint64(3), total)
	})

	s.Run("IndexRangeComposesWithPaginationAndDescending", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		s.storeAdvanceResult(seed.App.ID, 0, 0, nil,
			[][]byte{[]byte("r0"), []byte("r1"), []byte("r2"), []byte("r3"), []byte("r4")})

		indexRange := repository.Range{Start: 1, End: 3}
		reports, total, err := s.Repo.ListReports(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.ReportFilter{IndexRange: &indexRange},
			repository.Pagination{Limit: 1, Offset: 1}, true)
		s.Require().NoError(err)
		s.Require().Len(reports, 1)
		s.Equal(uint64(3), total)
		s.Equal(uint64(2), reports[0].Index)
	})

	s.Run("FilterByEpochIndex", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// EpochIndex filter also requires input.status = ACCEPTED,
		// so use StoreAdvanceResult to create the report with accepted input.
		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			Reports:    [][]byte{[]byte("epoch-report")},
			OutputsProof: OutputsProof{
				OutputsHash: UniqueHash(),
				MachineHash: UniqueHash(),
			},
		}
		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().NoError(err)

		epochIdx := uint64(0)
		reports, total, err := s.Repo.ListReports(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.ReportFilter{EpochIndex: &epochIdx},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(reports, 1)
		s.Equal(uint64(1), total)
	})

	s.Run("FilterByInputIndex", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			nil, [][]byte{[]byte("r0"), []byte("r1"), []byte("r2")})

		inputIdx := uint64(0)
		reports, total, err := s.Repo.ListReports(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.ReportFilter{InputIndex: &inputIdx},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(reports, 3)
		s.Equal(uint64(3), total)
	})

	s.Run("Pagination", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		data := make([][]byte, 5)
		for i := range data {
			data[i] = []byte("report-data")
		}
		s.storeAdvanceResult(seed.App.ID, 0, 0, nil, data)

		reports, total, err := s.Repo.ListReports(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.ReportFilter{},
			repository.Pagination{Limit: 2, Offset: 0}, false)
		s.Require().NoError(err)
		s.Len(reports, 2)
		s.Equal(uint64(5), total)
	})

	s.Run("Descending", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		s.storeAdvanceResult(seed.App.ID, 0, 0,
			nil, [][]byte{[]byte("r0"), []byte("r1"), []byte("r2")})

		reports, _, err := s.Repo.ListReports(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.ReportFilter{},
			repository.Pagination{Limit: 10}, true)
		s.Require().NoError(err)
		s.Require().Len(reports, 3)
		// Descending: highest index first
		s.Equal(uint64(2), reports[0].Index)
		s.Equal(uint64(1), reports[1].Index)
		s.Equal(uint64(0), reports[2].Index)
	})
}
