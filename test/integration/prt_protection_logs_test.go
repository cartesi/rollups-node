// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

const (
	prtStoredClockSource = "stored root descriptor"
	prtPinnedClockSource = "pinned root descriptor"
)

// Stored metadata can race the direct stop. The observer must recover the
// exact clock from a pinned contract descriptor in a later tick.
func prtProtectionStopLog(application string, address common.Address, epoch uint64,
	root common.Address, closeBlock uint64, cause, source string,
) ExpectedLog {
	clock := fmt.Sprintf(`clock_available=true join_close_block=%d`, closeBlock)
	sourcePattern := regexp.QuoteMeta(source)
	switch source {
	case prtStoredClockSource:
		clock = `(?:` + clock + `|clock_available=false clock_error="root descriptor is not stored")`
	case prtPinnedClockSource:
		sourcePattern += ` at block [0-9]+ \(0x[[:xdigit:]]{64}\)`
	}
	return ExpectedLog{
		Pattern: regexp.MustCompile(`PRT protection stopped .*application=` + regexp.QuoteMeta(application) +
			` address=` + regexp.QuoteMeta(strings.ToLower(address.Hex())) + fmt.Sprintf(` epoch_index=%d root_tournament=`, epoch) +
			regexp.QuoteMeta(strings.ToLower(root.Hex())) + ` cause="[^"]*` + regexp.QuoteMeta(cause) +
			`" clock_source="` + sourcePattern + `" ` + clock + `$`),
		Level: LevelError, Required: true,
		Reason: "the automatic stop identifies this epoch, root, cause, clock source, and exclusive join deadline",
	}
}
