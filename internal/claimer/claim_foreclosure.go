// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
)

// The complete stream through ref accounts for every prior acceptance. The
// protocol's terminal-foreclosure premise excludes acceptance after this flag.
func (cb *claimerBlockchain) claimForeclosureFinalized(ctx context.Context,
	app *model.Application, ref txsender.BlockRef) (bool, error) {
	if cb.defaultBlock != model.DefaultBlock_Finalized {
		return false, txsender.ErrInconclusive
	}
	if err := cb.checkClaimRead(ctx, ref); err != nil {
		return false, err
	}
	caller, err := iapplication.NewIApplicationCaller(app.IApplicationAddress, txsender.ProtectedClient{Client: cb.client})
	if err != nil {
		return false, err
	}
	foreclosed, err := caller.IsForeclosed(claimScanCallOpts(ctx, ref))
	if err != nil {
		return false, err
	}
	if err = cb.checkClaimReference(ctx, ref); err != nil {
		return false, err
	}
	return foreclosed, nil
}
