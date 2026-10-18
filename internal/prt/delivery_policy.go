// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/txsender"
)

type actionKind uint8

const (
	noAction actionKind = iota
	joinTournament
	stageTournament
	acceptTournament
	recoverBond
)

const (
	intrinsicGasFloor        = 21_000
	maximumJoinGas           = 2_000_000 // Read-only protocol calls, not transaction sizing.
	recoveryGasLimit         = 200_000   // Includes the payment callback's 50,000 gas allowance.
	rootBondRecoveryPageSize = 16
)

func deliveryPolicy(c config.PrtConfig) (txsender.Policy, error) {
	return txsender.NewPolicy(txsender.FinancialConfig{
		MaxFeePerGas: c.BlockchainTxMaxFeePerGas, Legacy: c.BlockchainLegacyEnabled,
		SigningTimeout: c.BlockchainHttpRequestTimeout,
	})
}

// A sweep visits a finite range, then starts again. It is not payment evidence.
type rootBondRecoveryScan struct {
	ThroughEpoch uint64
	AfterEpoch   *uint64
}
