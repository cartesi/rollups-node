// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

const passiveRefundEvent = "PartialBondRefund"

func (f *passiveDisputeFixture) assertAllEvents() {
	contractABI, err := itournament.ITournamentMetaData.GetAbi()
	f.r.NoError(err)
	seen := make(map[string]int)
	for _, layer := range f.layers {
		advances := f.listAdvances(layer)
		advanceByLog := make(map[string]*MatchAdvanced, len(advances))
		for _, event := range advances {
			advanceByLog[passiveEventIdentity(event.TxHash, event.LogIndex)] = event
		}
		tournamentAddress := layer.address.Hex()
		var bonds api.ListResponse[*BondEvent]
		f.r.NoError(f.rpc.Call(f.ctx, "cartesi_listBondEvents", api.ListBondEventsParams{Application: f.appName,
			TournamentAddress: &tournamentAddress, Limit: 100}, &bonds))
		bondByLog := make(map[string]*BondEvent, len(bonds.Data))
		for _, event := range bonds.Data {
			bondByLog[passiveEventIdentity(event.TxHash, event.LogIndex)] = event
			var single api.SingleResponse[*BondEvent]
			f.r.NoError(f.rpc.Call(f.ctx, "cartesi_getBondEvent", api.GetBondEventParams{Application: f.appName,
				TxHash: event.TxHash.Hex(), LogIndex: hexutil.EncodeUint64(event.LogIndex)}, &single))
			f.r.Equal(event, single.Data)
		}
		match := f.observedMatch(layer)
		for _, receipt := range f.receipts {
			for _, raw := range receipt.Logs {
				if raw.Address != layer.address {
					continue
				}
				f.r.NotEmpty(raw.Topics)
				definition, err := contractABI.EventByID(raw.Topics[0])
				f.r.NoError(err)
				seen[definition.Name]++
				switch definition.Name {
				case "CommitmentJoined":
					f.assertJoinedEvent(layer, raw)
				case "MatchCreated":
					event, err := layer.contract.ParseMatchCreated(*raw)
					f.r.NoError(err)
					f.r.Equal(common.Hash(event.MatchIdHash), match.IDHash)
					f.r.Equal(common.Hash(event.One), match.CommitmentOne)
					f.r.Equal(common.Hash(event.Two), match.CommitmentTwo)
					f.r.Equal(common.Hash(event.LeftOfTwo), match.LeftOfTwo)
					f.r.Equal(event.EliminableAt, match.EliminableAt)
					f.assertLogIdentity(raw, match.BlockNumber, match.TxHash, match.LogIndex)
				case "MatchAdvanced":
					event, err := layer.contract.ParseMatchAdvanced(*raw)
					f.r.NoError(err)
					key := passiveEventIdentity(raw.TxHash, uint64(raw.Index))
					row := advanceByLog[key]
					f.r.NotNil(row)
					f.r.Equal(common.Hash(event.MatchIdHash), row.IDHash)
					f.r.Equal(common.Hash(event.OtherParent), row.OtherParent)
					f.r.Equal(common.Hash(event.LeftNode), row.LeftNode)
					f.r.Zero(event.SegmentStartPosition.Cmp(row.SegmentStartPosition.ToBig()))
					f.r.Equal(event.EliminableAt, row.EliminableAt)
					f.assertLogIdentity(raw, row.BlockNumber, row.TxHash, row.LogIndex)
					delete(advanceByLog, key)
				case "LeafMatchSealed":
					event, err := layer.contract.ParseLeafMatchSealed(*raw)
					f.r.NoError(err)
					f.r.NotNil(match.LeafSeal)
					f.r.Equal(event.EliminableAt, match.LeafSeal.EliminableAt)
					f.assertLogIdentity(raw, match.LeafSeal.BlockNumber, match.LeafSeal.TxHash, match.LeafSeal.LogIndex)
				case "MatchDeleted":
					event, err := layer.contract.ParseMatchDeleted(*raw)
					f.r.NoError(err)
					f.r.Equal(common.Hash(event.MatchIdHash), match.IDHash)
					f.r.Equal(uint8(1), event.WinnerCommitment)
					f.r.Equal(WinnerCommitment_ONE, match.Winner)
					f.r.NotNil(match.DeletionTxHash)
					f.r.NotNil(match.DeletionLogIndex)
					f.assertLogIdentity(raw, match.DeletionBlockNumber, *match.DeletionTxHash, *match.DeletionLogIndex)
				case "NewInnerTournament":
					f.assertChildCreationEvent(layer, raw)
				case passiveRefundEvent, "BondRecovered":
					key := passiveEventIdentity(raw.TxHash, uint64(raw.Index))
					f.assertBondEvent(layer, definition.Name, raw, bondByLog[key])
					delete(bondByLog, key)
				default:
					f.r.FailNow("unexpected official tournament event", "%s", definition.Name)
				}
			}
		}
		f.r.Empty(advanceByLog, "every API advance must come from a receipt log")
		f.r.Empty(bondByLog, "every API bond event must come from a receipt log")
	}
	f.r.Equal(map[string]int{
		"CommitmentJoined": 6, "MatchCreated": 3, "MatchAdvanced": 89, "LeafMatchSealed": 1,
		"MatchDeleted": 3, "NewInnerTournament": 2, passiveRefundEvent: 95, "BondRecovered": 1,
	}, seen, "the live fixture must exercise all eight public tournament event types")
}

