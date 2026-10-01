// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
)

// The Sling node logs its claim for each epoch it stages (INFO, target
// cartesi_rollups_prt_node::epoch_manager). This is the Sling node's own
// claim, not an oracle value. The line is not a stable interface: the
// manifest records the Dave revision, and a missing line is reported as
// "not tested", never as a match.
var slingStageClaimPattern = regexp.MustCompile(
	`stage tournament result of epoch (\d+) with claim (0x[0-9a-fA-F]{64})`)

type epochClaim struct {
	Epoch      uint64      `json:"epoch"`
	Commitment common.Hash `json:"commitment"`
}

// parseSlingClaims reads Sling stage claims. The same epoch may be logged more
// than once (retries); different values for one epoch are an error.
func parseSlingClaims(r io.Reader) ([]epochClaim, error) {
	claims := map[uint64]common.Hash{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) //nolint:mnd
	for scanner.Scan() {
		match := slingStageClaimPattern.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}
		epoch, err := strconv.ParseUint(match[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid epoch in %q: %w", match[0], err)
		}
		commitment := common.HexToHash(match[2])
		if previous, ok := claims[epoch]; ok && previous != commitment {
			return nil, fmt.Errorf("the Sling log has two claims for epoch %d: %s and %s", epoch, previous, commitment)
		}
		claims[epoch] = commitment
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	result := make([]epochClaim, 0, len(claims))
	for epoch, commitment := range claims {
		result = append(result, epochClaim{Epoch: epoch, Commitment: commitment})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Epoch < result[j].Epoch })
	return result, nil
}

func parseSlingClaimsFile(path string) ([]epochClaim, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return parseSlingClaims(file)
}
