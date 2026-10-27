// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/mock"
)

func expectEpochOutputs(repo *Mockrepo, app *model.Application, epoch *model.Epoch, outputs []*model.Output) {
	span := repository.EpochOutputRange{Count: uint64(len(outputs))}
	if len(outputs) != 0 {
		span.First, span.Last = outputs[0].Index, outputs[len(outputs)-1].Index
	}
	repo.On("GetEpochOutputRange", mock.Anything, app.ID, epoch.Index).Return(span, nil).Once()
	if len(outputs) == 0 {
		return
	}
	repo.On("ScanOutputs", mock.Anything, app.ID, mock.Anything, mock.Anything, mock.Anything).
		Return(func(fn repository.OutputScanFunc) error {
			for _, output := range outputs {
				if err := fn(output.Index, output.RawData); err != nil {
					return err
				}
			}
			return nil
		}, nil).Maybe()
}
