// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"github.com/cartesi/rollups-node/internal/model"
	"math/big"
)

type claimWork struct {
	app   *model.Application
	epoch *model.Epoch
}

func (s *Service) observeClaims(
	ctx context.Context, epochs map[int64]*model.Epoch, apps map[int64]*model.Application, block *big.Int,
	process func(context.Context, claimWork, *big.Int) claimStepResult,
) (int, error) {
	transitions := 0
	var err error
	for id, epoch := range epochs {
		if ctx.Err() != nil {
			return transitions, errors.Join(err, ctx.Err())
		}
		result := process(ctx, claimWork{app: apps[id], epoch: epoch}, block)
		transitions += result.progress
		err = errors.Join(err, result.err)
		if result.drop {
			delete(epochs, id)
		}
	}
	return transitions, err
}
