// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"fmt"
	"math/big"
	"regexp"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

func (s *ForeclosePrtSuite) TestForeclosePrtLateRegistrationPreservesMismatchEvidence() {
	s.foreclosedEvidenceAfterRegistration(false)
}

func (s *ForeclosePrtSuite) TestForeclosePrtLateRegistrationPreservesMatchingEvidenceAtSameBlockSeal() {
	s.foreclosedEvidenceAfterRegistration(true)
}

// The two logical instances run sequentially. Removal deletes the first
// application's rows, and a stopped node releases its runtime before the new
// registration. Only the independently captured reference evidence is retained.
func (s *ForeclosePrtSuite) foreclosedEvidenceAfterRegistration(matching bool) {
	if !isNodeSelfManaged() {
		s.T().Skip("fresh-registration evidence requires a test-managed node")
	}
	t, r := s.T(), s.Require()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	startReaderNode(ctx, t)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		r.NoError(anvilRPC(cleanupCtx, "evm_setAutomine", true))
		r.NoError(anvilRPC(cleanupCtx, "evm_setIntervalMining", 1))
	})
	setAnvilIntervalMining(ctx, t, 0)
	setAnvilAutomine(ctx, t, true)

	dsn, err := config.GetDatabaseConnection()
	r.NoError(err)
	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	r.NoError(err)
	defer repo.Close()
	const guardianIndex = 1
	withdrawal, _ := withdrawalConfigForGuardian(t, guardianIndex)
	dapp := envOrDefault("CARTESI_TEST_DAPP_PATH", "applications/echo-dapp")
	nameA, nameB := uniqueAppName("foreclose-evidence-a"), uniqueAppName("foreclose-evidence-b")
	s.appName = nameA
	address, err := deployApplication(ctx, nameA, dapp,
		prtFlag, "--salt", uniqueSalt(), "--withdrawal-config", withdrawal,
		claimStagingPeriodFlag, "5")
	r.NoError(err)
	const payload = "foreclosed sealed epoch evidence"
	inputIndex, _, _ := sendInputThroughRelay(ctx, t, common.HexToAddress(address), payload)
	r.Zero(inputIndex)
	input, err := waitForInputProcessed(ctx, t, nameA, inputIndex)
	r.NoError(err)
	r.Equal(model.InputCompletionStatus_Accepted, input.Status)
	r.Equal(uint64(1), input.EpochIndex)

	// Epoch zero is the real empty deployment seal. An external actor settles
	// its true result so the input-carrying epoch is sealed on chain.
	_, err = waitForEpochStatus(ctx, t, nameA, 0, model.EpochStatus_ClaimComputed)
	r.NoError(err)
	empty, err := repo.GetEpoch(ctx, nameA, 0)
	r.NoError(err)
	r.NotNil(empty)
	r.Equal(empty.InputIndexLowerBound, empty.InputIndexUpperBound)
	actor := transactorForMnemonicIndex(ctx, t, s.ethClient, passiveActorOneIndex)
	r.NoError(anvilSetBalance(ctx, actor.From.Hex(), "0x3635c9adc5dea00000"))
	joinForeclosureEvidence(ctx, t, s.ethClient, actor, empty, true)
	_, err = mineForTournamentTimeout(ctx, s.ethClient, *empty.TournamentAddress)
	r.NoError(err)
	app, err := repo.GetApplication(ctx, nameA)
	r.NoError(err)
	r.NotNil(app)
	consensus, err := idaveconsensus.NewIDaveConsensus(app.IConsensusAddress, s.ethClient)
	r.NoError(err)
	sealedAt := acceptForeclosureEvidencePrefix(ctx, t, s.ethClient, consensus, actor, empty, app.IApplicationAddress, matching)
	_, err = waitForEpochStatus(ctx, t, nameA, input.EpochIndex, model.EpochStatus_ClaimComputed)
	r.NoError(err)
	target, err := repo.GetEpoch(ctx, nameA, input.EpochIndex)
	r.NoError(err)
	r.NotNil(target)
	r.Equal(sealedAt, target.LastBlock, "the local boundary must match the actual sealing receipt")
	r.True(target.HasCompleteStateProof())
	r.NotNil(target.Commitment)
	r.NotEmpty(target.CommitmentProof)

	root, winner := joinForeclosureEvidence(ctx, t, s.ethClient, actor, target, matching)
	if !matching {
		cause := fmt.Sprintf("Epoch %d has inconsistent machine hash between off-chain (%s) and on-chain (%s)",
			target.Index, target.MachineHash, winner)
		descriptor, descriptorErr := root.TournamentDescriptor(&bind.CallOpts{Context: ctx})
		r.NoError(descriptorErr)
		closeBlock := descriptor.StartInstant + descriptor.Allowance
		r.GreaterOrEqual(closeBlock, descriptor.StartInstant)
		expected := make([]ExpectedLog, 1, 11)
		expected[0] = anvilBlockOutOfRangeAllowlist
		for _, name := range []string{nameA, nameB} {
			for _, source := range []string{prtStoredClockSource, prtPinnedClockSource, prtCommittedClockSource} {
				report := prtProtectionStopLog(name, app.IApplicationAddress, target.Index,
					*target.TournamentAddress, closeBlock, cause, source)
				report.Required = false
				expected = append(expected, report)
			}
			identity := regexp.QuoteMeta(name)
			expected = append(expected, ExpectedLog{
				Pattern: regexp.MustCompile("marking application as diverged.*application=" + identity + ".*" + regexp.QuoteMeta(cause)),
				Level:   LevelError, Required: true, Reason: "this target's external winner has the fixture's false final state",
			}, ExpectedLog{
				Pattern: regexp.MustCompile("Tick.*service=prt.*observing PRT application " + identity + ": " + regexp.QuoteMeta(cause)),
				Level:   LevelError, Reason: "the PRT tick can return the same committed target mismatch",
			})
		}
		s.SetExpectedLogs(t, expected...)
	}
	_, err = mineForTournamentTimeout(ctx, s.ethClient, *target.TournamentAddress)
	r.NoError(err)
	if !matching {
		r.NoError(waitForApplicationStatus(ctx, t, nameA, model.ApplicationStatus_Diverged.String()))
		r.NoError(guardianForeclose(ctx, nameA, guardianIndex))
	}
	r.NoError(waitForApplicationForeclosed(ctx, t, nameA))
	// CLI removal uses the parent's policy; make the actual foreclosure
	// visible there without changing that policy in the parent environment.
	policy, err := config.GetBlockchainDefaultBlock()
	r.NoError(err)
	app, err = repo.GetApplication(ctx, nameA)
	r.NoError(err)
	if matching {
		r.Equal(app.ForecloseBlock, target.LastBlock, "the real seal and foreclosure must share the target boundary")
	}
	r.NoError(pollUntil(ctx, time.Second, func() (bool, error) {
		if _, finalityErr := advanceFixtureFinality(ctx, s.ethClient, policy); finalityErr != nil {
			return false, finalityErr
		}
		return confirmedBlockAtLeast(ctx, s.ethClient.HeaderByNumber, policy, app.ForecloseBlock)
	}))
	head, err := s.ethClient.BlockNumber(ctx)
	r.NoError(err)
	standing, err := root.TournamentStanding(&bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(head)})
	r.NoError(err)
	r.Equal(uint8(2), standing.Standing, "ITournament.Standing.ROOT_WINNER")
	r.Positive(standing.FinishedAt)
	r.LessOrEqual(standing.FinishedAt, head)
	r.Equal(winner, common.Hash(standing.FinalState))

	wantStatus := model.EpochStatus_ClaimForeclosed
	if !matching {
		wantStatus = model.EpochStatus_ClaimComputed
	}
	waitForeclosureEvidence(ctx, t, repo, nameA, target.Index, head, wantStatus, matching)
	prefixA, err := waitForEpochStatus(ctx, t, nameA, 0, model.EpochStatus_ClaimAccepted)
	r.NoError(err)
	r.NotNil(prefixA)
	r.NotNil(prefixA.ClaimTransactionHash, "the accepted empty prefix must retain its observed acceptance")
	r.Equal(empty.MachineHash, prefixA.MachineHash)
	r.Equal(empty.TxBufferDataBlock, prefixA.TxBufferDataBlock)
	a := captureForeclosureEvidence(ctx, t, repo, nameA, target.Index, payload)

	stopSharedNode(t)
	r.NoError(disableApplication(ctx, nameA))
	r.NoError(removeApplication(ctx, nameA))
	removed, err := repo.GetApplication(ctx, nameA)
	r.NoError(err)
	r.Nil(removed, "the original application and its cascading evidence rows must be removed")
	s.appName = ""
	r.NoError(registerPrtApplication(ctx, nameB, address, dapp))
	s.appName = nameB
	fresh, err := repo.GetApplication(ctx, nameB)
	r.NoError(err)
	r.NotNil(fresh)
	r.NotEqual(a.app.ID, fresh.ID)
	freshEpoch, err := repo.GetEpoch(ctx, nameB, target.Index)
	r.NoError(err)
	r.Nil(freshEpoch, "a stopped node has not ingested or computed the late registration")
	freshInput, err := repo.GetInput(ctx, nameB, inputIndex)
	r.NoError(err)
	r.Nil(freshInput, "the reference input must not be copied into the new application")
	startSharedNodeWithEnv(t, "CARTESI_FEATURE_CLAIM_SUBMISSION_ENABLED=false", "CARTESI_BLOCKCHAIN_DEFAULT_BLOCK=latest")
	waitForeclosureEvidence(ctx, t, repo, nameB, target.Index, head, wantStatus, matching)
	prefixB, err := waitForEpochStatus(ctx, t, nameB, 0, model.EpochStatus_ClaimAccepted)
	r.NoError(err)
	r.NotNil(prefixB)
	r.Equal(prefixA.ClaimTransactionHash, prefixB.ClaimTransactionHash)
	r.Equal(empty.MachineHash, prefixB.MachineHash)
	r.Equal(empty.TxBufferDataBlock, prefixB.TxBufferDataBlock)
	b := captureForeclosureEvidence(ctx, t, repo, nameB, target.Index, payload)
	compareForeclosureEvidence(t, a, b)
	for _, evidence := range []foreclosureEvidence{a, b} {
		r.Equal(model.TournamentStandingRootWinner, evidence.root.Snapshot.Standing)
		r.Equal(standing.FinishedAt, evidence.root.Snapshot.FinishedAtBlock)
		r.Equal(common.Hash(standing.Candidate), *evidence.root.Snapshot.WinnerCommitment)
		r.Equal(winner, *evidence.root.Snapshot.FinalStateHash)
		r.Equal(matching, *evidence.epoch.MachineHash == winner)
	}
	sealed, err := consensus.GetCurrentSealedEpoch(&bind.CallOpts{Context: ctx, BlockNumber: new(big.Int).SetUint64(head)})
	r.NoError(err)
	r.Equal(target.Index, sealed.EpochNumber.Uint64(), "the target must remain unaccepted on chain")
	r.Equal(*target.TournamentAddress, sealed.Tournament)
	r.Equal(target.InputIndexLowerBound, sealed.InputIndexLowerBound.Uint64())
	r.Equal(target.InputIndexUpperBound, sealed.InputIndexUpperBound.Uint64())
	r.False(sealed.IsTournamentResultStaged)
	after, err := s.ethClient.BlockNumber(ctx)
	r.NoError(err)
	r.Equal(head, after, "fresh reconstruction and diagnosis must send no chain transactions")
}

