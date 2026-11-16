// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"context"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const invalidRootsCase = "IOR"

func TestPRTFinalIORCanJoinWithoutSettlement(t *testing.T) {
	for _, kind := range []actionKind{joinTournament, stageTournament, acceptTournament} {
		for _, missing := range []bool{false, true} {
			name := invalidRootsCase
			if missing {
				name = "unknown source"
			}
			t.Run(actionName(kind)+"/"+name, func(t *testing.T) {
				f, held, _ := preparePRTAction(t, kind)
				f.epoch.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
				if missing {
					f.epoch.CalculatedOutputsRoot = nil
				}
				call, err := f.d.planAction(t.Context(), f.app, kind, f.block, observerFacts(100))
				switch {
				case missing:
					require.ErrorIs(t, err, txsender.ErrInconclusive)
					require.Nil(t, call)
				case kind == joinTournament:
					require.NoError(t, err)
					require.NotNil(t, call)
					report := submitPRTAction(t, f, *call)
					require.True(t, report.Attempted)
				default:
					require.ErrorIs(t, err, txsender.ErrIneligible)
					require.Nil(t, call)
				}
				// A synthetic retained consumer fixture also rejects inadequate source evidence.
				eligibility, checkErr := f.d.Recheck(t.Context(), f.block, held.Key)
				switch {
				case missing:
					require.ErrorIs(t, checkErr, txsender.ErrInconclusive)
				case kind == joinTournament:
					require.NoError(t, checkErr)
					require.Equal(t, txsender.Useful, eligibility)
				default:
					require.NoError(t, checkErr)
					require.Equal(t, txsender.Obsolete, eligibility)
				}
				if missing || kind != joinTournament {
					require.Empty(t, f.backend.published)
				}
				require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			})
		}
	}
}

func TestPRTFinalReloadRefusesIORAndUnknownSettlement(t *testing.T) {
	for _, kind := range []actionKind{stageTournament, acceptTournament} {
		for _, missing := range []bool{false, true} {
			name := invalidRootsCase
			if missing {
				name = "unknown source"
			}
			t.Run(actionName(kind)+"/"+name, func(t *testing.T) {
				f, _, _ := preparePRTAction(t, kind)
				reads := 0
				f.repo.deploymentCompletion = func(ctx context.Context, key repository.DeploymentIdentity, index uint64) (
					*repository.DeploymentCompletionState, error,
				) {
					completion, err := f.repo.GetEpochCompletionState(ctx, key.ApplicationAddress.Hex(), index)
					require.NoError(t, err)
					row, err := f.repo.GetDeploymentAdmissionState(ctx, key)
					require.NoError(t, err)
					current := *completion.Epoch
					current.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
					if missing {
						current.CalculatedOutputsRoot = nil
					}
					completion.Epoch = &current
					reads++
					return &repository.DeploymentCompletionState{Completion: completion, Deployment: row}, nil
				}
				// This rejection fixture simulates an incomplete or invalid final read, not a mutable canonical result.
				call, err := f.d.planAction(t.Context(), f.app, kind, f.block, observerFacts(100))
				require.Nil(t, call)
				if missing {
					require.ErrorIs(t, err, txsender.ErrInconclusive)
				} else {
					require.ErrorIs(t, err, txsender.ErrIneligible)
				}
				require.Positive(t, reads)
				require.Empty(t, f.backend.published)
			})
		}
	}
}

func TestPRTLaterHealthyEpochKeepsItsOwnPolicyAfterIOR(t *testing.T) {
	for _, kind := range []actionKind{joinTournament, stageTournament, acceptTournament} {
		t.Run(actionName(kind), func(t *testing.T) {
			f, _, _ := preparePRTAction(t, kind)
			previous := resultTestEpoch(model.EpochStatus_ClaimAccepted)
			previous.Index = f.epoch.Index - 1
			previous.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
			require.True(t, previous.HasInvalidOutputsRoot())
			f.repo.On("GetEpoch", mock.Anything, f.app.IApplicationAddress.Hex(), previous.Index).Return(previous, nil).Maybe()
			call, err := f.d.planAction(t.Context(), f.app, kind, f.block, observerFacts(100))
			require.NoError(t, err)
			require.NotNil(t, call)
			require.Equal(t, f.epoch.Index, call.Key.Epoch)
			parsed, parseErr := idaveconsensus.IDaveConsensusMetaData.GetAbi()
			require.NoError(t, parseErr)
			expectedTarget := f.app.IConsensusAddress
			if kind == joinTournament {
				parsed, parseErr = itournament.ITournamentMetaData.GetAbi()
				require.NoError(t, parseErr)
				expectedTarget = *f.epoch.TournamentAddress
			}
			require.Equal(t, expectedTarget, call.To)
			const selectorBytes = 4
			method, parseErr := parsed.MethodById(call.Data[:selectorBytes])
			require.NoError(t, parseErr)
			values, parseErr := method.Inputs.Unpack(call.Data[selectorBytes:])
			require.NoError(t, parseErr)
			if kind == joinTournament {
				require.Equal(t, "joinTournament", method.Name)
				require.Equal(t, [32]byte(*f.epoch.MachineHash), values[0])
			} else {
				if kind == stageTournament {
					require.Equal(t, "stageTournamentResult", method.Name)
				} else {
					require.Equal(t, "acceptStagedTournamentResult", method.Name)
				}
				require.Equal(t, new(big.Int).SetUint64(f.epoch.Index), values[0])
			}
			submitPRTAction(t, f, *call)
			require.True(t, previous.HasInvalidOutputsRoot())
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
		})
	}
}

