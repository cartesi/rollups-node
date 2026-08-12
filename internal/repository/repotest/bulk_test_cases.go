// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type BulkOperationsSuite struct {
	BaseSuite
}

func NewBulkOperationsSuite(factory RepositoryFactory) *BulkOperationsSuite {
	return &BulkOperationsSuite{BaseSuite: BaseSuite{factory: factory}}
}

func (s *BulkOperationsSuite) TestStoreAdvanceResult() {
	s.Run("RejectsNilResult", func() {
		err := s.Repo.StoreAdvanceResult(s.Ctx, 0, nil)
		s.Require().EqualError(err, "advance result must not be nil")
	})

	s.Run("AcceptedInput", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		machineHash := crypto.Keccak256Hash([]byte("machine"))
		outputsHash := crypto.Keccak256Hash([]byte("outputs"))

		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			Outputs:    [][]byte{[]byte("output1"), []byte("output2")},
			Reports:    [][]byte{[]byte("report1")},
			OutputsProof: OutputsProof{
				OutputsHash: outputsHash,
				MachineHash: machineHash,
			},
		}

		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().NoError(err)

		// Verify the input was updated
		input, err := s.Repo.GetInput(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(InputCompletionStatus_Accepted, input.Status)
		s.Require().NotNil(input.MachineHash)
		s.Equal(machineHash, *input.MachineHash)

		// Verify outputs were created
		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(outputs, 2)
		s.Equal(uint64(2), total)

		// Verify reports were created
		reports, total, err := s.Repo.ListReports(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.ReportFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(reports, 1)
		s.Equal(uint64(1), total)
	})

	s.Run("RejectedInput", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		machineHash := crypto.Keccak256Hash([]byte("machine-rejected"))

		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Rejected,
			OutputsProof: OutputsProof{
				MachineHash: machineHash,
			},
		}

		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().NoError(err)

		input, err := s.Repo.GetInput(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(InputCompletionStatus_Rejected, input.Status)
	})

	for _, status := range []InputCompletionStatus{
		InputCompletionStatus_Exception,
		InputCompletionStatus_MachineHalted,
	} {
		s.Run("CompletedStatus/"+status.String(), func() {
			seed := Seed(s.Ctx, s.T(), s.Repo)
			var exceptionData []byte
			if status == InputCompletionStatus_Exception {
				exceptionData = []byte{0xff, 0x00, 0x80}
			}
			result := &AdvanceResult{
				EpochIndex:    0,
				InputIndex:    0,
				Status:        status,
				ExceptionData: exceptionData,
				OutputsProof: OutputsProof{
					MachineHash: UniqueHash(),
				},
			}

			err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
			s.Require().NoError(err)
			input, err := s.Repo.GetInput(s.Ctx, seed.App.IApplicationAddress.String(), 0)
			s.Require().NoError(err)
			s.Equal(status, input.Status)
			s.Equal(exceptionData, input.ExceptionData)
		})
	}

	for _, test := range []struct {
		name          string
		status        InputCompletionStatus
		exceptionData []byte
	}{
		{"ExceptionWithoutData", InputCompletionStatus_Exception, nil},
		{"AcceptedWithExceptionData", InputCompletionStatus_Accepted, []byte("unexpected")},
	} {
		s.Run("Rejects"+test.name, func() {
			seed := Seed(s.Ctx, s.T(), s.Repo)
			err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, &AdvanceResult{
				EpochIndex:    0,
				InputIndex:    0,
				Status:        test.status,
				ExceptionData: test.exceptionData,
				OutputsProof: OutputsProof{
					MachineHash: UniqueHash(),
					OutputsHash: UniqueHash(),
				},
			})
			s.Require().ErrorContains(err, "exception data")
		})
	}

	invalidStatuses := []InputCompletionStatus{
		InputCompletionStatus_None,
		"OUTPUTS_LIMIT_EXCEEDED",
		"REPORTS_LIMIT_EXCEEDED",
		"CYCLE_LIMIT_EXCEEDED",
		"TIME_LIMIT_EXCEEDED",
		"PAYLOAD_LENGTH_LIMIT_EXCEEDED",
		"INVALID",
	}
	for _, status := range invalidStatuses {
		s.Run("RejectsNoncompletedStatus/"+status.String(), func() {
			seed := Seed(s.Ctx, s.T(), s.Repo)
			result := &AdvanceResult{
				EpochIndex: 0,
				InputIndex: 0,
				Status:     status,
				Outputs:    [][]byte{[]byte("must-not-be-stored")},
				OutputsProof: OutputsProof{
					MachineHash: UniqueHash(),
				},
			}

			err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
			s.Require().ErrorContains(err, "noncompleted status")
			input, err := s.Repo.GetInput(s.Ctx, seed.App.IApplicationAddress.String(), 0)
			s.Require().NoError(err)
			s.Equal(InputCompletionStatus_None, input.Status)
		})
	}

	s.Run("WithNoOutputsOrReports", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		machineHash := crypto.Keccak256Hash([]byte("machine-empty"))

		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			OutputsProof: OutputsProof{
				MachineHash: machineHash,
			},
		}

		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().NoError(err)

		input, err := s.Repo.GetInput(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(InputCompletionStatus_Accepted, input.Status)
	})

	// Verify that a failure mid-transaction rolls back all prior changes.
	// We trigger failure by providing a non-existent epoch index, causing the
	// epoch outputs proof update to fail after outputs and input are written.
	s.Run("RollbackOnPartialFailure", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		result := &AdvanceResult{
			EpochIndex: 99, // non-existent epoch
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			Outputs:    [][]byte{[]byte("should-be-rolled-back")},
			Reports:    [][]byte{[]byte("should-be-rolled-back")},
			OutputsProof: OutputsProof{
				OutputsHash: UniqueHash(),
				MachineHash: UniqueHash(),
			},
		}

		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().Error(err)

		// Input status should remain unchanged (NONE)
		input, err := s.Repo.GetInput(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(InputCompletionStatus_None, input.Status)

		// No outputs should have been persisted
		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(outputs)
		s.Equal(uint64(0), total)

		// No reports should have been persisted
		reports, total, err := s.Repo.ListReports(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.ReportFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(reports)
		s.Equal(uint64(0), total)

		// ProcessedInputs should remain at 0
		count, err := s.Repo.GetProcessedInputCount(
			s.Ctx, seed.App.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(0), count)
	})

	s.Run("DaveConsensusWithStateHashes", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)
		machineHash := crypto.Keccak256Hash([]byte("dave-machine"))
		outputsHash := crypto.Keccak256Hash([]byte("dave-outputs"))

		hash1 := [32]byte(crypto.Keccak256Hash([]byte("state-1")))
		hash2 := [32]byte(crypto.Keccak256Hash([]byte("state-2")))
		hash3 := [32]byte(crypto.Keccak256Hash([]byte("state-3")))

		result := &AdvanceResult{
			EpochIndex:          0,
			InputIndex:          0,
			Status:              InputCompletionStatus_Accepted,
			Outputs:             [][]byte{[]byte("dave-output")},
			PeriodicStateHashes: [][32]byte{hash1, hash2, hash3},
			PaddingRepetitions:  42,
			IsDaveConsensus:     true,
			OutputsProof: OutputsProof{
				OutputsHash: outputsHash,
				MachineHash: machineHash,
			},
		}

		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().NoError(err)

		// Verify state hashes were created (3 intermediate + 1 final = 4)
		epochIdx := uint64(0)
		stateHashes, total, err := s.Repo.ListStateHashes(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.StateHashFilter{EpochIndex: &epochIdx},
			repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(stateHashes, 4)
		s.Equal(uint64(4), total)

		// Verify intermediate hashes have Repetitions=1
		s.Equal(common.Hash(hash1), stateHashes[0].MachineHash)
		s.Equal(uint64(1), stateHashes[0].Repetitions)
		s.Equal(common.Hash(hash2), stateHashes[1].MachineHash)
		s.Equal(uint64(1), stateHashes[1].Repetitions)
		s.Equal(common.Hash(hash3), stateHashes[2].MachineHash)
		s.Equal(uint64(1), stateHashes[2].Repetitions)

		// Verify final hash has PaddingRepetitions as Repetitions
		s.Equal(machineHash, stateHashes[3].MachineHash)
		s.Equal(uint64(42), stateHashes[3].Repetitions)

		// Verify outputs were also created
		outputs, _, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(outputs, 1)

		// Verify input was updated
		input, err := s.Repo.GetInput(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(InputCompletionStatus_Accepted, input.Status)
	})
}

