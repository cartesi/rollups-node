// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func (f *passiveDisputeFixture) joinPair(layer *passiveDisputeLayer) {
	for side, tree := range layer.trees {
		if side == 1 && layer.descriptor.Level == 2 {
			// A late join receives less allowance. Preserve this deliberate
			// gap through the leaf race, so only B expires at its deadline.
			f.r.NoError(anvilMine(f.ctx, passiveLeafJoinDelay))
		}
		bond, err := layer.contract.BondValue(&bind.CallOpts{Context: f.ctx})
		f.r.NoError(err)
		opts := *f.actors[side]
		opts.Value = bond
		left, right, err := tree.rightmostChildren(tree.height)
		f.r.NoError(err)
		finalState, proof, err := tree.proof(tree.leafCount() - 1)
		f.r.NoError(err)
		tx, err := layer.contract.JoinTournament(&opts, finalState, proof, left, right)
		f.record(tx, err)
	}
}

func (f *passiveDisputeFixture) bisect(layer *passiveDisputeLayer) {
	var responder uint64
	for height := layer.descriptor.Height; height > 1; height-- {
		tree := layer.trees[responder]
		left, right, err := tree.rightmostChildren(height)
		f.r.NoError(err)
		nextLeft, nextRight, err := tree.rightmostChildren(height - 1)
		f.r.NoError(err)
		tx, err := layer.contract.AdvanceMatch(f.actors[responder], layer.matchID, left, right, nextLeft, nextRight)
		f.record(tx, err)
		responder ^= 1
		if height == layer.descriptor.Height {
			// Check a real intermediate snapshot, not only the final state.
			f.assertCurrent(layer, MatchPhaseBisecting)
		}
	}
}

func (f *passiveDisputeFixture) seal(layer *passiveDisputeLayer) common.Address {
	responder := (layer.descriptor.Height - 1) % 2
	tree := layer.trees[responder]
	left, right, err := tree.rightmostChildren(1)
	f.r.NoError(err)
	agree, proof, err := tree.proof(tree.leafCount() - 2)
	f.r.NoError(err)
	if layer.descriptor.Level == 2 {
		tx, err := layer.contract.SealLeafMatch(f.actors[responder], layer.matchID, left, right, agree, proof)
		f.record(tx, err)
		return common.Address{}
	}
	tx, err := layer.contract.SealInnerMatchAndCreateInnerTournament(f.actors[responder], layer.matchID,
		left, right, agree, proof)
	receipt := f.record(tx, err)
	for _, raw := range receipt.Logs {
		if raw.Address != layer.address {
			continue
		}
		event, err := layer.contract.ParseNewInnerTournament(*raw)
		if err == nil {
			f.r.Equal(layer.matchHash, common.Hash(event.MatchIdHash))
			return event.ChildTournament
		}
	}
	f.r.FailNow("successful inner seal must emit NewInnerTournament")
	return common.Address{}
}

func (f *passiveDisputeFixture) winLeafByTimeout() {
	layer := f.layers[len(f.layers)-1]
	one, err := layer.contract.CommitmentStanding(&bind.CallOpts{Context: f.ctx}, layer.trees[0].root())
	f.r.NoError(err)
	two, err := layer.contract.CommitmentStanding(&bind.CallOpts{Context: f.ctx}, layer.trees[1].root())
	f.r.NoError(err)
	f.r.True(one.ClockRunning)
	f.r.True(two.ClockRunning)
	f.r.Greater(one.ClockDeadline, two.ClockDeadline+uint64(len(f.layers)),
		"the winner must retain time to propagate through both parent tournaments")
	f.mineTo(two.ClockDeadline)
	timeout, err := layer.contract.ClassifyMatchTimeout(&bind.CallOpts{Context: f.ctx}, layer.matchID)
	f.r.NoError(err)
	f.r.Equal(uint8(1), timeout.Outcome, "only actor A must win, not ELIMINATE_BOTH")
	f.assertCurrent(layer, MatchPhaseSealed)
	left, right, err := layer.trees[0].rightmostChildren(layer.descriptor.Height)
	f.r.NoError(err)
	tx, err := layer.contract.WinMatchByTimeout(f.actors[0], layer.matchID, left, right)
	f.record(tx, err)
	f.assertCurrent(layer, MatchPhaseUninitialized)
}

// Receipt events cannot prove that no extra reverted/no-log transaction was
// sent. Inspect all transactions targeting this fixture's tournaments as well.
func (f *passiveDisputeFixture) assertOnlyExternalTransactions() {
	expected := make(map[common.Hash]struct{}, len(f.receipts))
	for _, receipt := range f.receipts {
		expected[receipt.TxHash] = struct{}{}
	}
	addresses := make(map[common.Address]struct{}, len(f.layers))
	for _, layer := range f.layers {
		addresses[layer.address] = struct{}{}
	}
	start := f.receipts[0].BlockNumber.Uint64()
	end, err := f.client.BlockNumber(f.ctx)
	f.r.NoError(err)
	for height := start; height <= end; height++ {
		block, err := f.client.BlockByNumber(f.ctx, new(big.Int).SetUint64(height))
		f.r.NoError(err)
		for _, tx := range block.Transactions() {
			if tx.To() == nil {
				continue
			}
			if _, ok := addresses[*tx.To()]; !ok {
				continue
			}
			f.r.Contains(expected, tx.Hash(), "the node must not send any tournament transaction")
			sender, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
			f.r.NoError(err)
			f.r.Contains([]common.Address{f.actors[0].From, f.actors[1].From}, sender)
		}
	}
}
