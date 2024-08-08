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

// An Input is sent by a advance-state request.
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

// A Query is sent by a inspect-state request.
type Query struct {
	Data []byte
}

// A Voucher is a type of machine output.
type Voucher struct {
	Address Address
	Value   *big.Int
	Data    []byte
}

// A Notice is a type of machine output.
type Notice struct {
	Data []byte
}

// Encode encodes an input.
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

// DecodeOutput decodes an output into either a voucher or a notice.
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

// ------------------------------------------------------------------------------------------------

func decodeArguments(payload []byte) (arguments []any, _ error) {
	method, err := ioABI.MethodById(payload)
	if err != nil {
		return nil, err
	}

	return method.Inputs.Unpack(payload[4:])
}