func (s *BulkOperationsSuite) TestStoreAdvanceResultRollback() {
	// Trigger rollback by referencing a non-existent input index (the input
	// doesn't exist in the DB, so updateInput will fail with sql.ErrNoRows).
	// This tests that outputs inserted earlier in the same transaction are
	// rolled back when a subsequent step fails.
	s.Run("RollbackOnInputUpdateFailure", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 999, // non-existent input index
			Status:     InputCompletionStatus_Accepted,
			Outputs:    [][]byte{[]byte("should-be-rolled-back")},
			Reports:    [][]byte{[]byte("should-be-rolled-back")},
			OutputsProof: OutputsProof{
				OutputsHash: UniqueHash(),
				MachineHash: UniqueHash(),
			},
		}

		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().Error(err)

		// Verify no outputs were persisted (rolled back)
		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(outputs)
		s.Equal(uint64(0), total)

		// Verify no reports were persisted (rolled back)
		reports, total, err := s.Repo.ListReports(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.ReportFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(reports)
		s.Equal(uint64(0), total)

		// Verify the original input is untouched
		input, err := s.Repo.GetInput(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(InputCompletionStatus_None, input.Status)

		// Verify ProcessedInputs remains 0
		count, err := s.Repo.GetProcessedInputCount(
			s.Ctx, seed.App.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(0), count)
	})

	// Trigger rollback by providing a valid input index but a non-existent
	// app ID, so updateApp fails. This verifies that outputs, reports, and
	// the input status update are all rolled back.
	s.Run("RollbackOnAppUpdateFailure", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			Outputs:    [][]byte{[]byte("should-be-rolled-back")},
			Reports:    [][]byte{[]byte("should-be-rolled-back")},
			OutputsProof: OutputsProof{
				OutputsHash: UniqueHash(),
				MachineHash: UniqueHash(),
			},
		}

		// Use a non-existent app ID -- updateApp will fail
		// because no application row matches.
		err := s.Repo.StoreAdvanceResult(s.Ctx, 999999, result)
		s.Require().Error(err)

		// Verify the original input is untouched
		input, err := s.Repo.GetInput(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(InputCompletionStatus_None, input.Status)

		// Verify no outputs were persisted
		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(outputs)
		s.Equal(uint64(0), total)
	})

	// Verify that when Dave consensus state hash insertion fails (due to
	// a bad epoch index), all prior work (outputs, reports) is rolled back.
	s.Run("DaveConsensusRollbackOnStateHashFailure", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		result := &AdvanceResult{
			EpochIndex:          99, // non-existent epoch
			InputIndex:          0,
			Status:              InputCompletionStatus_Accepted,
			Outputs:             [][]byte{[]byte("should-be-rolled-back")},
			PeriodicStateHashes: [][32]byte{{1}, {2}},
			PaddingRepetitions:  10,
			IsDaveConsensus:     true,
			OutputsProof: OutputsProof{
				OutputsHash: UniqueHash(),
				MachineHash: UniqueHash(),
			},
		}

		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().Error(err)

		// Verify no outputs were persisted (rolled back)
		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(outputs)
		s.Equal(uint64(0), total)

		// Verify the input remains unprocessed
		input, err := s.Repo.GetInput(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(InputCompletionStatus_None, input.Status)
	})
}

