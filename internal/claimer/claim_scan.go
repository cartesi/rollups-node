// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	claimScanBlockLimit  uint64 = 4096
	claimScanEventTarget uint64 = 256
	claimScanSplitFactor uint64 = 2
	claimDecimalRadix    int    = 10
)

var errClaimDeploymentPending = fmt.Errorf("%w: application deployment is not visible at the selected head", txsender.ErrInconclusive)

type observedClaimKey struct {
	Covered string
	Machine common.Hash
}

type observedClaim struct {
	Key                  observedClaimKey
	Outputs              common.Hash
	Stage, Acceptance    *types.Log
	StagingBlock, Period *big.Int
}

func (c observedClaim) acceptanceFrom() string {
	if c.StagingBlock == nil || c.Period == nil {
		return "unknown"
	}
	return new(big.Int).Add(c.StagingBlock, c.Period).String()
}

type claimRead struct {
	At               txsender.BlockRef
	Staged, Accepted *big.Int
}

type claimScan struct {
	Read   claimRead
	Claims []observedClaim
}

func claimUint256(n *big.Int) bool { return n != nil && n.Sign() >= 0 && n.BitLen() <= 256 }

func (cb *claimerBlockchain) claimReference(ctx context.Context, number uint64) (txsender.BlockRef, error) {
	if number == cb.observed.Number {
		return cb.observed, nil
	}
	h, err := cb.backend.Header(ctx, hexutil.EncodeUint64(number))
	if err != nil {
		return txsender.BlockRef{}, err
	}
	if h == nil || h.Number == nil || !h.Number.IsUint64() || h.Number.Uint64() != number {
		return txsender.BlockRef{}, txsender.ErrInconclusive
	}
	return txsender.BlockRef{Number: number, Hash: h.Hash()}, nil
}

func (cb *claimerBlockchain) checkClaimReference(ctx context.Context, ref txsender.BlockRef) error {
	if ref.Hash == (common.Hash{}) || ref.Number > cb.observed.Number {
		return txsender.ErrInconclusive
	}
	canonical, err := cb.backend.Canonical(ctx, ref)
	if err != nil {
		return err
	}
	if !canonical {
		return txsender.ErrNonCanonical
	}
	return nil
}

// Tick already validated this exact observed head. Other references still need
// a pre-use check; post-read checks always use checkClaimReference directly.
func (cb *claimerBlockchain) checkClaimRead(ctx context.Context, ref txsender.BlockRef) error {
	if ref == cb.observed && ref.Hash != (common.Hash{}) {
		return ctx.Err()
	}
	return cb.checkClaimReference(ctx, ref)
}

func claimScanCallOpts(ctx context.Context, ref txsender.BlockRef) *bind.CallOpts {
	return &bind.CallOpts{Context: ctx, BlockHash: ref.Hash}
}

