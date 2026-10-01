// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"fmt"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

func (s *ReportSuite) TestListReportsCompletedStatuses() {
	for _, terminal := range model.InputCompletionStatusAllValues {
		if !terminal.IsTerminal() {
			continue
		}
		s.Run(terminal.String(), func() {
			app := seedReportInputs(s.Ctx, s.T(), s.Repo)
			statuses := []model.InputCompletionStatus{
				model.InputCompletionStatus_Accepted, model.InputCompletionStatus_Rejected, terminal,
			}
			for i, status := range statuses {
				StoreAdvanceResult(s.Ctx, s.T(), s.Repo, app.ID, 0, uint64(i), status,
					nil, [][]byte{[]byte(fmt.Sprintf("report-%d", i))})
			}
			epoch, missingEpoch, rejectedInput := uint64(0), uint64(1), uint64(1)
			const allReports, rejectedAndTerminalReports, terminalReportIndex = 3, 2, 2
			for _, query := range []struct {
				name       string
				filter     repository.ReportFilter
				pagination repository.Pagination
				descending bool
				indices    []uint64
				total      uint64
			}{
				{name: "unfiltered", indices: []uint64{0, 1, terminalReportIndex}, total: allReports},
				{name: "epoch", filter: repository.ReportFilter{EpochIndex: &epoch},
					indices: []uint64{0, 1, terminalReportIndex}, total: allReports},
				{name: "input", filter: repository.ReportFilter{InputIndex: &rejectedInput}, indices: []uint64{1}, total: 1},
				{name: "epoch and input", filter: repository.ReportFilter{EpochIndex: &epoch, InputIndex: &rejectedInput},
					indices: []uint64{1}, total: 1},
				{name: "epoch pagination", filter: repository.ReportFilter{EpochIndex: &epoch},
					pagination: repository.Pagination{Limit: 1, Offset: 1}, descending: true, indices: []uint64{1}, total: allReports},
				{name: "epoch and range", filter: repository.ReportFilter{
					EpochIndex: &epoch, IndexRange: &repository.Range{Start: 1, End: terminalReportIndex},
				}, descending: true, indices: []uint64{terminalReportIndex, 1}, total: rejectedAndTerminalReports},
				{name: "missing epoch", filter: repository.ReportFilter{EpochIndex: &missingEpoch}},
			} {
				reports, total, err := s.Repo.ListReports(s.Ctx, app.Name,
					query.filter, query.pagination, query.descending)
				s.Require().NoError(err, query.name)
				s.Equal(query.total, total, query.name)
				s.Require().Len(reports, len(query.indices), query.name)
				for i, report := range reports {
					index := query.indices[i]
					s.Equal(index, report.Index, query.name)
					s.Equal(index, report.InputIndex, query.name)
					s.Zero(report.EpochIndex, query.name)
					s.Equal([]byte(fmt.Sprintf("report-%d", index)), report.RawData, query.name)
				}
			}
			for i := range statuses {
				report, err := s.Repo.GetReport(s.Ctx, app.Name, uint64(i))
				s.Require().NoError(err)
				s.Require().NotNil(report)
				s.Equal(uint64(i), report.Index)
				s.Equal(uint64(i), report.InputIndex)
				s.Zero(report.EpochIndex)
				s.Equal([]byte(fmt.Sprintf("report-%d", i)), report.RawData)
			}
		})
	}
}