func (s *BulkOperationsSuite) TestStoreClaimAndProofs() {
	s.Run("StoresClaimAndOutputProofs", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// Advance epoch to INPUTS_PROCESSED so StoreClaimAndProofs can set CLAIM_COMPUTED
		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			seed.App.IApplicationAddress.String(), seed.Epoch, EpochStatus_InputsProcessed)

		// First store an advance result to create outputs
		machineHash := crypto.Keccak256Hash([]byte("machine"))
		outputData := []byte("output-for-claim")
		outputsHash := crypto.Keccak256Hash([]byte("outputs-merkle"))

		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			Outputs:    [][]byte{outputData},
			OutputsProof: OutputsProof{
				OutputsHash: outputsHash,
				MachineHash: machineHash,
			},
		}
		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().NoError(err)

		// Now store claim and proofs using Commitment/CommitmentProof fields
		commitmentHash := crypto.Keccak256Hash([]byte("commitment"))
		seed.Epoch.Commitment = &commitmentHash
		seed.Epoch.CommitmentProof = []common.Hash{UniqueHash(), UniqueHash()}

		outputHash := crypto.Keccak256Hash(outputData)
		proof := []common.Hash{UniqueHash(), UniqueHash()}
		out := &Output{
			InputEpochApplicationID: seed.App.ID,
			InputIndex:              0,
			Index:                   0,
			RawData:                 outputData,
			Hash:                    &outputHash,
			OutputHashesSiblings:    proof,
		}

		err = s.Repo.StoreClaimAndProofs(s.Ctx, seed.Epoch, []*Output{out})
		s.Require().NoError(err)

		gotEpoch, err := s.Repo.GetEpoch(s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_ClaimComputed, gotEpoch.Status)
		s.Require().NotNil(gotEpoch.Commitment)
		s.Equal(commitmentHash, *gotEpoch.Commitment)
	})
}

