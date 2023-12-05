// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package readerclient

import (
	"context"
	"fmt"

	"github.com/Khan/genqlient/graphql"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Voucher struct {
	// Voucher index within the context of the input that produced it
	Index int `json:"index"`
	// Input whose processing produced the voucher
	InputIndex int `json:"inputIndex"`
	// Transaction destination address in Ethereum hex binary format (20 bytes), starting with '0x'
	Destination common.Address `json:"destination"`
	// Voucher data as a payload in Ethereum hex binary format, starting with '0x'
	Payload hexutil.Bytes `json:"payload"`
	// Proof object that allows this voucher to be validated by the base layer blockchain
	Proof *Proof `json:"proof"`
}

func newVoucher(
	index int,
	inputIndex int,
	destination string,
	payload string,
	proof *Proof,
) (*Voucher, error) {
	convPayload, err := hexutil.Decode(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload to bytes: %v", err)
	}

	voucher := Voucher{
		index,
		inputIndex,
		common.HexToAddress(destination),
		convPayload,
		proof,
	}

	return &voucher, err
}

// Get multiple vouchers from graphql.
func GetVouchers(
	ctx context.Context,
	client graphql.Client,
) ([]Voucher, error) {

	var vouchers []Voucher

	resp, err := getVouchers(ctx, client)
	if err != nil {
		return nil, err
	}

	for _, edge := range resp.Vouchers.Edges {

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
			edge.Node.Input.Index,
			edge.Node.Destination,
			edge.Node.Payload,
			proof,
		)
		if err != nil {
			return nil, err
		}

		vouchers = append(vouchers, *voucher)
	}

	return vouchers, err
}

// Get multiple vouchers from GraphQL for the given input index.
func GetInputVouchers(
	ctx context.Context,
	client graphql.Client,
	inputIndex int,
) ([]Voucher, error) {

	var vouchers []Voucher

	resp, err := getInputVouchers(ctx, client, inputIndex)
	if err != nil {
		return nil, err
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

	return vouchers, err
}

// Get voucher from GraphQL given the input and voucher indices.
func GetVoucher(
	ctx context.Context,
	client graphql.Client,
	voucherIndex int,
	inputIndex int,
) (*Voucher, error) {
	resp, err := getVoucher(ctx, client, voucherIndex, inputIndex)
	if err != nil {
		return nil, err
	}

	proof, err := newProof(
		resp.Voucher.Proof.Validity.InputIndexWithinEpoch,
		resp.Voucher.Proof.Validity.OutputIndexWithinInput,
		resp.Voucher.Proof.Validity.OutputHashesRootHash,
		resp.Voucher.Proof.Validity.VouchersEpochRootHash,
		resp.Voucher.Proof.Validity.NoticesEpochRootHash,
		resp.Voucher.Proof.Validity.MachineStateHash,
		resp.Voucher.Proof.Validity.OutputHashInOutputHashesSiblings,
		resp.Voucher.Proof.Validity.OutputHashesInEpochSiblings,
		resp.Voucher.Proof.Context,
	)
	if err != nil {
		return nil, err
	}

	voucher, err := newVoucher(
		resp.Voucher.Index,
		resp.Voucher.Input.Index,
		resp.Voucher.Destination,
		resp.Voucher.Payload,
		proof,
	)

	return voucher, err
}
