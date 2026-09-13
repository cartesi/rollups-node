// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package ethutil

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cartesi/rollups-node/pkg/contracts/ierc20metadata"
	"github.com/cartesi/rollups-node/pkg/contracts/iusdwithdrawaloutputbuilder"
)

// usdAccountSize is the exact byte length of the LibUsdAccount encoding consumed
// by every UsdWithdrawalOutputBuilder:
//
//	bytes 0..11  uint96 balance, little-endian
//	bytes 12..31 20-byte user address
const usdAccountSize = 32

// DescribeWithdrawalAccount renders a multi-line human description of the
// `account` bytes consumed by an IApplication.withdraw() call so the
// operator can verify the recipient and amount before signing.
//
// Algorithm:
//
//  1. Call IUsdWithdrawalOutputBuilder.Token() on the on-chain builder.
//     A revert here means the builder is not a USD-family builder; the
//     caller should fall back to a raw-bytes display.
//  2. Require and split the exact 32-byte record into recipient and balance
//     per LibUsdAccount. Any other length is a hard error.
//  3. Best-effort fetch IERC20Metadata.Symbol() and Decimals() on the
//     returned token address so the balance can be rendered as a
//     fixed-point amount. If either view reverts (broken or non-standard
//     ERC-20), the raw uint96 balance is shown unmodified.
//
// Tri-state return:
//
//   - (desc, true, nil):  builder recognized and account decoded.
//   - ("",   true, err):  builder recognized but the bytes do not match
//     the USD encoding — surface to the operator.
//   - ("",  false, nil):  Token() reverted — caller falls back to raw
//     bytes and stricter confirmation.
func DescribeWithdrawalAccount(
	ctx context.Context,
	client *ethclient.Client,
	builder common.Address,
	account []byte,
) (description string, matched bool, err error) {
	b, err := iusdwithdrawaloutputbuilder.NewIUsdWithdrawalOutputBuilder(builder, client)
	if err != nil {
		return "", false, nil
	}
	token, err := b.Token(&bind.CallOpts{Context: ctx})
	if err != nil {
		return "", false, nil
	}
	recipient, balance, err := decodeUSDAccount(account)
	if err != nil {
		return "", true, fmt.Errorf("%w (token %s)", err, token)
	}

	symbol, decimals, metaOK := fetchERC20Metadata(ctx, client, token)
	tokenLine := fmt.Sprintf("  token:               %s", token)
	if metaOK {
		tokenLine = fmt.Sprintf("  token:               %s %s", token, symbol)
	}
	var amountLine string
	if metaOK {
		amountLine = fmt.Sprintf(
			"  amount:              %s %s  (raw: %s, decimals: %d)",
			formatTokenAmount(balance, decimals), symbol, balance.String(), decimals)
	} else {
		amountLine = fmt.Sprintf(
			"  amount (raw uint96): %s  (token metadata unavailable)",
			balance.String())
	}
	return fmt.Sprintf(
		"USD-style account (recognized via IUsdWithdrawalOutputBuilder.Token)\n"+
			"%s\n  recipient:           %s\n%s",
		tokenLine, recipient, amountLine,
	), true, nil
}

func decodeUSDAccount(account []byte) (common.Address, *big.Int, error) {
	if len(account) != usdAccountSize {
		return common.Address{}, nil, fmt.Errorf(
			"USD account must be exactly %d bytes, got %d",
			usdAccountSize,
			len(account),
		)
	}

	var balanceBytes [12]byte
	for i := range balanceBytes {
		balanceBytes[len(balanceBytes)-1-i] = account[i]
	}
	balance := new(big.Int).SetBytes(balanceBytes[:])
	var recipient common.Address
	copy(recipient[:], account[12:usdAccountSize])
	return recipient, balance, nil
}

// fetchERC20Metadata best-effort-fetches the symbol and decimals of an
// ERC-20 token. Returns ok=false if either view reverts so the caller can
// fall back to a raw integer display rather than guessing.
func fetchERC20Metadata(
	ctx context.Context,
	client *ethclient.Client,
	token common.Address,
) (symbol string, decimals uint8, ok bool) {
	md, err := ierc20metadata.NewIERC20Metadata(token, client)
	if err != nil {
		return "", 0, false
	}
	opts := &bind.CallOpts{Context: ctx}
	symbol, err = md.Symbol(opts)
	if err != nil {
		return "", 0, false
	}
	decimals, err = md.Decimals(opts)
	if err != nil {
		return "", 0, false
	}
	return symbol, decimals, true
}

// formatTokenAmount converts a raw integer balance into the conventional
// fixed-point string (e.g. balance=1_500_000, decimals=6 → "1.5"). Trailing
// zeros in the fractional part are trimmed so common round amounts render
// compactly.
func formatTokenAmount(raw *big.Int, decimals uint8) string {
	if decimals == 0 {
		return raw.String()
	}
	denom := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	whole, frac := new(big.Int).QuoRem(new(big.Int).Set(raw), denom, new(big.Int))
	if frac.Sign() == 0 {
		return whole.String()
	}
	fracStr := fmt.Sprintf("%0*s", decimals, frac.String())
	for len(fracStr) > 0 && fracStr[len(fracStr)-1] == '0' {
		fracStr = fracStr[:len(fracStr)-1]
	}
	return whole.String() + "." + fracStr
}