func (s *BulkOperationsSuite) TestStoreTournamentEvents() {
	// setupTournamentWithMatch creates a PRT app with a tournament, two
	// commitments, and one match, all stored via StoreTournamentEvents.
	type tournamentSetup struct {
		app       *Application
		tournAddr common.Address
		match     *Match
	}
	setupTournamentWithMatch := func() *tournamentSetup {
		s.T().Helper()
		app := NewApplicationBuilder().
			WithConsensus(Consensus_PRT).
			Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).WithBlocks(0, 9).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		tournAddr := UniqueAddress()
		tournament := NewTournamentBuilder(app.ID).
			WithEpochIndex(0).WithAddress(tournAddr).Build()
		err = s.Repo.CreateTournament(
			s.Ctx, app.IApplicationAddress.String(), tournament)
		s.Require().NoError(err)

		commitment1 := NewCommitmentBuilder(app.ID).
			WithEpochIndex(0).WithTournamentAddress(tournAddr).Build()
		commitment2 := NewCommitmentBuilder(app.ID).
			WithEpochIndex(0).WithTournamentAddress(tournAddr).Build()

		matchIDHash := UniqueHash()
		match := NewMatchBuilder(app.ID).
			WithEpochIndex(0).
			WithTournamentAddress(tournAddr).
			WithIDHash(matchIDHash).
			WithCommitmentOne(commitment1.Commitment).
			WithCommitmentTwo(commitment2.Commitment).
			Build()

		err = s.Repo.StoreTournamentEvents(
			s.Ctx, app.ID,
			[]*Commitment{commitment1, commitment2},
			[]*Match{match},
			nil, nil, 100)
		s.Require().NoError(err)

		return &tournamentSetup{app: app, tournAddr: tournAddr, match: match}
	}

	s.Run("StoresCommitmentsAndMatches", func() {
		ts := setupTournamentWithMatch()

		// Verify commitment was stored
		gotCommitment, err := s.Repo.GetCommitment(
			s.Ctx, ts.app.IApplicationAddress.String(),
			0, ts.tournAddr.String(), ts.match.CommitmentOne.Hex())
		s.Require().NoError(err)
		s.Equal(ts.match.CommitmentOne, gotCommitment.Commitment)

		// Verify match was stored
		gotMatch, err := s.Repo.GetMatch(
			s.Ctx, ts.app.IApplicationAddress.String(),
			0, ts.tournAddr.String(), ts.match.IDHash.Hex())
		s.Require().NoError(err)
		s.Equal(ts.match.IDHash, gotMatch.IDHash)
	})

	s.Run("StoresMatchAdvanced", func() {
		ts := setupTournamentWithMatch()

		// Now store a match advanced event for the existing match
		ma := NewMatchAdvancedBuilder(ts.app.ID).
			WithEpochIndex(0).
			WithTournamentAddress(ts.tournAddr).
			WithIDHash(ts.match.IDHash).
			Build()

		err := s.Repo.StoreTournamentEvents(
			s.Ctx, ts.app.ID,
			nil, nil,
			[]*MatchAdvanced{ma}, nil, 200)
		s.Require().NoError(err)

		// Verify match advanced was stored
		gotMA, err := s.Repo.GetMatchAdvanced(
			s.Ctx, ts.app.IApplicationAddress.String(),
			0, ts.tournAddr.String(), ts.match.IDHash.Hex(),
			hex.EncodeToString(ma.OtherParent[:]))
		s.Require().NoError(err)
		s.Require().NotNil(gotMA)
		s.Equal(ma.OtherParent, gotMA.OtherParent)
	})

	s.Run("UpdatesDeletedMatches", func() {
		ts := setupTournamentWithMatch()

		// Mark the match as deleted (winner decided)
		deletedMatch := &Match{
			EpochIndex:          0,
			TournamentAddress:   ts.tournAddr,
			IDHash:              ts.match.IDHash,
			Winner:              WinnerCommitment_ONE,
			DeletionReason:      MatchDeletionReason_TIMEOUT,
			DeletionBlockNumber: 200,
			DeletionTxHash:      UniqueHash(),
		}

		err := s.Repo.StoreTournamentEvents(
			s.Ctx, ts.app.ID,
			nil, nil, nil,
			[]*Match{deletedMatch}, 300)
		s.Require().NoError(err)

		// Verify the match was updated
		gotMatch, err := s.Repo.GetMatch(
			s.Ctx, ts.app.IApplicationAddress.String(),
			0, ts.tournAddr.String(), ts.match.IDHash.Hex())
		s.Require().NoError(err)
		s.Equal(WinnerCommitment_ONE, gotMatch.Winner)
		s.Equal(MatchDeletionReason_TIMEOUT, gotMatch.DeletionReason)
	})
}

