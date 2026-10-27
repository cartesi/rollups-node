// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestChildCreationComparesEveryDescriptorFieldAndBond(t *testing.T) {
	const level = uint64(1)
	for _, test := range []struct {
		name string
		edit func(*TournamentCreation)
	}{
		{"address", func(c *TournamentCreation) { c.Address[0]++ }},
		{"initial hash", func(c *TournamentCreation) { c.Descriptor.InitialHash[0]++ }},
		{"base cycle high bit", func(c *TournamentCreation) {
			c.Descriptor.BaseCycle = new(big.Int).Add(c.Descriptor.BaseCycle, new(big.Int).Lsh(big.NewInt(1), 64))
		}},
		{"descriptor stride", func(c *TournamentCreation) { c.Descriptor.Log2Stride++ }},
		{"descriptor height", func(c *TournamentCreation) { c.Descriptor.Height++ }},
		{"descriptor level", func(c *TournamentCreation) { c.Descriptor.Level++ }},
		{"kind", func(c *TournamentCreation) { c.Descriptor.Kind = model.TournamentKindNonLeaf }},
		{"start", func(c *TournamentCreation) { c.Descriptor.StartInstant++ }},
		{"allowance", func(c *TournamentCreation) { c.Descriptor.Allowance++ }},
		{"bond high bit", func(c *TournamentCreation) {
			c.BondValue, _ = model.Uint256FromBig(new(big.Int).Add(c.BondValue.ToBig(), new(big.Int).Lsh(big.NewInt(1), 64)))
		}},
		{"creation block", func(c *TournamentCreation) { c.Event.BlockNumber++ }},
	} {
		t.Run(new(big.Int).SetUint64(level).String()+"/"+test.name, func(t *testing.T) {
			d := TournamentDescriptor{InitialHash: common.HexToHash("0x11"), BaseCycle: big.NewInt(7), Log2Stride: 0, Height: 68,
				Level: level, Kind: model.TournamentKindLeaf, StartInstant: 10, Allowance: 20}
			c := creationFixture(t, common.HexToAddress("0xa0"), d)
			row := &model.Tournament{Address: c.Address, InitialHash: common.HexToHash("0x11"), BaseCycle: model.Uint256{7},
				Log2Step: 0,
				Height:   68, Level: level, Kind: model.TournamentKindLeaf, StartInstant: 10, Allowance: 20, BondValue: c.BondValue}
			require.NoError(t, applyTournamentCreationEvent(row, c))
			row.CreationEvent = nil // Exercise first publication without a stored identity guard.
			test.edit(c)
			require.Error(t, applyTournamentCreationEvent(row, c))
			require.Nil(t, row.CreationEvent)
		})
	}
}

func TestNewTournamentRejectsZeroBondBeforeStanding(t *testing.T) {
	s, _ := newPRTServiceMock()
	adapter := &tournamentAdapterMock{}
	adapter.On("Descriptor", mock.Anything).Return(TournamentDescriptor{BaseCycle: big.NewInt(0),
		Kind: model.TournamentKindLeaf}, nil).Once()
	adapter.On("BondValue", mock.Anything).Return(big.NewInt(0), nil).Once()
	row, err := s.readTournament(t.Context(), prtRevertTestApp(), checkpointEpoch(0, "0xa0"), RootLevel, nil, nil,
		common.HexToAddress("0xa0"), adapter, 1, 100)
	require.ErrorContains(t, err, "zero bond")
	require.Nil(t, row)
	adapter.AssertExpectations(t)
	adapter.AssertNotCalled(t, "Standing", mock.Anything)
}
