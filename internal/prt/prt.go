// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"errors"
	"fmt"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
)

type PrtRepository interface {
	ListApplications(ctx context.Context, f repository.ApplicationFilter, p repository.Pagination, descending bool) ([]*Application, uint64, error)
	UpdateApplicationState(ctx context.Context, appID int64, state ApplicationState, reason *string) error
}

func getAllRunningApplications(ctx context.Context, er PrtRepository) ([]*Application, uint64, error) {
	f := repository.ApplicationFilter{State: Pointer(ApplicationState_Enabled)}
	return er.ListApplications(ctx, f, repository.Pagination{}, false)
}

// setApplicationInoperable marks an application as inoperable with the given reason,
// logs any error that occurs during the update, and returns an error with the reason.
func (v *Service) setApplicationInoperable(ctx context.Context, app *Application, reasonFmt string, args ...interface{}) error {
	reason := fmt.Sprintf(reasonFmt, args...)
	appAddress := app.IApplicationAddress.String()

	// Log the reason first
	v.Logger.Error(reason, "application", appAddress)

	// Update application state
	err := v.repository.UpdateApplicationState(ctx, app.ID, ApplicationState_Inoperable, &reason)
	if err != nil {
		v.Logger.Error("failed to update application state to inoperable", "app", appAddress, "err", err)
	}

	// Return the error with the reason
	return errors.New(reason)
}

func (v *Service) validateApplication(ctx context.Context, app *Application) error {
	v.Logger.Debug("Starting validation", "application", app.Name)
	return nil
}
