// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"github.com/cartesi/rollups-node/internal/repository"
)

const claimPageSize uint64 = 16

const (
	claimSubmitPage = iota
	claimStagePage
	claimAcceptPage
)

// Observation has its own application rotation and per-app epoch positions.
func (s *Service) selectClaimPage(ctx context.Context, kind int) (repository.ClaimPage, error) {
	selectPage := s.repository.SelectClaimsToSubmitPage
	if kind == claimStagePage {
		selectPage = s.repository.SelectClaimsToStagePage
	}
	if kind == claimAcceptPage {
		selectPage = s.repository.SelectClaimsToAcceptPage
	}
	page, err := selectPage(ctx, s.pageCursors[kind], claimPageSize)
	if err != nil {
		return page, err
	}
	s.pageCursors[kind].AfterApplicationID = page.ApplicationID
	return page, nil
}
