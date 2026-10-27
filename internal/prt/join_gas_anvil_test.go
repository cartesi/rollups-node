// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package prt

import (
	"context"
	"log/slog"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
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
	"github.com/stretchr/testify/require"
)

// The peer's join lands after our last simulation and before our POST. The same
// execution branch can change when the peer precedes us within one block.
type joinGasBackend struct {
	txsender.ProtectedClient
	beforePublish func()
}

func (b *joinGasBackend) Publish(ctx context.Context, raw []byte) (common.Hash, error) {
	if b.beforePublish != nil {
		hook := b.beforePublish
		b.beforePublish = nil
		hook()
	}
	return b.ProtectedClient.Publish(ctx, raw)
}

type joinGasPlanner struct{ request txsender.Request }

func (p joinGasPlanner) Next(context.Context, txsender.BlockRef, *txsender.Current[uint8]) (txsender.Decision[uint8], error) {
	return txsender.Publish(uint8(1), p.request), nil
}

func (joinGasPlanner) Recheck(_ context.Context, _ txsender.BlockRef, _ uint8) (txsender.Eligibility, error) {
	return txsender.Useful, nil
}

func TestAnvilJoinGasAfterPeerJoin(t *testing.T) {
	if os.Getenv("CARTESI_PRT_ANVIL_TEST") != "1" {
		t.Skip("set CARTESI_PRT_ANVIL_TEST=1 for the isolated Anvil release fixture")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	client, err := ethutil.DialEthClient(ctx, os.Getenv("CARTESI_BLOCKCHAIN_HTTP_ENDPOINT"))
	require.NoError(t, err)
	t.Cleanup(client.Close)
	factoryAddress := os.Getenv("CARTESI_CONTRACTS_DAVE_APP_FACTORY_ADDRESS")
	if factoryAddress == "" {
		factoryAddress = "0xd34BEC37Fa5816ABA2f87BdaD2E13dd1B161370f"
	}
	factory, err := idaveappfactory.NewIDaveAppFactory(common.HexToAddress(factoryAddress), client)
	require.NoError(t, err)
	for _, fixture := range []struct {
		name      string
		peerFirst bool
		estimate  bool
		halfOnly  bool
	}{
		{name: "estimate_only_without_peer", estimate: true},
		{name: "estimate_only_loses_pairing_race", peerFirst: true, estimate: true},
		{name: "half_padding_alone_loses_pairing_race", peerFirst: true, halfOnly: true},
		{name: "padded_estimate_survives_pairing_race", peerFirst: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ours, peer := joinGasSigner(ctx, t, client), joinGasSigner(ctx, t, client)
			salt := crypto.Keccak256Hash(ours.From.Bytes())
			addresses, err := factory.CalculateDaveAppAddress(&bind.CallOpts{Context: ctx}, salt,
				big.NewInt(1000), ours.From, nil, idaveappfactory.WithdrawalConfig{}, salt)
			require.NoError(t, err)
			deployment, err := factory.NewDaveApp(ours, salt, big.NewInt(1000), ours.From,
				nil, idaveappfactory.WithdrawalConfig{}, salt)
			require.NoError(t, err)
			joinGasMined(ctx, t, client, deployment, types.ReceiptStatusSuccessful)
			consensus, err := NewDaveConsensusAdapter(addresses.DaveConsensusAddress, client)
			require.NoError(t, err)
			sealed, err := consensus.GetCurrentSealedEpoch(&bind.CallOpts{Context: ctx})
			require.NoError(t, err)
			tournament, err := itournament.NewITournament(sealed.Tournament, client)
			require.NoError(t, err)
			bond, err := tournament.BondValue(&bind.CallOpts{Context: ctx})
			require.NoError(t, err)
			machine := common.HexToHash("0x0a")
			epoch := &model.Epoch{MachineHash: &machine, CommitmentProof: make([]common.Hash, model.Log2EpochComputationHashLeafCount)}
			data, err := joinData(epoch)
			require.NoError(t, err)
			request := txsender.Request{To: sealed.Tournament, Data: data, Value: bond}
			estimated, err := client.EstimateGas(ctx, ethereum.CallMsg{From: ours.From, To: &request.To, Data: data, Value: bond})
			require.NoError(t, err)
			var controlGas uint64
			if fixture.estimate {
				controlGas = estimated
			}
			if fixture.halfOnly {
				controlGas = estimated + estimated/2
			}
			backend := &joinGasBackend{ProtectedClient: txsender.ProtectedClient{Client: client}}
			if fixture.peerFirst {
				backend.beforePublish = func() {
					peerHash := common.HexToHash("0x0b")
					proof := make([]common.Hash, model.Log2EpochComputationHashLeafCount)
					position := uint64(1<<model.Log2EpochComputationHashLeafCount) - 1
					left, right, err := merkle.RootChildrenFromProof(peerHash, proof, position)
					require.NoError(t, err)
					peer.Value = bond
					tx, err := tournament.JoinTournament(peer, peerHash, hashSliceToByteSlice(proof), left, right)
					require.NoError(t, err)
					joinGasMined(ctx, t, client, tx, types.ReceiptStatusSuccessful)
				}
			}
			chainID, err := client.ChainID(ctx)
			require.NoError(t, err)
			var hash common.Hash
			if controlGas != 0 {
				hash = joinGasPublishControl(ctx, t, backend, ours, chainID, request, controlGas)
			} else {
				sender, err := txsender.New[uint8](backend, ethutil.NewStaticTransactOptsFactory(ours), chainID.Uint64(),
					txsender.Policy{}, func(context.Context) error { return nil }, slog.Default())
				require.NoError(t, err)
				report, err := sender.PublishNext(ctx, joinGasPlanner{request: request})
				require.NoError(t, err)
				require.True(t, report.Attempted)
				hash = report.Hash
			}
			tx, _, err := client.TransactionByHash(ctx, hash)
			require.NoError(t, err)
			require.Equal(t, data, tx.Data())
			require.Equal(t, sealed.Tournament, *tx.To())
			require.Equal(t, bond, tx.Value())
			if controlGas != 0 {
				require.Equal(t, controlGas, tx.Gas(), "the control must use its exact insufficient-gas envelope")
			} else {
				require.GreaterOrEqual(t, tx.Gas(), estimated+max(estimated, 150_000),
					"value-bearing joins need branch padding beyond the lone-commitment estimate")
			}
			wantStatus := uint64(types.ReceiptStatusSuccessful)
			if fixture.peerFirst && (fixture.estimate || fixture.halfOnly) {
				wantStatus = types.ReceiptStatusFailed
			}
			receipt := joinGasMined(ctx, t, client, tx, wantStatus)
			joined := false
			for _, log := range receipt.Logs {
				if log.Address != sealed.Tournament {
					continue
				}
				event, parseErr := tournament.ParseCommitmentJoined(*log)
				if parseErr == nil && event.Submitter == ours.From {
					require.Equal(t, [32]byte(machine), event.FinalStateHash)
					position := uint64(1<<model.Log2EpochComputationHashLeafCount) - 1
					left, right, proofErr := merkle.RootChildrenFromProof(machine, epoch.CommitmentProof, position)
					require.NoError(t, proofErr)
					require.Equal(t, [32]byte(crypto.Keccak256Hash(left[:], right[:])), event.Commitment)
					joined = true
				}
			}
			require.Equal(t, wantStatus == types.ReceiptStatusSuccessful, joined, "only successful envelopes may join our commitment")
			t.Logf("estimate=%d signed_gas=%d used=%d status=%d", estimated, tx.Gas(), receipt.GasUsed, receipt.Status)
		})
	}
}

