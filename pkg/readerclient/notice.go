// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package readerclient

import (
	"context"
	"fmt"

	"github.com/Khan/genqlient/graphql"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Notice struct {
	// Notice index within the context of the input that produced it
	Index int `json:"index"`
	// Input whose processing produced the notice
	InputIndex int `json:"inputIndex"`
	// Notice data as a payload in Ethereum hex binary format, starting with '0x'
	Payload hexutil.Bytes `json:"payload"`
	// Proof object that allows this notice to be validated by the base layer blockchain
	Proof Proof `json:"proof"`
}

func newNotice(
	index int,
	inputIndex int,
	payload string,
	proof Proof,
) (*Notice, error) {
	convPayload, err := hexutil.Decode(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload to bytes: %v", err)
	}

	notice := Notice{
		index,
		inputIndex,
		convPayload,
		proof,
	}

	return &notice, err
}

// Get multiple notices from graphql.
func GetNotices(
	ctx context.Context,
	client graphql.Client,
) ([]Notice, error) {

	var notices []Notice

	resp, err := getNotices(ctx, client)
	if err != nil {
		return nil, err
	}

	for _, edge := range resp.Notices.Edges {

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
			edge.Node.Input.Index,
			edge.Node.Payload,
			proof,
		)
		if err != nil {
			return nil, err
		}

		notices = append(notices, *notice)
	}

	return notices, err
}

// Get multiple notices from GraphQL for the given input index.
func GetInputNotices(
	ctx context.Context,
	client graphql.Client,
	inputIndex int,
) ([]Notice, error) {

	var notices []Notice

	resp, err := getInputNotices(ctx, client, inputIndex)
	if err != nil {
		return nil, err
	}

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

	return notices, err
}

// Get notice from GraphQL given the input and notice indices.
func GetNotice(
	ctx context.Context,
	client graphql.Client,
	noticeIndex int,
	inputIndex int,
) (*Notice, error) {
	resp, err := getNotice(ctx, client, noticeIndex, inputIndex)
	if err != nil {
		return nil, err
	}

	proof, err := newProof(
		resp.Notice.Proof.Validity.InputIndexWithinEpoch,
		resp.Notice.Proof.Validity.OutputIndexWithinInput,
		resp.Notice.Proof.Validity.OutputHashesRootHash,
		resp.Notice.Proof.Validity.VouchersEpochRootHash,
		resp.Notice.Proof.Validity.NoticesEpochRootHash,
		resp.Notice.Proof.Validity.MachineStateHash,
		resp.Notice.Proof.Validity.OutputHashInOutputHashesSiblings,
		resp.Notice.Proof.Validity.OutputHashesInEpochSiblings,
		resp.Notice.Proof.Context,
	)

	notice, err := newNotice(
		resp.Notice.Index,
		resp.Notice.Input.Index,
		resp.Notice.Payload,
		*proof,
	)

	return notice, err
}
