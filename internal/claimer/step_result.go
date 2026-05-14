// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

// claimStepResult tells the outer loop what happened after one work item.
// The handler already did the real work, such as DB writes or chain reads.
// The loop only needs to count progress, return one error, and maybe remove
// the item from the in-memory work map.
type claimStepResult struct {
	progress int
	drop     bool
	err      error
}

func claimNoProgress() claimStepResult {
	return claimStepResult{}
}

func claimProgressed(n int) claimStepResult {
	return claimStepResult{progress: n}
}

func claimDropped(err error) claimStepResult {
	return claimStepResult{drop: true, err: err}
}

func claimWorkCompleted(n int) claimStepResult {
	return claimStepResult{progress: n, drop: true}
}

func claimRetryLater(err error) claimStepResult {
	return claimStepResult{err: err}
}