func TestPRTCurrentIORWithLaterTerminalStillJoins(t *testing.T) {
	for _, kind := range []actionKind{joinTournament, stageTournament, acceptTournament} {
		t.Run(actionName(kind), func(t *testing.T) {
			f, _, _ := preparePRTAction(t, kind)
			f.epoch.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
			setPRTTestStop(f, model.ApplicationStatus_MachineHalted, f.epoch.Index+1)
			f.repo.problems[f.app.ID].InputIndex = new(f.epoch.InputIndexUpperBound)
			f.app.ProcessedInputs = f.epoch.InputIndexUpperBound + 1
			call, err := f.d.planAction(t.Context(), f.app, kind, f.block, observerFacts(100))
			if kind == joinTournament {
				require.NoError(t, err)
				require.NotNil(t, call)
				submitPRTAction(t, f, *call)
			} else {
				require.ErrorIs(t, err, txsender.ErrIneligible)
				require.Nil(t, call)
				require.Empty(t, f.backend.published)
			}
			require.Equal(t, f.epoch.Index+1, f.repo.problems[f.app.ID].EpochIndex)
			require.Equal(t, f.epoch.InputIndexUpperBound, *f.repo.problems[f.app.ID].InputIndex)
			require.Equal(t, model.ApplicationStatus_MachineHalted, f.app.Status)
			require.True(t, f.epoch.HasInvalidOutputsRoot())
		})
	}
}

// The retained bad source is a rejection fixture, not a mutable canonical root.
func TestPRTRecheckIORIsObsoleteWhileProtocolWaits(t *testing.T) {
	for _, kind := range []actionKind{stageTournament, acceptTournament} {
		for _, invalid := range []bool{false, true} {
			name := "healthy"
			if invalid {
				name = invalidRootsCase
			}
			t.Run(actionName(kind)+"/"+name, func(t *testing.T) {
				f, held, view := preparePRTAction(t, kind)
				if invalid {
					f.epoch.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
				}
				if kind == stageTournament {
					view.stage.IsFinished = false
				} else {
					view.accept.IsClaimStagingPeriodOver = false
					view.accept.DoAllSentriesAgreeWithStagedTournamentResult = false
				}
				updatePRTView(f, f.block.Number+1, view)
				got, err := f.d.Recheck(t.Context(), f.block, held.Key)
				require.NoError(t, err)
				expected := txsender.ProtocolWait
				if invalid {
					expected = txsender.Obsolete
				}
				require.Equal(t, expected, got)
				require.Empty(t, f.backend.published)
			})
		}
	}
}

func TestPRTFreshUncomputedEpochIsSilentlyIneligible(t *testing.T) {
	for _, status := range []model.EpochStatus{model.EpochStatus_Closed, model.EpochStatus_InputsProcessed} {
		t.Run(status.String(), func(t *testing.T) {
			f, _, _ := preparePRTAction(t, stageTournament)
			f.epoch.Status = status
			f.epoch.CalculatedOutputsRoot, f.epoch.MachineHash = nil, nil
			f.epoch.Commitment, f.epoch.TxBufferDataBlock = nil, nil
			call, err := f.d.planAction(t.Context(), f.app, stageTournament, f.block, observerFacts(f.block.Number))
			require.Nil(t, call)
			require.ErrorIs(t, err, txsender.ErrIneligible)
			require.Empty(t, f.backend.published)
		})
	}
}