// The contract checks membership only. The false-final tree represents an
// external invalid execution claim; it does not fabricate a valid VM step.
func joinForeclosureEvidence(ctx context.Context, t testing.TB, client *ethclient.Client,
	actor *bind.TransactOpts, epoch *model.Epoch, matching bool,
) (*itournament.ITournament, common.Hash) {
	t.Helper()
	r := require.New(t)
	r.NotNil(epoch.TournamentAddress)
	r.NotNil(epoch.MachineHash)
	r.NotNil(epoch.Commitment)
	root, err := itournament.NewITournament(*epoch.TournamentAddress, client)
	r.NoError(err)
	descriptor, err := root.TournamentDescriptor(&bind.CallOpts{Context: ctx})
	r.NoError(err)
	r.Zero(descriptor.Level)
	r.Equal(descriptor.Height, uint64(len(epoch.CommitmentProof)))
	final := *epoch.MachineHash
	proof := make([][32]byte, len(epoch.CommitmentProof))
	for i, sibling := range epoch.CommitmentProof {
		proof[i] = sibling
	}
	left, right := epoch.CommitmentProof[len(proof)-1], final
	for _, sibling := range epoch.CommitmentProof[:len(proof)-1] {
		right = crypto.Keccak256Hash(sibling[:], right[:])
	}
	r.Equal(*epoch.Commitment, crypto.Keccak256Hash(left[:], right[:]), "the final-leaf path must open the exact local root")
	if !matching {
		final = crypto.Keccak256Hash(final[:], []byte("external false final state"))
		tree, treeErr := newSparseDisputeCommitment(descriptor.Height, *epoch.MachineHash, final)
		r.NoError(treeErr)
		left, right, err = tree.rightmostChildren(descriptor.Height)
		r.NoError(err)
		_, proof, err = tree.proof(tree.leafCount() - 1)
		r.NoError(err)
		r.NotEqual(*epoch.MachineHash, final)
	}
	bond, err := root.BondValue(&bind.CallOpts{Context: ctx})
	r.NoError(err)
	opts := *actor
	opts.Value = bond
	tx, err := root.JoinTournament(&opts, final, proof, left, right)
	r.NoError(err)
	r.Equal(types.ReceiptStatusSuccessful, waitReceipt(ctx, t, client, tx).Status)
	return root, final
}