func (s *BulkOperationsSuite) TestConcurrentStoreAdvanceResult() {
	// Verify that concurrent StoreAdvanceResult calls for different
	// applications succeed independently without corrupting data.
	s.Run("DifferentApplications", func() {
		seed1 := Seed(s.Ctx, s.T(), s.Repo)
		seed2 := Seed(s.Ctx, s.T(), s.Repo)

		var wg sync.WaitGroup
		errs := make([]error, 2)

		seeds := []*SeedResult{seed1, seed2}
		for i, seed := range seeds {
			wg.Add(1)
			go func() {
				defer wg.Done()
				result := &AdvanceResult{
					EpochIndex: 0,
					InputIndex: 0,
					Status:     InputCompletionStatus_Accepted,
					Outputs:    [][]byte{[]byte(fmt.Sprintf("output-%d", i))},
					Reports:    [][]byte{[]byte(fmt.Sprintf("report-%d", i))},
					OutputsProof: OutputsProof{
						OutputsHash: crypto.Keccak256Hash(
							[]byte(fmt.Sprintf("outputs-%d", i))),
						MachineHash: crypto.Keccak256Hash(
							[]byte(fmt.Sprintf("machine-%d", i))),
					},
				}
				errs[i] = s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
			}()
		}
		wg.Wait()

		s.Require().NoError(errs[0], "first app store should succeed")
		s.Require().NoError(errs[1], "second app store should succeed")

		// Verify first application
		input1, err := s.Repo.GetInput(
			s.Ctx, seed1.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(InputCompletionStatus_Accepted, input1.Status)

		count1, err := s.Repo.GetProcessedInputCount(
			s.Ctx, seed1.App.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(1), count1)

		outputs1, total1, err := s.Repo.ListOutputs(
			s.Ctx, seed1.App.IApplicationAddress.String(),
			repository.OutputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(outputs1, 1)
		s.Equal(uint64(1), total1)

		// Verify second application
		input2, err := s.Repo.GetInput(
			s.Ctx, seed2.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(InputCompletionStatus_Accepted, input2.Status)

		count2, err := s.Repo.GetProcessedInputCount(
			s.Ctx, seed2.App.IApplicationAddress.String())
		s.Require().NoError(err)
		s.Equal(uint64(1), count2)

		outputs2, total2, err := s.Repo.ListOutputs(
			s.Ctx, seed2.App.IApplicationAddress.String(),
			repository.OutputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Len(outputs2, 1)
		s.Equal(uint64(1), total2)
	})

	// Verify that concurrent StoreAdvanceResult calls for the same input
	// do not corrupt data. At least one goroutine must succeed and the
	// final state must be consistent.
	s.Run("SameInputAtomicity", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		const numGoroutines = 5
		var wg sync.WaitGroup
		errs := make([]error, numGoroutines)

		for i := range numGoroutines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				result := &AdvanceResult{
					EpochIndex: 0,
					InputIndex: 0,
					Status:     InputCompletionStatus_Accepted,
					Outputs:    [][]byte{[]byte(fmt.Sprintf("output-%d", i))},
					OutputsProof: OutputsProof{
						OutputsHash: crypto.Keccak256Hash(
							[]byte(fmt.Sprintf("outputs-%d", i))),
						MachineHash: crypto.Keccak256Hash(
							[]byte(fmt.Sprintf("machine-%d", i))),
					},
				}
				errs[i] = s.Repo.StoreAdvanceResult(
					s.Ctx, seed.App.ID, result)
			}()
		}
		wg.Wait()

		successCount := 0
		for _, err := range errs {
			if err == nil {
				successCount++
			}
		}
		s.GreaterOrEqual(successCount, 1,
			"at least one concurrent store should succeed")

		// Verify data integrity: the input must be in Accepted state
		input, err := s.Repo.GetInput(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(InputCompletionStatus_Accepted, input.Status)

		// ProcessedInputs must reflect exactly one successful processing
		count, err := s.Repo.GetProcessedInputCount(
			s.Ctx, seed.App.IApplicationAddress.String())
		s.Require().NoError(err)
		s.GreaterOrEqual(count, uint64(1))
	})
}

func (s *BulkOperationsSuite) TestStoreClaimAndProofsRollback() {
	// When the epoch doesn't exist, updateEpochClaim should fail with
	// RowsAffected == 0 and the whole transaction should be rolled back.
	s.Run("RollbackOnEpochUpdateFailure", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// Store advance result first so the epoch has outputs
		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			Outputs:    [][]byte{[]byte("output")},
			OutputsProof: OutputsProof{
				OutputsHash: crypto.Keccak256Hash([]byte("outputs")),
				MachineHash: crypto.Keccak256Hash([]byte("machine")),
			},
		}
		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().NoError(err)

		// Build an epoch that doesn't exist in the DB
		commitmentHash := crypto.Keccak256Hash([]byte("commitment"))
		badEpoch := &Epoch{
			ApplicationID:   seed.App.ID,
			Index:           99, // doesn't exist
			Commitment:      &commitmentHash,
			CommitmentProof: []common.Hash{UniqueHash()},
		}

		err = s.Repo.StoreClaimAndProofs(s.Ctx, badEpoch, nil)
		s.Require().Error(err)

		// Verify the real epoch is untouched
		gotEpoch, err := s.Repo.GetEpoch(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_Closed, gotEpoch.Status)
		s.Nil(gotEpoch.Commitment)
	})

	// When updateOutputs fails (output doesn't exist), the epoch status
	// change from updateEpochClaim must also be rolled back.
	s.Run("RollbackOnOutputProofUpdateFailure", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// Advance epoch to INPUTS_PROCESSED so updateEpochClaim can
		// set CLAIM_COMPUTED (the trigger rejects other transitions).
		AdvanceEpochStatus(s.Ctx, s.T(), s.Repo,
			seed.App.IApplicationAddress.String(), seed.Epoch,
			EpochStatus_InputsProcessed)

		// Store advance result to create one output (index 0)
		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			Outputs:    [][]byte{[]byte("real-output")},
			OutputsProof: OutputsProof{
				OutputsHash: crypto.Keccak256Hash([]byte("outputs")),
				MachineHash: crypto.Keccak256Hash([]byte("machine")),
			},
		}
		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().NoError(err)

		// Prepare a valid epoch claim (this part would succeed)
		commitmentHash := crypto.Keccak256Hash([]byte("commitment"))
		seed.Epoch.Commitment = &commitmentHash
		seed.Epoch.CommitmentProof = []common.Hash{UniqueHash()}

		// Prepare an output with a non-existent index to trigger failure
		badHash := UniqueHash()
		nonExistentOutput := &Output{
			InputEpochApplicationID: seed.App.ID,
			InputIndex:              0,
			Index:                   999, // doesn't exist
			Hash:                    &badHash,
			OutputHashesSiblings:    []common.Hash{UniqueHash()},
		}

		err = s.Repo.StoreClaimAndProofs(
			s.Ctx, seed.Epoch, []*Output{nonExistentOutput})
		s.Require().Error(err)

		// Verify the epoch status was rolled back — still InputsProcessed
		gotEpoch, err := s.Repo.GetEpoch(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_InputsProcessed, gotEpoch.Status,
			"epoch status should be rolled back to InputsProcessed")
		s.Nil(gotEpoch.Commitment,
			"commitment should not have been persisted")

		// Verify the real output's hash was NOT changed
		realOutput, err := s.Repo.GetOutput(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Nil(realOutput.Hash,
			"existing output hash should remain nil after rollback")
	})
}