func (cb *claimerBlockchain) scanClaims(ctx context.Context, app *model.Application, cursor uint64,
	previous *claimRead, maxRange uint64) (*claimScan, error) {
	head := cb.observed
	if app == nil || head.Hash == (common.Hash{}) || cursor > head.Number {
		return nil, txsender.ErrInconclusive
	}
	if previous != nil {
		if previous.At.Number != cursor || !claimUint256(previous.Staged) || !claimUint256(previous.Accepted) ||
			previous.Accepted.Cmp(previous.Staged) > 0 {
			return nil, txsender.ErrInconclusive
		}
		if previous.At == head {
			return nil, nil
		}
		if err := cb.checkClaimRead(ctx, previous.At); err != nil {
			return nil, err
		}
	}
	protected := txsender.ProtectedClient{Client: cb.client}
	application, err := iapplication.NewIApplicationCaller(app.IApplicationAddress, protected)
	if err != nil {
		return nil, err
	}
	checkBinding := func(ref txsender.BlockRef) error {
		address, readErr := application.GetOutputsMerkleRootValidator(claimScanCallOpts(ctx, ref))
		if readErr != nil {
			return readErr
		}
		if address != app.IConsensusAddress {
			return fmt.Errorf("%w: registered consensus %s differs from observed %s",
				txsender.ErrInconclusive, app.IConsensusAddress, address)
		}
		return nil
	}
	if err = checkBinding(head); err != nil {
		if errors.Is(err, bind.ErrNoCode) && previous == nil {
			return nil, errClaimDeploymentPending
		}
		return nil, err
	}
	caller, err := iconsensus.NewIConsensusCaller(app.IConsensusAddress, protected)
	if err != nil {
		return nil, err
	}
	counters := func(ref txsender.BlockRef) (claimRead, error) {
		staged, readErr := caller.GetNumberOfStagedClaims(claimScanCallOpts(ctx, ref), app.IApplicationAddress)
		if errors.Is(readErr, bind.ErrNoCode) && ref.Number < head.Number {
			// The protected binding confirmed no code at this canonical hash.
			// Missing historical state remains an error, never zero counters.
			return claimRead{At: ref, Staged: new(big.Int), Accepted: new(big.Int)}, nil
		}
		if readErr != nil {
			return claimRead{}, readErr
		}
		accepted, readErr := caller.GetNumberOfAcceptedClaims(claimScanCallOpts(ctx, ref), app.IApplicationAddress)
		if readErr != nil {
			return claimRead{}, readErr
		}
		if !claimUint256(staged) || !claimUint256(accepted) || accepted.Cmp(staged) > 0 {
			return claimRead{}, txsender.ErrInconclusive
		}
		return claimRead{At: ref, Staged: staged, Accepted: accepted}, nil
	}
	var before claimRead
	if previous != nil {
		before = *previous
	} else {
		deployment, readErr := application.GetDeploymentBlockNumber(claimScanCallOpts(ctx, head))
		if readErr != nil {
			return nil, readErr
		}
		if deployment == nil || !deployment.IsUint64() || deployment.Uint64() > head.Number {
			return nil, txsender.ErrInconclusive
		}
		// No claim can precede the application's deployment. Include its entire
		// deployment block, even if the InputBox was registered much later.
		if deployment.Uint64() > cursor {
			ref, readErr := cb.claimReference(ctx, deployment.Uint64()-1)
			if readErr != nil {
				return nil, readErr
			}
			before = claimRead{At: ref, Staged: new(big.Int), Accepted: new(big.Int)}
		} else {
			ref, readErr := cb.claimReference(ctx, cursor)
			if readErr != nil {
				return nil, readErr
			}
			if err = checkBinding(ref); err != nil {
				return nil, err
			}
			before, err = counters(ref)
			if err != nil {
				return nil, err
			}
		}
	}
	if !claimUint256(before.Staged) || !claimUint256(before.Accepted) || before.Accepted.Cmp(before.Staged) > 0 {
		return nil, txsender.ErrInconclusive
	}
	if before.At.Number == head.Number {
		return nil, nil
	}
	tip, err := counters(head)
	if err != nil {
		return nil, err
	}
	if tip.Staged.Cmp(before.Staged) < 0 || tip.Accepted.Cmp(before.Accepted) < 0 {
		return nil, txsender.ErrInconclusive
	}
	result := &claimScan{Read: tip}
	if tip.Staged.Cmp(before.Staged) != 0 || tip.Accepted.Cmp(before.Accepted) != 0 {
		start := before.At.Number + 1
		limit := claimScanBlockLimit
		if maxRange > 0 && maxRange < limit {
			limit = maxRange
		}
		end := head.Number
		if end-start >= limit {
			end = start + limit - 1
		}
		for {
			if end != head.Number {
				ref, readErr := cb.claimReference(ctx, end)
				if readErr != nil {
					return nil, readErr
				}
				if err = checkBinding(ref); err != nil {
					return nil, err
				}
				result.Read, err = counters(ref)
				if err != nil {
					return nil, err
				}
			}
			if result.Read.Staged.Cmp(before.Staged) < 0 || result.Read.Accepted.Cmp(before.Accepted) < 0 ||
				result.Read.Staged.Cmp(tip.Staged) > 0 || result.Read.Accepted.Cmp(tip.Accepted) > 0 {
				return nil, txsender.ErrInconclusive
			}
			delta := new(big.Int).Sub(result.Read.Staged, before.Staged)
			delta.Add(delta, new(big.Int).Sub(result.Read.Accepted, before.Accepted))
			// This is an admission target. An indivisible block is always read whole.
			if end == start || delta.Cmp(new(big.Int).SetUint64(claimScanEventTarget)) <= 0 {
				break
			}
			end = start + (end-start)/claimScanSplitFactor
		}
		result.Claims, err = cb.readClaimLogs(ctx, app, start, before, result.Read, caller)
		if err != nil {
			return nil, err
		}
	}
	references := []txsender.BlockRef{before.At, result.Read.At}
	if result.Read.At != head {
		references = append(references, head)
	}
	for _, ref := range references {
		if err = cb.checkClaimReference(ctx, ref); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func orderedClaimLog(previous, next types.Log) bool {
	if previous.BlockNumber != next.BlockNumber {
		return previous.BlockNumber < next.BlockNumber
	}
	return previous.BlockHash == next.BlockHash && previous.Index < next.Index && previous.TxIndex <= next.TxIndex &&
		(previous.TxIndex == next.TxIndex) == (previous.TxHash == next.TxHash)
}

func (cb *claimerBlockchain) readClaimLogs(ctx context.Context, app *model.Application, start uint64,
	before, after claimRead, caller *iconsensus.IConsensusCaller) ([]observedClaim, error) {
	abi, err := iconsensus.IConsensusMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	stagedTopic, acceptedTopic := abi.Events["ClaimStaged"].ID, abi.Events["ClaimAccepted"].ID
	binding, err := iconsensus.NewIConsensus(app.IConsensusAddress, cb.client)
	if err != nil {
		return nil, err
	}
	filter := ethutil.Filter{MaxChunkSize: new(big.Int).SetUint64(claimScanBlockLimit), Logger: cb.logger}
	logs, err := filter.ChunkedFilterLogs(ctx, cb.client, ethereum.FilterQuery{
		Addresses: []common.Address{app.IConsensusAddress},
		Topics:    [][]common.Hash{{stagedTopic, acceptedTopic}, {common.BytesToHash(app.IApplicationAddress.Bytes())}},
		FromBlock: new(big.Int).SetUint64(start), ToBlock: new(big.Int).SetUint64(after.At.Number),
	})
	if err != nil {
		return nil, err
	}
	var claims []observedClaim
	positions := map[observedClaimKey]int{}
	checked := map[txsender.BlockRef]bool{}
	var prior *types.Log
	var staged, accepted uint64
	for raw, readErr := range logs {
		if readErr != nil {
			return nil, readErr
		}
		if raw == nil || raw.Address != app.IConsensusAddress || raw.BlockNumber < start || raw.BlockNumber > after.At.Number ||
			raw.Removed || raw.BlockHash == (common.Hash{}) || raw.TxHash == (common.Hash{}) || len(raw.Topics) != 2 ||
			raw.Topics[1] != common.BytesToHash(app.IApplicationAddress.Bytes()) || prior != nil && !orderedClaimLog(*prior, *raw) {
			return nil, txsender.ErrInconclusive
		}
		ref := txsender.BlockRef{Number: raw.BlockNumber, Hash: raw.BlockHash}
		if !checked[ref] {
			if err = cb.checkClaimReference(ctx, ref); err != nil {
				return nil, err
			}
			checked[ref] = true
		}
		claim := observedClaim{}
		switch raw.Topics[0] {
		case stagedTopic:
			event, parseErr := binding.ParseClaimStaged(*raw)
			if parseErr != nil {
				return nil, parseErr
			}
			claim.Key = observedClaimKey{Covered: event.LastProcessedBlockNumber.String(), Machine: common.Hash(event.MachineMerkleRoot)}
			claim.Outputs, claim.Stage = common.Hash(event.OutputsMerkleRoot), raw
			staged++
		case acceptedTopic:
			event, parseErr := binding.ParseClaimAccepted(*raw)
			if parseErr != nil {
				return nil, parseErr
			}
			claim.Key = observedClaimKey{Covered: event.LastProcessedBlockNumber.String(), Machine: common.Hash(event.MachineMerkleRoot)}
			claim.Outputs, claim.Acceptance = common.Hash(event.OutputsMerkleRoot), raw
			accepted++
		default:
			return nil, txsender.ErrInconclusive
		}
		if position, exists := positions[claim.Key]; exists {
			saved := &claims[position]
			if saved.Outputs != claim.Outputs || claim.Stage != nil || saved.Acceptance != nil {
				return nil, txsender.ErrInconclusive
			}
			saved.Acceptance = claim.Acceptance
		} else {
			positions[claim.Key] = len(claims)
			claims = append(claims, claim)
		}
		prior = raw
	}
	// Count every log before projecting to pending claims. An acceptance can
	// refer to a claim that a prior local comparison already completed.
	if new(big.Int).Add(before.Staged, new(big.Int).SetUint64(staged)).Cmp(after.Staged) != 0 ||
		new(big.Int).Add(before.Accepted, new(big.Int).SetUint64(accepted)).Cmp(after.Accepted) != 0 {
		return nil, fmt.Errorf("%w: staged/accepted logs do not match pinned counters", txsender.ErrInconclusive)
	}
	var period *big.Int
	for i := range claims {
		claim := &claims[i]
		if claim.Stage == nil {
			continue
		}
		if period == nil {
			period, err = caller.GetClaimStagingPeriod(claimScanCallOpts(ctx, after.At))
			if err != nil {
				return nil, err
			}
			if !claimUint256(period) {
				return nil, txsender.ErrInconclusive
			}
		}
		covered, ok := new(big.Int).SetString(claim.Key.Covered, claimDecimalRadix)
		if !ok {
			return nil, txsender.ErrInconclusive
		}
		state, readErr := caller.GetClaim(claimScanCallOpts(ctx, after.At), app.IApplicationAddress, covered, claim.Key.Machine)
		if readErr != nil {
			return nil, readErr
		}
		expected := claimStatusStaged
		if claim.Acceptance != nil {
			expected = claimStatusAccepted
		}
		if state.Status != expected || common.Hash(state.StagedOutputsMerkleRoot) != claim.Outputs ||
			!claimUint256(state.StagingBlockNumber) {
			return nil, txsender.ErrInconclusive
		}
		claim.StagingBlock, claim.Period = state.StagingBlockNumber, new(big.Int).Set(period)
	}
	return claims, nil
}