func acceptForeclosureEvidencePrefix(ctx context.Context, t testing.TB, client *ethclient.Client,
	consensus *idaveconsensus.IDaveConsensus, actor *bind.TransactOpts, epoch *model.Epoch,
	application common.Address, sameBlock bool,
) uint64 {
	t.Helper()
	r := require.New(t)
	state, err := epoch.StateProof()
	r.NoError(err)
	proof := idaveconsensus.MachineValidityProof{
		IflagsYProof:    idaveconsensus.LeafProof{DataBlock: state.IflagsYDataBlock, Siblings: state.IflagsYProof},
		HtifTohostProof: idaveconsensus.LeafProof{DataBlock: state.HtifTohostDataBlock, Siblings: state.HtifTohostProof},
		TxBufferProof:   idaveconsensus.LeafProof{DataBlock: state.TxBufferDataBlock, Siblings: state.TxBufferProof},
	}
	tx, err := consensus.StageTournamentResult(actor, new(big.Int).SetUint64(epoch.Index), proof)
	r.NoError(err)
	r.Equal(types.ReceiptStatusSuccessful, waitReceipt(ctx, t, client, tx).Status)
	period, err := consensus.GetClaimStagingPeriod(&bind.CallOpts{Context: ctx})
	r.NoError(err)
	r.True(period.IsUint64())
	r.LessOrEqual(period.Uint64(), uint64(maxBlocksToMine))
	r.NoError(anvilMine(ctx, int(period.Uint64()))) //nolint:gosec // Bounded by maxBlocksToMine.
	if sameBlock {
		return sealAndForecloseInOneBlock(ctx, t, client, consensus, application, epoch.Index)
	}
	tx, err = consensus.AcceptStagedTournamentResult(actor, new(big.Int).SetUint64(epoch.Index))
	r.NoError(err)
	receipt := waitReceipt(ctx, t, client, tx)
	r.Equal(types.ReceiptStatusSuccessful, receipt.Status)
	return receipt.BlockNumber.Uint64()
}

