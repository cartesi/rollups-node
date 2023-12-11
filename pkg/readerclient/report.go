// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package readerclient

import (
	"context"
	"fmt"

	"github.com/Khan/genqlient/graphql"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Report struct {
	// Report index within the context of the input that produced it
	Index int `json:"index"`
	// Input whose processing produced the report
	InputIndex int `json:"inputIndex"`
	// Report data as a payload in Ethereum hex binary format, starting with '0x'
	Payload hexutil.Bytes `json:"payload"`
}

func newReport(
	index int,
	inputIndex int,
	payload string,
) (*Report, error) {
	convPayload, err := hexutil.Decode(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload to bytes: %v", err)
	}

	report := Report{
		index,
		inputIndex,
		convPayload,
	}

	return &report, err
}

// Get report from GraphQL given the input and report indices.
func GetReport(
	ctx context.Context,
	client graphql.Client,
	reportIndex int,
	inputIndex int,
) (*Report, error) {
	resp, err := getReport(ctx, client, reportIndex, inputIndex)
	if err != nil {
		return nil, err
	}

	report, err := newReport(
		resp.Report.Index,
		resp.Report.Input.Index,
		resp.Report.Payload,
	)

	return report, err
}
