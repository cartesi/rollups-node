// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package rollupsmachine

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

type Input struct {
	ChainId        uint64
	AppContract    [20]byte
	Sender         [20]byte
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
	Address [20]byte
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
	case 3:
		voucher := &Voucher{
			Address: [20]byte(arguments[0].(common.Address)),
			Value:   arguments[1].(*big.Int),
			Data:    arguments[2].([]byte),
		}
		return voucher, nil, nil
	default:
		return nil, nil, fmt.Errorf("not an output: len(arguments) == %d, should be 1 or 3", length)
	}
}

var ioABI abi.ABI

func init() {
	json := `[{
        "type" : "function",
        "name" : "EvmAdvance",
        "inputs" : [
            { "type" : "uint256" },
            { "type" : "address" },
            { "type" : "address" },
            { "type" : "uint256" },
            { "type" : "uint256" },
            { "type" : "uint256" },
            { "type" : "bytes"   }
        ]
    }, {
        "type" : "function",
        "name" : "Voucher",
        "inputs" : [
            { "type" : "address" },
            { "type" : "uint256" },
            { "type" : "bytes"   }
        ]
    }, {
        "type" : "function",
        "name" : "Notice",
        "inputs" : [
            { "type" : "bytes"   }
        ]
    }]`

	var err error
	ioABI, err = abi.JSON(strings.NewReader(json))
	if err != nil {
		panic(err)
	}
}