func (s *BulkOperationsSuite) TestStoreTournamentEventsRollback() {
	// Helper: create a PRT application with one closed epoch and a tournament.
	setupPRTApp := func() (app *Application, tournAddr common.Address) {
		s.T().Helper()
		app = NewApplicationBuilder().
			WithConsensus(Consensus_PRT).
			Create(s.Ctx, s.T(), s.Repo)

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).WithBlocks(0, 9).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			s.Ctx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().NoError(err)

		tournAddr = UniqueAddress()
		tournament := NewTournamentBuilder(app.ID).
			WithEpochIndex(0).WithAddress(tournAddr).Build()
		err = s.Repo.CreateTournament(
			s.Ctx, app.IApplicationAddress.String(), tournament)
		s.Require().NoError(err)
		return app, tournAddr
	}

	// Insert valid commitments + a match that references a non-existent
	// tournament address, causing an FK violation. The commitments
	// inserted in the same transaction must be rolled back.
	s.Run("RollbackOnMatchInsertFailure", func() {
		app, tournAddr := setupPRTApp()

		// Valid commitment targeting the real tournament
		commitment := NewCommitmentBuilder(app.ID).
			WithEpochIndex(0).WithTournamentAddress(tournAddr).Build()

		// Match targeting a non-existent tournament → FK violation
		bogusAddr := UniqueAddress()
		match := NewMatchBuilder(app.ID).
			WithEpochIndex(0).
			WithTournamentAddress(bogusAddr).
			WithCommitmentOne(commitment.Commitment).
			WithCommitmentTwo(UniqueHash()).
			Build()

		err := s.Repo.StoreTournamentEvents(
			s.Ctx, app.ID,
			[]*Commitment{commitment},
			[]*Match{match},
			nil, nil, 100)
		s.Require().Error(err)

		// Verify the commitment was rolled back
		got, err := s.Repo.GetCommitment(
			s.Ctx, app.IApplicationAddress.String(),
			0, tournAddr.String(), commitment.Commitment.Hex())
		s.Require().NoError(err)
		s.Nil(got, "commitment should have been rolled back")
	})

	// Insert valid commitments + try to delete (update) a non-existent
	// match. updateMatches returns an error when RowsAffected == 0.
	// The commitments inserted earlier in the same tx must be rolled back.
	s.Run("RollbackOnMatchDeleteFailure", func() {
		app, tournAddr := setupPRTApp()

		// Valid new commitment
		newCommitment := NewCommitmentBuilder(app.ID).
			WithEpochIndex(0).WithTournamentAddress(tournAddr).Build()

		// Try to delete a match that doesn't exist
		deletedMatch := &Match{
			EpochIndex:        0,
			TournamentAddress: tournAddr,
			IDHash:            UniqueHash(), // doesn't exist
			Winner:            WinnerCommitment_ONE,
			DeletionReason:    MatchDeletionReason_TIMEOUT,
		}

		err := s.Repo.StoreTournamentEvents(
			s.Ctx, app.ID,
			[]*Commitment{newCommitment},
			nil, nil,
			[]*Match{deletedMatch}, 100)
		s.Require().Error(err)

		// Verify the new commitment was rolled back
		got, err := s.Repo.GetCommitment(
			s.Ctx, app.IApplicationAddress.String(),
			0, tournAddr.String(), newCommitment.Commitment.Hex())
		s.Require().NoError(err)
		s.Nil(got, "commitment should have been rolled back")
	})

	// Insert a match advanced event for a match that doesn't exist,
	// causing an FK violation. Commitments in the same tx should roll back.
	s.Run("RollbackOnMatchAdvancedInsertFailure", func() {
		app, tournAddr := setupPRTApp()

		// Valid new commitment
		newCommitment := NewCommitmentBuilder(app.ID).
			WithEpochIndex(0).WithTournamentAddress(tournAddr).Build()

		// Match advanced for a non-existent match → FK violation
		bogusMA := NewMatchAdvancedBuilder(app.ID).
			WithEpochIndex(0).
			WithTournamentAddress(tournAddr).
			WithIDHash(UniqueHash()). // match doesn't exist
			Build()

		err := s.Repo.StoreTournamentEvents(
			s.Ctx, app.ID,
			[]*Commitment{newCommitment},
			nil,
			[]*MatchAdvanced{bogusMA},
			nil, 100)
		s.Require().Error(err)

		// Verify the commitment was rolled back
		got, err := s.Repo.GetCommitment(
			s.Ctx, app.IApplicationAddress.String(),
			0, tournAddr.String(), newCommitment.Commitment.Hex())
		s.Require().NoError(err)
		s.Nil(got, "commitment should have been rolled back")
	})
}

