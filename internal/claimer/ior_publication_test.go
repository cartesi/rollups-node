// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestClaimPublisherWithholdsIORAndUnknownResults(t *testing.T) {
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		for _, staged := range []bool{false, true} {
			for _, missing := range []bool{false, true} {
				name := "submit"
				if staged {
					name = "accept external stage"
				}
				source := "IOR"
				if missing {
					source = "unknown source"
				}
				t.Run(consensus.String()+"/"+name+"/"+source, func(t *testing.T) {
					s, backend, lookup := newDeliveryFixture(t, model.DefaultBlock_Latest)
					app, epoch := deliveryApplication(1, consensus)
					lookup.add(app, epoch)
					backend.apps[app.IApplicationAddress] = app
					if staged {
						epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
						backend.claims[app.ID] = makeClaimStatus(claimStatusStaged, epoch, 30)
					}
					block := txsender.BlockRef{Number: backend.last, Hash: deliveryHeader(backend.last).Hash()}
					work := repository.ClaimWork{Application: app, Epoch: epoch}
					healthy, state, err := s.publisher.decide(t.Context(), block, work, false)
					require.NoError(t, err)
					require.Equal(t, txsender.Useful, state, "healthy control must reach a real publishable action")
					epoch.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
					if missing {
						epoch.CalculatedOutputsRoot = nil
					}
					for _, held := range []bool{false, true} {
						_, state, err = s.publisher.decide(t.Context(), block, work, held)
						if missing {
							require.ErrorIs(t, err, txsender.ErrInconclusive)
						} else {
							require.NoError(t, err)
							require.Equal(t, txsender.Obsolete, state)
						}
						if !missing {
							require.NotEqual(t, txsender.Useful, state)
						}
					}
					_, state, err = s.publisher.currentDecision(t.Context(), block, healthy.action)
					if missing {
						require.ErrorIs(t, err, txsender.ErrInconclusive)
					} else {
						require.NoError(t, err)
						require.Equal(t, txsender.Obsolete, state)
					}
					require.Empty(t, backend.published)
					require.Equal(t, model.ApplicationStatus_OK, app.Status)
				})
			}
		}
	}
}

func TestClaimPublisherLaterHealthyEpochIsNotVetoedByPriorIOR(t *testing.T) {
	for _, consensus := range []model.Consensus{model.Consensus_Authority, model.Consensus_Quorum} {
		t.Run(consensus.String(), func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t, model.DefaultBlock_Latest)
			app, epoch := deliveryApplication(1, consensus)
			previous := makeComputedEpoch(app, epoch.Index-1)
			previous.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
			lookup.add(app, previous)
			lookup.add(app, epoch)
			backend.apps[app.IApplicationAddress] = app
			require.NoError(t, s.publisher.tick(t.Context()))
			require.Len(t, backend.published, 1)
			require.Equal(t, "submitClaim", publishedClaimMethod(t, backend.published[0].Data()))
			parsed, err := iconsensus.IConsensusMetaData.GetAbi()
			require.NoError(t, err)
			const selectorBytes = 4
			values, err := parsed.Methods[claimSubmitMethod].Inputs.Unpack(backend.published[0].Data()[selectorBytes:])
			require.NoError(t, err)
			require.Equal(t, app.IApplicationAddress, values[0])
			require.Equal(t, new(big.Int).SetUint64(epoch.LastBlock), values[1])
			const machineArgument = 2
			require.Equal(t, [32]byte(*epoch.MachineHash), values[machineArgument])
			require.True(t, previous.HasInvalidOutputsRoot())
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
		})
	}
}

func TestClaimObserverComparesDeclaredIORResult(t *testing.T) {
	for _, mismatch := range []string{"none", "machine", "outputs"} {
		t.Run(mismatch, func(t *testing.T) {
			observer, repo, chain, app := observerFixture(t)
			epoch := makeComputedEpoch(app, 1)
			epoch.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
			require.True(t, epoch.HasInvalidOutputsRoot())
			machine, declared := *epoch.MachineHash, *epoch.TxBufferDataBlock
			acceptedMachine, acceptedOutputs := machine, declared
			if mismatch == "machine" {
				acceptedMachine[0] ^= 1
			}
			if mismatch == "outputs" {
				acceptedOutputs[0] ^= 1
			}
			fact := observerFact(19, 25, acceptedMachine, acceptedOutputs)
			fact.Acceptance = &types.Log{BlockNumber: 28, BlockHash: common.HexToHash("0x28"), TxHash: common.HexToHash("0x2")}
			repo.roots[19] = readyRoots(machine, declared)
			chain.scan = func(context.Context, *model.Application, uint64, *claimRead) (*claimScan, error) {
				batch := observerBatch(29, fact)
				batch.Read.Accepted = big.NewInt(1)
				return batch, nil
			}
			_, err := observer.observe(t.Context(), 29)
			require.NoError(t, err)
			if mismatch == "none" {
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
				require.Zero(t, repo.statusCalls)
			} else {
				require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
				require.Equal(t, 1, repo.statusCalls)
			}
			require.True(t, epoch.HasInvalidOutputsRoot())
		})
	}
}
