// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contract

import (
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type printer struct {
	w       io.Writer
	indent  int
	started bool
}

// withSection runs fn within an indented section, guaranteeing endSection is always called.
func (p *printer) withSection(title string, fn func()) {
	if p.started {
		fmt.Fprintf(p.w, "\n")
	}
	p.started = true
	fmt.Fprintf(p.w, "%s%s\n", strings.Repeat("\t", p.indent), title)
	p.indent++
	fn()
	p.indent--
}

func (p *printer) field(name, value string) {
	padding := strings.Repeat(" ", max(1, 24-len(name))) //nolint:mnd
	fmt.Fprintf(p.w, "%s%s%s%s\n", strings.Repeat("\t", p.indent), name, padding, value)
}

func (p *printer) fieldErr(label string, err error) {
	p.field("ERROR", fmt.Sprintf("%s: %v", label, err))
}

// footer prints the block, chain ID, and a disclaimer at the end of every output.
func (p *printer) footer(blockNum, chainID uint64, blockTime ...uint64) {
	ts := uint64(0)
	if len(blockTime) > 0 {
		ts = blockTime[0]
	}
	fmt.Fprintf(p.w, "\nState at block            %d (chain ID: %d)%s\n",
		blockNum, chainID, formatBlockTime(ts))
	fmt.Fprintf(p.w, "\nNote: this is an experimental diagnostic tool. "+
		"Values are read directly from on-chain contracts\n"+
		"and may not reflect finalized state. Verify critical information independently.\n")
}

func formatAddr(addr common.Address) string {
	return addr.Hex() // always full: 0x + 40 hex chars
}

func formatHash(hash [32]byte) string {
	h := common.BytesToHash(hash[:])
	return h.Hex() // always full: 0x + 64 hex chars
}

var weiDivisor = new(big.Float).SetInt(
	new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil), //nolint:mnd
)

func weiToETH(wei *big.Int) string {
	eth := new(big.Float).SetInt(wei)
	eth.Quo(eth, weiDivisor)
	return fmt.Sprintf("%s ETH", eth.Text('f', 6)) //nolint:mnd
}

func tournamentStatus(closed, finished bool) string {
	switch {
	case finished:
		return "FINISHED"
	case closed:
		return "CLOSED (matches still running)"
	default:
		return "OPEN"
	}
}

func matchDeletionReason(reason uint8) string {
	switch reason {
	case 0:
		return "STEP (on-chain proof)"
	case 1:
		return "TIMEOUT"
	case 2: //nolint:mnd
		return "CHILD_TOURNAMENT"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", reason)
	}
}

func matchWinner(winner uint8) string {
	switch winner {
	case 0:
		return "NONE (both eliminated)"
	case 1:
		return "ONE"
	case 2: //nolint:mnd
		return "TWO"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", winner)
	}
}

// formatBlockTime returns " (2006-01-02 15:04:05 UTC)" if ts > 0, else "".
func formatBlockTime(ts uint64) string {
	if ts == 0 {
		return ""
	}
	t := time.Unix(int64(ts), 0).UTC() //nolint:gosec // timestamp won't overflow int64
	return fmt.Sprintf(" (%s)", t.Format("2006-01-02 15:04:05 UTC"))
}