func (s *BulkOperationsSuite) TestContextCancellation() {
	// Verify that StoreAdvanceResult respects a cancelled context
	// and does not persist any data.
	s.Run("StoreAdvanceResultWithCancelledContext", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		cancelledCtx, cancel := context.WithCancel(s.Ctx)
		cancel() // cancel immediately

		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			Outputs:    [][]byte{[]byte("should-not-persist")},
			Reports:    [][]byte{[]byte("should-not-persist")},
			OutputsProof: OutputsProof{
				OutputsHash: UniqueHash(),
				MachineHash: UniqueHash(),
			},
		}

		err := s.Repo.StoreAdvanceResult(cancelledCtx, seed.App.ID, result)
		s.Require().Error(err)
		s.Require().ErrorIs(err, context.Canceled)

		// Data should not have been persisted
		input, err := s.Repo.GetInput(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(InputCompletionStatus_None, input.Status,
			"input should remain unprocessed after context cancellation")

		outputs, total, err := s.Repo.ListOutputs(
			s.Ctx, seed.App.IApplicationAddress.String(),
			repository.OutputFilter{}, repository.Pagination{Limit: 10}, false)
		s.Require().NoError(err)
		s.Empty(outputs)
		s.Equal(uint64(0), total)
	})

	// Verify that CreateEpochsAndInputs respects a cancelled context.
	s.Run("CreateEpochsAndInputsWithCancelledContext", func() {
		app := NewApplicationBuilder().Create(s.Ctx, s.T(), s.Repo)

		cancelledCtx, cancel := context.WithCancel(s.Ctx)
		cancel() // cancel immediately

		epoch := NewEpochBuilder(app.ID).
			WithIndex(0).WithStatus(EpochStatus_Closed).WithBlocks(0, 9).Build()
		input := NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()

		err := s.Repo.CreateEpochsAndInputs(
			cancelledCtx, app.IApplicationAddress.String(),
			map[*Epoch][]*Input{epoch: {input}}, 10)
		s.Require().Error(err)

		// The epoch should not have been persisted
		got, err := s.Repo.GetEpoch(
			s.Ctx, app.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Nil(got, "epoch should not exist after context cancellation")
	})

	// Verify that StoreClaimAndProofs respects a cancelled context.
	s.Run("StoreClaimAndProofsWithCancelledContext", func() {
		seed := Seed(s.Ctx, s.T(), s.Repo)

		// Store an advance result so the epoch has outputs
		result := &AdvanceResult{
			EpochIndex: 0,
			InputIndex: 0,
			Status:     InputCompletionStatus_Accepted,
			Outputs:    [][]byte{[]byte("output")},
			OutputsProof: OutputsProof{
				OutputsHash: crypto.Keccak256Hash([]byte("outputs")),
				MachineHash: crypto.Keccak256Hash([]byte("machine")),
			},
		}
		err := s.Repo.StoreAdvanceResult(s.Ctx, seed.App.ID, result)
		s.Require().NoError(err)

		cancelledCtx, cancel := context.WithCancel(s.Ctx)
		cancel()

		commitmentHash := crypto.Keccak256Hash([]byte("commitment"))
		seed.Epoch.Commitment = &commitmentHash
		seed.Epoch.CommitmentProof = []common.Hash{UniqueHash()}

		err = s.Repo.StoreClaimAndProofs(cancelledCtx, seed.Epoch, nil)
		s.Require().Error(err)

		// Epoch should remain in Closed state
		gotEpoch, err := s.Repo.GetEpoch(
			s.Ctx, seed.App.IApplicationAddress.String(), 0)
		s.Require().NoError(err)
		s.Equal(EpochStatus_Closed, gotEpoch.Status,
			"epoch status should not change after context cancellation")
	})
}
