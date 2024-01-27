// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package machine

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

type Input struct {
	Sender         [20]byte
	BlockNumber    uint64
	BlockTimestamp uint64
	Index          uint64
	Data           []byte
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
	address := common.BytesToAddress(input.Sender[:])
	blockNumber := new(big.Int).SetUint64(input.BlockNumber)
	blockTimestamp := new(big.Int).SetUint64(input.BlockTimestamp)
	index := new(big.Int).SetUint64(input.Index)
	return ioABI.Pack("EvmAdvance", address, blockNumber, blockTimestamp, index, input.Data)
}

func (query Query) Encode() ([]byte, error) {
	return ioABI.Pack("EvmInspect", query.Data)
}

func Decode(payload []byte) (*Voucher, *Notice, error) {
	method, err := ioABI.MethodById(payload)
	if err != nil {
		return nil, nil, err
	}

	arguments, err := method.Inputs.Unpack(payload[4:])
	if err != nil {
		return nil, nil, err
	}

	switch len(arguments) {
	case 1:
		data := arguments[0].([]byte)
		return nil, &Notice{Data: data}, nil
	case 3:
		voucher := &Voucher{
			Address: [20]byte(arguments[0].([]byte)),
			Data:    arguments[2].([]byte),
		}
		voucher.Value.SetBytes(arguments[1].([]byte))
		return voucher, nil, nil
	default:
		panic(unreachable)
	}
}

var ioABI abi.ABI

func init() {
	json := `[{
        "type" : "function",
        "name" : "EvmAdvance",
        "inputs" : [
            { "type" : "address" },
            { "type" : "uint256" },
            { "type" : "uint256" },
            { "type" : "uint256" },
            { "type" : "bytes"   }
        ]
    }, {
        "type" : "function",
        "name" : "EvmInspect",
        "inputs" : [
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
