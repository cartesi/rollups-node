// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package readerclient

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Input struct {
	// Input index starting from genesis
	Index int `json:"index"`
	// Status of the input
	Status CompletionStatus `json:"status"`
	// Address responsible for submitting the input
	MsgSender common.Address `json:"msgSender"`
	// Timestamp of the input submission, defined by the base layer's block where it was recorded
	Timestamp time.Duration `json:"timestamp"`
	// Number of the base layer block in which the input was recorded
	BlockNumber uint64 `json:"blockNumber"`
	// Input payload in Ethereum hex binary format, starting with '0x'
	Payload hexutil.Bytes `json:"payload"`
}

func newInput(
	index int,
	status CompletionStatus,
	msgSender string,
	timestamp string,
	blockNumber string,
	payload string,
) (*Input, error) {
	convTimestamp, err := strconv.ParseUint(timestamp, 0, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp: %v", err)
	}

	convBlockNumber, err := strconv.ParseUint(blockNumber, 0, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse block number: %v", err)
	}
	convPayload, err := hexutil.Decode(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload to bytes: %v", err)
	}

	input := Input{
		index,
		status,
		common.HexToAddress(msgSender),
		time.Duration(convTimestamp),
		convBlockNumber,
		convPayload,
	}

	return &input, err
}

// GetInput returns the input at index
func GetInput(
	ctx context.Context,
	client graphql.Client,
	index int,
) (*Input, error) {
	resp, err := getInput(ctx, client, index)
	if err != nil {
		return nil, err
	}

	input, err := newInput(
		resp.Input.Index,
		resp.Input.Status,
		resp.Input.MsgSender,
		resp.Input.Timestamp,
		resp.Input.BlockNumber,
		resp.Input.Payload,
	)

	return input, err
}

// GetInputs returns multiple inputs ordered by index
func GetInputs(
	ctx context.Context,
	client graphql.Client,
) ([]Input, error) {

	var inputs []Input

	resp, err := getInputs(ctx, client)
	if err != nil {
		return nil, err
	}

	for _, edge := range resp.Inputs.Edges {

		input, err := newInput(
			edge.Node.Index,
			edge.Node.Status,
			edge.Node.MsgSender,
			edge.Node.Timestamp,
			edge.Node.BlockNumber,
			edge.Node.Payload,
		)
		if err != nil {
			return nil, err
		}

		inputs = append(inputs, *input)
	}

	return inputs, err
}