// Both permissionless acceptance and guardian foreclosure use the guardian's
// consecutive nonces. The receipts prove their actual EVM order in one block.
func sealAndForecloseInOneBlock(ctx context.Context, t testing.TB, client *ethclient.Client,
	consensus *idaveconsensus.IDaveConsensus, application common.Address, prefix uint64,
) uint64 {
	t.Helper()
	r := require.New(t)
	guardian := transactorForMnemonicIndex(ctx, t, client, 1)
	nonce, err := client.PendingNonceAt(ctx, guardian.From)
	r.NoError(err)
	contract, err := iapplication.NewIApplication(application, client)
	r.NoError(err)
	setAnvilAutomine(ctx, t, false)
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		r.NoError(anvilRPC(cleanupCtx, "evm_setAutomine", true))
	}()
	acceptOpts := *guardian
	acceptOpts.Nonce = new(big.Int).SetUint64(nonce)
	acceptOpts.GasLimit = 3_000_000
	accept, err := consensus.AcceptStagedTournamentResult(&acceptOpts, new(big.Int).SetUint64(prefix))
	r.NoError(err)
	forecloseOpts := *guardian
	forecloseOpts.Nonce = new(big.Int).SetUint64(nonce + 1)
	forecloseOpts.GasLimit = 300_000
	foreclose, err := contract.Foreclose(&forecloseOpts)
	r.NoError(err)
	r.NoError(anvilMine(ctx, 1))
	accepted := waitReceipt(ctx, t, client, accept)
	foreclosed := waitReceipt(ctx, t, client, foreclose)
	r.Equal(types.ReceiptStatusSuccessful, accepted.Status)
	r.Equal(types.ReceiptStatusSuccessful, foreclosed.Status)
	r.Equal(accepted.BlockNumber, foreclosed.BlockNumber)
	r.Equal(accepted.BlockHash, foreclosed.BlockHash)
	r.Less(accepted.TransactionIndex, foreclosed.TransactionIndex)
	return foreclosed.BlockNumber.Uint64()
}

