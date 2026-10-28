// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"fmt"
	"math/big"
	"time"
)

const (
	latestBlockTag                 = "latest"
	decimalBase                    = 10
	uint256Bits                    = 256
	percentBase                    = 100
	legacyRetryPercent             = 10
	baseFeeMultiplier              = 2
	gasPaddingDivisor              = 2
	defaultCallBytes               = 128 << 10
	supportedTransactionGasCeiling = 1 << 24
	minimumGasPadding              = 150_000
	intrinsicGas                   = 21_000
)

type FinancialConfig struct {
	MaxFeePerGas   string
	Legacy         bool
	SigningTimeout time.Duration
}

func NewPolicy(c FinancialConfig) (Policy, error) {
	p := Policy{Legacy: c.Legacy, SigningTimeout: c.SigningTimeout}
	if c.MaxFeePerGas != "" {
		for _, digit := range c.MaxFeePerGas {
			if digit < '0' || digit > '9' {
				return Policy{}, fmt.Errorf("CARTESI_BLOCKCHAIN_TX_MAX_FEE_PER_GAS must be positive decimal wei")
			}
		}
		var ok bool
		p.MaxFee, ok = new(big.Int).SetString(c.MaxFeePerGas, decimalBase)
		if !ok {
			return Policy{}, fmt.Errorf("CARTESI_BLOCKCHAIN_TX_MAX_FEE_PER_GAS must be positive decimal wei")
		}
	}
	return p, p.Validate()
}

func RuntimePolicy() Policy { return Policy{} }

func (p Policy) Validate() error {
	if p.SigningTimeout < 0 {
		return fmt.Errorf("transaction signing timeout must not be negative")
	}
	if p.MaxFee != nil && (p.MaxFee.Sign() <= 0 || p.MaxFee.BitLen() > uint256Bits) {
		return fmt.Errorf("CARTESI_BLOCKCHAIN_TX_MAX_FEE_PER_GAS must be a positive uint256 amount in wei")
	}
	return nil
}

func copyInt(x *big.Int) *big.Int {
	if x == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(x)
}

func minimum(a, b *big.Int) *big.Int {
	if a.Cmp(b) < 0 {
		return copyInt(a)
	}
	return copyInt(b)
}

// replacementMinimum is one rounded step, including a strict increase at zero.
func replacementMinimum(x *big.Int) *big.Int {
	rounded := new(big.Int).Mul(x, big.NewInt(percentBase+legacyRetryPercent))
	rounded.Add(rounded, big.NewInt(percentBase-1)).Div(rounded, big.NewInt(percentBase))
	plusOne := new(big.Int).Add(x, big.NewInt(1))
	if rounded.Cmp(plusOne) < 0 {
		return plusOne
	}
	return rounded
}
