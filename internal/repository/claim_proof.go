// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
)

// OutputProof identifies one output and its cumulative-tree proof.
// The writer obtains the application ID from the claim's epoch.
type OutputProof struct {
	Index    uint64
	Hash     common.Hash
	Siblings []common.Hash
}

// ClaimProofWriter uses the claim transaction's connection.
// It is valid only during write and is not safe for concurrent use.
// A scan callback must not call the writer while its rows are open.
type ClaimProofWriter interface {
	ScanOutputs(ctx context.Context, first, last uint64, fn OutputScanFunc) error
	StoreOutputProofs(ctx context.Context, proofs []OutputProof) error
}

// ClaimProofWriteFunc writes proofs before the claim transaction commits.
// An error rolls back the claim and all proof batches.
type ClaimProofWriteFunc func(context.Context, ClaimProofWriter) error