func waitForeclosureEvidence(ctx context.Context, t testing.TB, repo repository.Repository,
	name string, index, head uint64, status model.EpochStatus, matching bool,
) {
	t.Helper()
	require.NoError(t, pollUntil(ctx, time.Second, func() (bool, error) {
		app, err := repo.GetApplication(ctx, name)
		if err != nil || app == nil || app.ForecloseBlock == 0 || !app.ForeclosureScanCaughtUp() {
			return false, err
		}
		wantHealth := model.ApplicationStatus_Diverged
		if matching {
			wantHealth = model.ApplicationStatus_OK
		}
		if app.Status != wantHealth {
			return false, nil
		}
		epoch, err := repo.GetEpoch(ctx, name, index)
		if err != nil || epoch == nil || epoch.Status != status || epoch.TournamentAddress == nil {
			return false, err
		}
		root, err := repo.GetTournament(ctx, name, epoch.TournamentAddress.Hex())
		return root != nil && root.Snapshot.FinishedAtBlock > 0 &&
			root.Snapshot.FinishedAtBlock <= root.Snapshot.AsOfBlock && root.Snapshot.AsOfBlock <= head, err
	}), "reconstruct the target and its observed diagnosis at the pinned head")
}

type foreclosureEvidence struct {
	app     *model.Application
	epoch   *model.Epoch
	input   *model.Input
	outputs []*model.Output
	reports []*model.Report
	hashes  []model.EpochStateHash
	root    *model.Tournament
	problem *model.ApplicationProblem
}