func (f *passiveDisputeFixture) assertLogIdentity(raw *types.Log, block uint64, hash common.Hash, index uint64) {
	f.t.Helper()
	f.r.Equal(raw.BlockNumber, block)
	f.r.Equal(raw.TxHash, hash)
	f.r.Equal(uint64(raw.Index), index)
}

func (f *passiveDisputeFixture) assertJoinedEvent(layer *passiveDisputeLayer, raw *types.Log) {
	event, err := layer.contract.ParseCommitmentJoined(*raw)
	f.r.NoError(err)
	var response api.SingleResponse[*Commitment]
	f.r.NoError(f.rpc.Call(f.ctx, "cartesi_getCommitment", api.GetCommitmentParams{Application: f.appName,
		EpochIndex: "0x0", TournamentAddress: layer.address.Hex(), Commitment: common.Hash(event.Commitment).Hex()}, &response))
	f.r.NotNil(response.Data)
	f.r.Equal(common.Hash(event.FinalStateHash), response.Data.FinalStateHash)
	f.r.Equal(event.Submitter, response.Data.SubmitterAddress)
	f.assertLogIdentity(raw, response.Data.BlockNumber, response.Data.TxHash, response.Data.LogIndex)
}

func (f *passiveDisputeFixture) assertChildCreationEvent(layer *passiveDisputeLayer, raw *types.Log) {
	event, err := layer.contract.ParseNewInnerTournament(*raw)
	f.r.NoError(err)
	var response api.SingleResponse[*Tournament]
	f.r.NoError(f.rpc.Call(f.ctx, "cartesi_getTournament", api.GetTournamentParams{Application: f.appName,
		Address: event.ChildTournament.Hex()}, &response))
	f.r.NotNil(response.Data)
	child := response.Data
	f.r.NotNil(child.CreationEvent)
	f.r.Equal(&layer.address, child.ParentTournamentAddress)
	f.r.NotNil(child.ParentMatchIDHash)
	f.r.Equal(common.Hash(event.MatchIdHash), *child.ParentMatchIDHash)
	f.assertLogIdentity(raw, child.CreationEvent.BlockNumber, child.CreationEvent.TxHash, child.CreationEvent.LogIndex)
}

func (f *passiveDisputeFixture) assertBondEvent(layer *passiveDisputeLayer, name string, raw *types.Log, row *BondEvent) {
	f.r.NotNil(row)
	f.r.Equal(layer.address, row.TournamentAddress)
	f.r.Zero(row.EpochIndex)
	f.assertLogIdentity(raw, row.BlockNumber, row.TxHash, row.LogIndex)
	if name == passiveRefundEvent {
		event, err := layer.contract.ParsePartialBondRefund(*raw)
		f.r.NoError(err)
		f.r.Equal(BondEventPartialRefund, row.Type)
		f.r.NotNil(row.Refund)
		f.r.Nil(row.Recovery)
		f.r.Equal(event.Recipient, row.Refund.Recipient)
		f.r.Equal(event.Success, row.Refund.Success)
		f.r.Zero(event.Value.Cmp(row.Refund.Value.ToBig()))
	} else {
		event, err := layer.contract.ParseBondRecovered(*raw)
		f.r.NoError(err)
		f.r.Equal(BondEventRecovered, row.Type)
		f.r.Nil(row.Refund)
		f.r.NotNil(row.Recovery)
		f.r.Equal(common.Hash(event.Commitment), row.Recovery.Commitment)
		f.r.Equal(event.Claimer, row.Recovery.Claimer)
		f.r.Equal(f.actors[0].From, row.Recovery.Claimer)
		f.r.Zero(event.Payment.Cmp(row.Recovery.Payment.ToBig()))
		f.r.Zero(event.Burned.Cmp(row.Recovery.Burned.ToBig()))
	}
}
