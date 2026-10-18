// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"fmt"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"math/big"
)

func addDeliveryJoinCandidate(f *deliveryFixture, id int64, allowance uint64) *model.Application {
	app := *f.app
	app.ID, app.Name = id, fmt.Sprintf("candidate-%d", id)
	app.IApplicationAddress = common.BigToAddress(big.NewInt(id*10 + 1))
	app.IConsensusAddress = common.BigToAddress(big.NewInt(id*10 + 2))
	epoch := *f.epoch
	epoch.ApplicationID, epoch.TournamentAddress = id, new(common.BigToAddress(big.NewInt(id*10+3)))
	f.repo.apps = append(f.repo.apps, &app)
	f.backend.apps = f.repo.apps
	f.repo.On("GetEpoch", mock.Anything, app.IApplicationAddress.Hex(), epoch.Index).Return(&epoch, nil)
	f.repo.publishRoot(&app, &epoch, app.LastTournamentCheckBlock)
	consensus := &daveConsensusAdapterMock{}
	view := resultTestSnapshot(&epoch, false)
	view.stage.IsFinished = false
	consensus.On("GetCurrentSealedEpoch", mock.Anything).Return(view.sealed, nil)
	consensus.On("CanStageTournamentResult", mock.Anything).Return(view.stage, nil)
	consensus.On("CanAcceptStagedTournamentResult", mock.Anything).Return(view.accept, nil)
	f.factory.On("CreateDaveConsensusAdapter", app.IConsensusAddress).Return(consensus, nil)
	deliveryJoinAdapter(f, &epoch, allowance)
	return &app
}

func deliveryJoinAdapter(f *deliveryFixture, epoch *model.Epoch, allowance uint64) {
	adapter := &tournamentAdapterMock{}
	adapter.On("CommitmentStanding", mock.Anything, [32]byte(*epoch.Commitment)).Return(CommitmentStanding{}, nil)
	adapter.On("Descriptor", mock.Anything).Return(TournamentDescriptor{
		Level: 0, Height: model.Log2EpochComputationHashLeafCount, StartInstant: 100, Allowance: allowance,
	}, nil)
	adapter.On("Standing", mock.Anything).Return(TournamentStanding{AcceptsJoins: true}, nil)
	adapter.On("BondValue", mock.Anything).Return(big.NewInt(50), nil)
	f.factory.On("CreateTournamentAdapter", *epoch.TournamentAddress).Return(adapter, nil)
}
