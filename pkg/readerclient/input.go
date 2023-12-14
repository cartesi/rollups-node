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
	// Notices from this particular input
	Notices []Notice `json:"notices"`
	// Vouchers from this particular input
	Vouchers []Voucher `json:"vouchers"`
	// Reports from this particular input
	Reports []Report `json:"reports"`
}

func newInput(
	index int,
	status CompletionStatus,
	msgSender string,
	timestamp string,
	blockNumber string,
	payload string,
	notices []Notice,
	vouchers []Voucher,
	reports []Report,
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
		notices,
		vouchers,
		reports,
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

	var notices []Notice
	var vouchers []Voucher
	var reports []Report

	for _, edge := range resp.Input.Notices.Edges {

		proof, err := newProof(
			edge.Node.Proof.Validity.InputIndexWithinEpoch,
			edge.Node.Proof.Validity.OutputIndexWithinInput,
			edge.Node.Proof.Validity.OutputHashesRootHash,
			edge.Node.Proof.Validity.VouchersEpochRootHash,
			edge.Node.Proof.Validity.NoticesEpochRootHash,
			edge.Node.Proof.Validity.MachineStateHash,
			edge.Node.Proof.Validity.OutputHashInOutputHashesSiblings,
			edge.Node.Proof.Validity.OutputHashesInEpochSiblings,
			edge.Node.Proof.Context,
		)
		if err != nil {
			return nil, err
		}

		notice, err := newNotice(
			edge.Node.Index,
			resp.Input.Index,
			edge.Node.Payload,
			proof,
		)
		if err != nil {
			return nil, err
		}

		notices = append(notices, *notice)
	}

	for _, edge := range resp.Input.Vouchers.Edges {

		proof, err := newProof(
			edge.Node.Proof.Validity.InputIndexWithinEpoch,
			edge.Node.Proof.Validity.OutputIndexWithinInput,
			edge.Node.Proof.Validity.OutputHashesRootHash,
			edge.Node.Proof.Validity.VouchersEpochRootHash,
			edge.Node.Proof.Validity.NoticesEpochRootHash,
			edge.Node.Proof.Validity.MachineStateHash,
			edge.Node.Proof.Validity.OutputHashInOutputHashesSiblings,
			edge.Node.Proof.Validity.OutputHashesInEpochSiblings,
			edge.Node.Proof.Context,
		)
		if err != nil {
			return nil, err
		}

		voucher, err := newVoucher(
			edge.Node.Index,
			resp.Input.Index,
			edge.Node.Destination,
			edge.Node.Payload,
			proof,
		)
		if err != nil {
			return nil, err
		}

		vouchers = append(vouchers, *voucher)
	}

	for _, edge := range resp.Input.Reports.Edges {

		report, err := newReport(
			edge.Node.Index,
			resp.Input.Index,
			edge.Node.Payload,
		)
		if err != nil {
			return nil, err
		}

		reports = append(reports, *report)
	}

	input, err := newInput(
		resp.Input.Index,
		resp.Input.Status,
		resp.Input.MsgSender,
		resp.Input.Timestamp,
		resp.Input.BlockNumber,
		resp.Input.Payload,
		notices,
		vouchers,
		reports,
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

	for _, inputEdge := range resp.Inputs.Edges {

		var notices []Notice
		var vouchers []Voucher
		var reports []Report

		for _, edge := range inputEdge.Node.Notices.Edges {

			proof, err := newProof(
				edge.Node.Proof.Validity.InputIndexWithinEpoch,
				edge.Node.Proof.Validity.OutputIndexWithinInput,
				edge.Node.Proof.Validity.OutputHashesRootHash,
				edge.Node.Proof.Validity.VouchersEpochRootHash,
				edge.Node.Proof.Validity.NoticesEpochRootHash,
				edge.Node.Proof.Validity.MachineStateHash,
				edge.Node.Proof.Validity.OutputHashInOutputHashesSiblings,
				edge.Node.Proof.Validity.OutputHashesInEpochSiblings,
				edge.Node.Proof.Context,
			)
			if err != nil {
				return nil, err
			}

			notice, err := newNotice(
				edge.Node.Index,
				inputEdge.Node.Index,
				edge.Node.Payload,
				proof,
			)
			if err != nil {
				return nil, err
			}

			notices = append(notices, *notice)
		}

		for _, edge := range inputEdge.Node.Vouchers.Edges {

			proof, err := newProof(
				edge.Node.Proof.Validity.InputIndexWithinEpoch,
				edge.Node.Proof.Validity.OutputIndexWithinInput,
				edge.Node.Proof.Validity.OutputHashesRootHash,
				edge.Node.Proof.Validity.VouchersEpochRootHash,
				edge.Node.Proof.Validity.NoticesEpochRootHash,
				edge.Node.Proof.Validity.MachineStateHash,
				edge.Node.Proof.Validity.OutputHashInOutputHashesSiblings,
				edge.Node.Proof.Validity.OutputHashesInEpochSiblings,
				edge.Node.Proof.Context,
			)
			if err != nil {
				return nil, err
			}

			voucher, err := newVoucher(
				edge.Node.Index,
				inputEdge.Node.Index,
				edge.Node.Destination,
				edge.Node.Payload,
				proof,
			)
			if err != nil {
				return nil, err
			}

			vouchers = append(vouchers, *voucher)
		}

		for _, edge := range inputEdge.Node.Reports.Edges {

			report, err := newReport(
				edge.Node.Index,
				inputEdge.Node.Index,
				edge.Node.Payload,
			)
			if err != nil {
				return nil, err
			}

			reports = append(reports, *report)
		}

		input, err := newInput(
			inputEdge.Node.Index,
			inputEdge.Node.Status,
			inputEdge.Node.MsgSender,
			inputEdge.Node.Timestamp,
			inputEdge.Node.BlockNumber,
			inputEdge.Node.Payload,
			notices,
			vouchers,
			reports,
		)
		if err != nil {
			return nil, err
		}

		inputs = append(inputs, *input)
	}

	return inputs, err
}
