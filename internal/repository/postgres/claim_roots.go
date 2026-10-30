// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
)

// Input coverage, the whole validation prefix and its final roots must use one
// statement snapshot. A/Q omit empty epochs; their last validated roots carry
// forward to a later aligned B. The first blocker seek matches the partial index.
const claimRootsSQL = `SELECT a.epoch_length,a.last_input_check_block,a.template_hash,
 last_epoch.index,last_epoch.machine_hash,last_epoch.tx_buffer_data_block,
 blocker.index,blocker.status,a.problem_epoch_index,a.problem_kind,a.problem_input_index
 FROM application a LEFT JOIN LATERAL (
 SELECT e.index,e.machine_hash,e.tx_buffer_data_block FROM epoch e
 WHERE e.application_id=a.id AND e.last_block <= $2::numeric
 ORDER BY e.last_block DESC,e.index DESC LIMIT 1) last_epoch ON true
 LEFT JOIN LATERAL (
 SELECT e.index,e.status FROM epoch e WHERE e.application_id=a.id
 AND e.first_block <= $2::numeric AND NOT e.claim_validated
 ORDER BY e.first_block LIMIT 1) blocker ON true WHERE a.id=$1`

func (r *PostgresRepository) ReadClaimRoots(ctx context.Context, appID int64, covered uint64) (repository.ClaimRoots, error) {
	ctx, cancel := beginDatabaseOperation(ctx)
	defer cancel()
	var result repository.ClaimRoots
	var length, inputBlock uint64
	var template common.Hash
	var epoch, blocker, problemEpoch, problemInput *uint64
	var machine, outputs *common.Hash
	var blockerStatus *model.EpochStatus
	var problemKind *model.ApplicationStatus
	err := r.db.QueryRow(ctx, claimRootsSQL, appID, strconv.FormatUint(covered, 10)).Scan(
		&length, &inputBlock, &template, &epoch, &machine, &outputs, &blocker, &blockerStatus, &problemEpoch, &problemKind, &problemInput)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, repository.ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if length == 0 {
		result.ReasonClass, result.Reason = "epoch_length", "registered epoch length is zero"
		return result, nil
	}
	if covered%length != length-1 {
		result.ReasonClass, result.Reason = "boundary", "covered block is not an A/Q epoch boundary"
		return result, nil
	}
	if inputBlock >= covered && blocker == nil {
		result.Ready = true
		result.MachineHash = template
		result.OutputsRoot = merkle.CreatePostContext()[merkle.TREE_DEPTH]
		if epoch != nil {
			if machine == nil || outputs == nil {
				return repository.ClaimRoots{}, fmt.Errorf("validated epoch %d has no claim roots", *epoch)
			}
			result.MachineHash, result.OutputsRoot = *machine, *outputs
		}
		return result, nil
	}
	problem, err := decodeApplicationProblem(problemEpoch, problemKind, problemInput)
	if err != nil {
		result.ReasonClass, result.Reason = "stop_metadata", "local stop metadata is inconsistent: "+err.Error()
		return result, nil
	}
	switch {
	case problem != nil && problem.EpochIndex <= covered/length:
		result.ReasonClass = "local_stop/" + string(problem.Kind)
		result.Reason = fmt.Sprintf("local %s at epoch %d prevents validation of the covered prefix", problem.Kind, problem.EpochIndex)
	case blocker != nil:
		result.ReasonClass = "validation/" + string(*blockerStatus)
		result.Reason = fmt.Sprintf("local %s at epoch %d is not validated", *blockerStatus, *blocker)
	default:
		result.ReasonClass = "input_coverage"
		result.Reason = fmt.Sprintf("input coverage ends at block %d, before covered block %d", inputBlock, covered)
	}
	return result, nil
}
