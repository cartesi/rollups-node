// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package prt

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveappfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type joinRevertBackend struct {
	*joinGasBackend
	publications []*types.Transaction
}

func (b *joinRevertBackend) Publish(ctx context.Context, raw []byte) (common.Hash, error) {
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(raw); err != nil {
		return common.Hash{}, err
	}
	b.publications = append(b.publications, tx)
	return b.joinGasBackend.Publish(ctx, raw)
}

// Contracts, RPC, sender, planner and event observation are real. The computed
// epoch and repository use the unit fixture; this does not test PostgreSQL.
func TestAnvilIncludedJoinReplansFromChain(t *testing.T) {
	if os.Getenv("CARTESI_PRT_ANVIL_TEST") != "1" {
		t.Skip("set CARTESI_PRT_ANVIL_TEST=1 for the Anvil join inclusion fixture")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	client, ours, chainID := prtCapacityChain(ctx, t)
	peer := joinGasSigner(ctx, t, client)
	factoryAddress := os.Getenv("CARTESI_CONTRACTS_DAVE_APP_FACTORY_ADDRESS")
	if factoryAddress == "" {
		factoryAddress = "0xF8f6dFA3A3e40C10926839FFd8a221C87C8b2ccD"
	}
	factory, err := idaveappfactory.NewIDaveAppFactory(common.HexToAddress(factoryAddress), client)
	require.NoError(t, err)
	salt := crypto.Keccak256Hash(ours.From.Bytes())
	withdrawal := idaveappfactory.WithdrawalConfig{}
	addresses, err := factory.CalculateDaveAppAddress(&bind.CallOpts{Context: ctx}, salt, big.NewInt(1000),
		ours.From, nil, withdrawal, salt)
	require.NoError(t, err)
	deployment, err := factory.NewDaveApp(ours, salt, big.NewInt(1000), ours.From, nil, withdrawal, salt)
	require.NoError(t, err)
	deployed := joinGasMined(ctx, t, client, deployment, types.ReceiptStatusSuccessful)
	consensus, err := NewDaveConsensusAdapter(addresses.DaveConsensusAddress, client)
	require.NoError(t, err)
	sealed, err := consensus.GetCurrentSealedEpoch(&bind.CallOpts{Context: ctx})
	require.NoError(t, err)
	tournament, err := itournament.NewITournament(sealed.Tournament, client)
	require.NoError(t, err)
	bond, err := tournament.BondValue(&bind.CallOpts{Context: ctx})
	require.NoError(t, err)

	f := newDeliveryFixture(t)
	f.app.IApplicationAddress, f.app.IConsensusAddress = addresses.AppContractAddress, addresses.DaveConsensusAddress
	f.app.LastTournamentCheckBlock = 0
	f.epoch.Index, f.epoch.LastBlock = sealed.EpochNumber, deployed.BlockNumber.Uint64()
	f.epoch.InputIndexLowerBound, f.epoch.InputIndexUpperBound = sealed.InputIndexLowerBound, sealed.InputIndexUpperBound
	f.epoch.TournamentAddress = &sealed.Tournament
	position := new(big.Int).SetUint64(1<<model.Log2EpochComputationHashLeafCount - 1)
	proof := merkle.Proof{Node: *f.epoch.MachineHash, Pos: position, Siblings: f.epoch.CommitmentProof}
	f.epoch.Commitment = new(proof.BuildRoot())
	f.repo.signer, f.repo.publishedRoots = ours.From, nil
	f.repo.On("GetEpoch", mock.Anything, f.app.IApplicationAddress.Hex(), f.epoch.Index).Return(f.epoch, nil)
	f.repo.On("GetLastNonOpenEpochIndex", mock.Anything, mock.Anything).Return((*uint64)(nil), nil)
	f.repo.On("ListTournaments", mock.Anything, f.app.Name, repository.TournamentFilter{}, repository.Pagination{}, false).
		Return([]*model.Tournament{}, uint64(0), nil)
	f.repo.On("ListCommitments", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Commitment{}, uint64(0), nil)
	f.repo.On("ListMatches", mock.Anything, f.app.Name, mock.Anything, repository.Pagination{}, false).
		Return([]*model.Match{}, uint64(0), nil)
	admitTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	f.d.client, f.d.adapterFactory, f.d.defaultBlock = client, NewDefaultAdapterFactory(client), model.DefaultBlock_Latest
	f.d.txOptsFactory = ethutil.NewStaticTransactOptsFactory(ours)
	backend := &joinRevertBackend{joinGasBackend: &joinGasBackend{ProtectedClient: txsender.ProtectedClient{Client: client}}}
	f.d.deliveryBackend = backend
	f.d.sender, err = txsender.New[actionID](backend, f.d.txOptsFactory, chainID.Uint64(), txsender.Policy{},
		f.d.guardPublication, f.d.Logger)
	require.NoError(t, err)
	observe := func() {
		header, readErr := client.HeaderByNumber(ctx, nil)
		require.NoError(t, readErr)
		facts := txsender.BlockRef{Number: header.Number.Uint64(), Hash: header.Hash()}
		require.NoError(t, f.d.observeWindow(ctx, f.app, []*model.Epoch{f.epoch}, consensus, facts))
	}
	// The producer can advance between the account and observation reads.
	// Retry that ordinary wait, as the service loop does; other errors fail.
	tick := func() {
		tickCtx, stop := context.WithTimeout(ctx, 10*time.Second)
		defer stop()
		retry := time.NewTicker(100 * time.Millisecond)
		defer retry.Stop()
		for {
			tickErr := f.d.tick(tickCtx)
			if !errors.Is(tickErr, txsender.ErrInconclusive) || !txsender.WaitOnly(tickErr) {
				require.NoError(t, tickErr)
				return
			}
			select {
			case <-tickCtx.Done():
				require.NoError(t, tickCtx.Err())
				return
			case <-retry.C:
			}
		}
	}
	observe()
	require.Len(t, f.repo.windows, 1, "fresh work needs an observed root before publication")
	var peerReceipt *types.Receipt
	backend.beforePublish = func(publishCtx context.Context) {
		left, right, proofErr := merkle.RootChildrenFromProof(*f.epoch.MachineHash, f.epoch.CommitmentProof, position.Uint64())
		require.NoError(t, proofErr)
		peerOpts := *peer
		peerOpts.Context, peerOpts.Value = publishCtx, bond
		tx, joinErr := tournament.JoinTournament(&peerOpts, *f.epoch.MachineHash,
			hashSliceToByteSlice(f.epoch.CommitmentProof), left, right)
		require.NoError(t, joinErr)
		peerReceipt = joinGasMined(publishCtx, t, client, tx, types.ReceiptStatusSuccessful)
	}
	tick()
	require.Len(t, backend.publications, 1)
	oursTx := backend.publications[0]
	oursReceipt := joinGasMined(ctx, t, client, oursTx, types.ReceiptStatusFailed)
	require.NotNil(t, peerReceipt)
	require.Negative(t, peerReceipt.BlockNumber.Cmp(oursReceipt.BlockNumber), "the hook waits for the peer before our POST")
	require.Less(t, oursReceipt.GasUsed, oursTx.Gas(), "the receipt must not be an out-of-gas result")
	// Our reverted call did not change tournament state. Replay at its receipt
	// block also preserves the inclusion clock, unlike an earlier peer block.
	_, replayErr := client.CallContract(ctx, ethereum.CallMsg{
		From: ours.From, To: oursTx.To(), Value: oursTx.Value(), Data: oursTx.Data(), Gas: oursTx.Gas(),
		GasFeeCap: oursTx.GasFeeCap(), GasTipCap: oursTx.GasTipCap(),
	}, oursReceipt.BlockNumber)
	require.Error(t, replayErr)
	revertData, ok := ethclient.RevertErrorData(replayErr)
	require.True(t, ok)
	contractABI, err := itournament.ITournamentMetaData.GetAbi()
	require.NoError(t, err)
	initialized, ok := contractABI.Errors["ClockAlreadyInitialized"]
	require.True(t, ok)
	require.Equal(t, initialized.ID[:4], revertData)
	require.Empty(t, oursReceipt.Logs, "the reverted join emits no CommitmentJoined from the node")
	nonce, err := client.NonceAt(ctx, ours.From, nil)
	require.NoError(t, err)
	require.Equal(t, oursTx.Nonce()+1, nonce, "the included revert still consumes the node's nonce")
	observe()
	require.Len(t, f.repo.windows, 2)
	batch := f.repo.windows[1].Batches[0]
	require.Len(t, batch.Commitments, 1)
	require.Equal(t, *f.epoch.Commitment, batch.Commitments[0].Commitment)
	require.Equal(t, peer.From, batch.Commitments[0].SubmitterAddress)
	require.Equal(t, peerReceipt.TxHash, batch.Commitments[0].TxHash)
	require.NotEqual(t, oursReceipt.TxHash, batch.Commitments[0].TxHash)
	adapter, err := NewITournamentAdapter(sealed.Tournament, client)
	require.NoError(t, err)
	header, err := client.HeaderByNumber(ctx, nil)
	require.NoError(t, err)
	live := txsender.BlockRef{Number: header.Number.Uint64(), Hash: header.Hash()}
	opts := &bind.CallOpts{Context: ctx, BlockHash: live.Hash}
	standing, err := adapter.CommitmentStanding(opts, *f.epoch.Commitment)
	require.NoError(t, err)
	require.True(t, standing.Joined)
	require.Equal(t, peer.From, standing.Claimer)
	descriptor, err := adapter.Descriptor(opts)
	require.NoError(t, err)
	require.Less(t, live.Number+1, descriptor.StartInstant+descriptor.Allowance, "the no-work verdict is not deadline refusal")
	plan, err := f.d.planAction(ctx, f.app, joinTournament, live, live)
	require.NoError(t, err)
	require.Nil(t, plan, "the contract standing makes this ordinary no-work, not a paused publication")
	tick()
	require.Len(t, backend.publications, 1, "an already joined commitment is ordinary no-work after the revert")
	require.NotContains(t, logs.String(), "PRT cannot join tournament")
	require.NotContains(t, logs.String(), "level=ERROR")
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	require.Equal(t, model.EpochStatus_ClaimComputed, f.epoch.Status)
	require.Nil(t, f.epoch.ClaimTransactionHash)
	require.False(t, f.d.publicationStopped())
	f.repo.AssertNotCalled(t, "UpdateApplicationStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	t.Logf("peer=%s status=%d block=%s; node=%s status=%d block=%s nonce=%d",
		peerReceipt.TxHash, peerReceipt.Status, peerReceipt.BlockNumber,
		oursReceipt.TxHash, oursReceipt.Status, oursReceipt.BlockNumber, oursTx.Nonce())
}
