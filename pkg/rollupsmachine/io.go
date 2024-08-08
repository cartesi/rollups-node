// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package rollupsmachine

import (
	_ "embed"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

var (
	//go:embed abi.json
	jsonABI string

	ioABI abi.ABI
)

func init() {
	var err error
	ioABI, err = abi.JSON(strings.NewReader(jsonABI))
	if err != nil {
		panic(err)
	}
}

type Input struct {
	ChainId        uint64
	AppContract    Address
	Sender         Address
	BlockNumber    uint64
	BlockTimestamp uint64
	// PrevRandao     uint64
	Index uint64
	Data  []byte
}

type Query struct {
	Data []byte
}

type Voucher struct {
	Address Address
	Value   *big.Int
	Data    []byte
}

type Notice struct {
	Data []byte
}

func (input Input) Encode() ([]byte, error) {
	chainId := new(big.Int).SetUint64(input.ChainId)
	appContract := common.BytesToAddress(input.AppContract[:])
	sender := common.BytesToAddress(input.Sender[:])
	blockNumber := new(big.Int).SetUint64(input.BlockNumber)
	blockTimestamp := new(big.Int).SetUint64(input.BlockTimestamp)
	// prevRandao := new(big.Int).SetUint64(input.PrevRandao)
	index := new(big.Int).SetUint64(input.Index)
	return ioABI.Pack("EvmAdvance", chainId, appContract, sender, blockNumber, blockTimestamp,
		index, input.Data)
}

func (query Query) Encode() ([]byte, error) {
	return query.Data, nil
}

func decodeArguments(payload []byte) (arguments []any, _ error) {
	method, err := ioABI.MethodById(payload)
	if err != nil {
		return nil, err
	}

	return method.Inputs.Unpack(payload[4:])
}

func DecodeOutput(payload []byte) (*Voucher, *Notice, error) {
	arguments, err := decodeArguments(payload)
	if err != nil {
		return nil, nil, err
	}

	switch length := len(arguments); length {
	case 1:
		notice := &Notice{Data: arguments[0].([]byte)}
		return nil, notice, nil
	case 3: //nolint:mnd
		voucher := &Voucher{
			Address: Address(arguments[0].(common.Address)),
			Value:   arguments[1].(*big.Int),
			Data:    arguments[2].([]byte),
		}
		return voucher, nil, nil
	default:
		return nil, nil, fmt.Errorf("not an output: len(arguments) == %d, should be 1 or 3", length)
	}
}