func captureForeclosureEvidence(ctx context.Context, t testing.TB, repo repository.Repository,
	name string, index uint64, payload string,
) foreclosureEvidence {
	t.Helper()
	r := require.New(t)
	var evidence foreclosureEvidence
	var err error
	evidence.app, err = repo.GetApplication(ctx, name)
	r.NoError(err)
	evidence.epoch, err = repo.GetEpoch(ctx, name, index)
	r.NoError(err)
	completion, err := repo.GetEpochCompletionState(ctx, name, index)
	r.NoError(err)
	r.NotNil(completion)
	evidence.problem = completion.Problem
	evidence.input, err = repo.GetInput(ctx, name, 0)
	r.NoError(err)
	r.NotNil(evidence.input)
	r.Equal(model.InputCompletionStatus_Accepted, evidence.input.Status)
	r.True(evidence.epoch.HasCompleteStateProof())
	r.NotNil(evidence.epoch.CalculatedOutputsRoot)
	r.NotNil(evidence.epoch.Commitment)
	r.NotEmpty(evidence.epoch.CommitmentProof)
	evidence.outputs, _, err = repo.ListOutputs(ctx, name, repository.OutputFilter{EpochIndex: &index}, repository.Pagination{}, false)
	r.NoError(err)
	r.Len(evidence.outputs, echoOutputsPerInput)
	for _, output := range evidence.outputs {
		r.NotNil(output.Hash)
		r.Equal(crypto.Keccak256Hash(output.RawData), *output.Hash)
		r.NotEmpty(output.OutputHashesSiblings)
	}
	evidence.reports, _, err = repo.ListReports(ctx, name, repository.ReportFilter{EpochIndex: &index}, repository.Pagination{}, false)
	r.NoError(err)
	r.Len(evidence.reports, echoReportsPerInput)
	r.Equal([]byte(payload), evidence.reports[0].RawData, "the guest must echo the independently supplied payload")
	var after *uint64
	for {
		page, readErr := repo.ListEpochStateHashes(ctx, evidence.app.ID, index, after, repository.MaxStateHashPageSize)
		r.NoError(readErr)
		if len(page) == 0 {
			break
		}
		evidence.hashes = append(evidence.hashes, page...)
		last := page[len(page)-1].Index
		after = &last
	}
	r.NotEmpty(evidence.hashes)
	evidence.root, err = repo.GetTournament(ctx, name, evidence.epoch.TournamentAddress.Hex())
	r.NoError(err)
	r.NotNil(evidence.root)
	return evidence
}

func compareForeclosureEvidence(t testing.TB, a, b foreclosureEvidence) {
	t.Helper()
	r := require.New(t)
	compareReplayedInput(t, r, a.input, b.input, 0)
	r.Equal(a.input.ExceptionData, b.input.ExceptionData)
	r.Equal(a.input.MachineHash, b.input.MachineHash)
	r.Equal(a.input.TxBufferDataBlock, b.input.TxBufferDataBlock)
	compareReplayedEpoch(t, r, a.epoch, b.epoch, a.epoch.Index)
	r.Equal(a.epoch.CalculatedOutputsRoot, b.epoch.CalculatedOutputsRoot)
	proofA, err := a.epoch.StateProof()
	r.NoError(err)
	proofB, err := b.epoch.StateProof()
	r.NoError(err)
	r.Equal(proofA, proofB, "all three canonical state leaves and their sibling paths must survive reconstruction")
	r.Equal(a.epoch.Commitment, b.epoch.Commitment)
	r.Equal(a.epoch.CommitmentProof, b.epoch.CommitmentProof)
	r.Equal(a.hashes, b.hashes, "commitment input order, sampled hashes and repetition counts must agree")
	r.Equal(a.app.ForecloseBlock, b.app.ForecloseBlock)
	r.Equal(a.app.Status == model.ApplicationStatus_Diverged, b.app.Status == model.ApplicationStatus_Diverged)
	r.Equal(a.problem, b.problem)
	r.Len(b.outputs, len(a.outputs))
	for i, output := range a.outputs {
		replayed := b.outputs[i]
		r.Equal(output.Index, replayed.Index)
		r.Equal(output.InputIndex, replayed.InputIndex)
		r.Equal(output.EpochIndex, replayed.EpochIndex)
		r.Equal(output.RawData, replayed.RawData)
		r.Equal(output.Hash, replayed.Hash)
		r.Equal(output.OutputHashesSiblings, replayed.OutputHashesSiblings)
	}
	r.Len(b.reports, len(a.reports))
	for i, report := range a.reports {
		r.Equal(report.Index, b.reports[i].Index)
		r.Equal(report.InputIndex, b.reports[i].InputIndex)
		r.Equal(report.EpochIndex, b.reports[i].EpochIndex)
		r.Equal(report.RawData, b.reports[i].RawData)
	}
}