// GasLimit is a service floor. Negative controls bypass sender sizing so their
// exact raw/half-padded envelopes stay insufficient after the peer's join.
// The publication hook still runs strictly after this last exact preflight.
func joinGasPublishControl(
	ctx context.Context, t *testing.T, backend *joinGasBackend, signer *bind.TransactOpts,
	chainID *big.Int, request txsender.Request, gas uint64,
) common.Hash {
	t.Helper()
	header, err := backend.Header(ctx, "latest")
	require.NoError(t, err)
	block := txsender.BlockRef{Number: header.Number.Uint64(), Hash: header.Hash()}
	nonce, err := backend.Nonce(ctx, signer.From, block)
	require.NoError(t, err)
	tip, err := backend.Tip(ctx)
	require.NoError(t, err)
	feeCap := new(big.Int).Add(new(big.Int).Mul(header.BaseFee, big.NewInt(2)), tip)
	unsigned := types.NewTx(&types.DynamicFeeTx{
		ChainID: chainID, Nonce: nonce, To: &request.To, Value: request.Value, Data: request.Data,
		Gas: gas, GasFeeCap: feeCap, GasTipCap: tip,
	})
	signed, err := signer.Signer(signer.From, unsigned)
	require.NoError(t, err)
	raw, err := signed.MarshalBinary()
	require.NoError(t, err)
	_, err = backend.Call(ctx, ethereum.CallMsg{
		From: signer.From, To: &request.To, Value: request.Value, Data: request.Data,
		Gas: gas, GasFeeCap: feeCap, GasTipCap: tip,
	}, block)
	require.NoError(t, err, "the control envelope must work before the competing join")
	hash, err := backend.Publish(ctx, raw)
	require.NoError(t, err)
	require.Equal(t, signed.Hash(), hash)
	return hash
}

func joinGasSigner(ctx context.Context, t *testing.T, client *ethclient.Client) *bind.TransactOpts {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	chainID, err := client.ChainID(ctx)
	require.NoError(t, err)
	opts, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	require.NoError(t, err)
	var result any
	require.NoError(t, client.Client().CallContext(ctx, &result, "anvil_setBalance", opts.From, "0x56bc75e2d63100000"))
	opts.Context = ctx
	return opts
}

func joinGasMined(ctx context.Context, t *testing.T, client *ethclient.Client,
	tx *types.Transaction, status uint64) *types.Receipt {
	t.Helper()
	receipt, err := bind.WaitMined(ctx, client, tx)
	require.NoError(t, err)
	require.Equal(t, status, receipt.Status)
	return receipt
}
