// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import "github.com/cartesi/rollups-node/internal/model"

type ClaimWork struct {
	Application *model.Application
	Epoch       *model.Epoch
}

// ClaimPage is one application's oldest row and its resumed later-row window.
type ClaimPage struct {
	Work          []ClaimWork
	ApplicationID int64
	NextEpoch     *uint64
}

// ClaimCursor rotates application IDs and resumes epochs independently within
// each application. Epoch numbers from different applications are never compared.
// Observer and publisher keep separate cursors for each stored claim status.
type ClaimCursor struct {
	AfterApplicationID int64
	Epochs             map[int64]uint64
	// PublishableOnly limits publisher pagination to eligible epoch prefixes.
	// Observers leave it false to reconcile every enabled status.
	PublishableOnly bool
}

func (c *ClaimCursor) Advance(work ClaimWork, page ClaimPage) {
	c.AfterApplicationID = work.Application.ID
	if c.Epochs == nil {
		c.Epochs = make(map[int64]uint64)
	}
	if len(page.Work) == 0 {
		return
	}
	if work.Epoch.Index > page.Work[0].Epoch.Index {
		c.Epochs[work.Application.ID] = work.Epoch.Index
	}
	if page.NextEpoch == nil && work.Epoch.Index == page.Work[len(page.Work)-1].Epoch.Index {
		delete(c.Epochs, work.Application.ID)
	}
}
