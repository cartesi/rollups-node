// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/lmittmann/tint"

	"github.com/ethereum/go-ethereum/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type prtRepositoryMock struct {
	mock.Mock
}

func (m *prtRepositoryMock) ListApplications(
	ctx context.Context,
	f repository.ApplicationFilter,
	pagination repository.Pagination,
	descending bool,
) ([]*model.Application, uint64, error) {
	args := m.Called(ctx, f, pagination, descending)
	return args.Get(0).([]*model.Application), args.Get(1).(uint64), args.Error(2)
}

func newServiceMock() (*Service, *prtRepositoryMock) {
	opts := &tint.Options{
		Level:     slog.LevelDebug,
		AddSource: true,
		// RFC3339 with milliseconds and without timezone
		TimeFormat: "2006-01-02T15:04:05.000",
	}
	handler := tint.NewHandler(os.Stdout, opts)
	repository := &prtRepositoryMock{}

	prt := &Service{
		Service: service.Service{
			Name:   "prt",
			Logger: slog.New(handler),
		},
		repository: repository,
	}
	return prt, repository
}

func makeApplication(id int64) *model.Application {
	return &model.Application{
		ID:                  id,
		IApplicationAddress: common.HexToAddress("0x01"),
		IConsensusAddress:   common.HexToAddress("0x01"),
		IInputBoxAddress:    common.HexToAddress("0x02"),
	}
}

// //////////////////////////////////////////////////////////////////////////////
// Basic Service Tests
// //////////////////////////////////////////////////////////////////////////////
func TestServiceMethods(t *testing.T) {
	s, _ := newServiceMock()

	assert.True(t, s.Alive())
	assert.True(t, s.Ready())
	assert.Empty(t, s.Reload())
	assert.Empty(t, s.Stop(false))
	assert.NotEmpty(t, s.String())
}

func TestTick_NoApplications(t *testing.T) {
	s, r := newServiceMock()
	defer r.AssertExpectations(t)

	r.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]*model.Application{}, uint64(0), nil).Once()

	errs := s.Tick()
	assert.Empty(t, errs)
}

func TestTick_WithApplications(t *testing.T) {
	s, r := newServiceMock()
	defer r.AssertExpectations(t)

	app := makeApplication(1)
	apps := []*model.Application{app}

	r.On("ListApplications", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(apps, uint64(1), nil).Once()

	errs := s.Tick()
	assert.Empty(t, errs)
}
