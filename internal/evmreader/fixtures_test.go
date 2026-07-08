// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import (
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iinputbox"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Fixed addresses used across evmreader tests.
var (
	app1Addr      = common.HexToAddress("0x2E663fe9aE92275242406A185AA4fC8174339D3E")
	app2Addr      = common.HexToAddress("0x78c716FDaE477595a820D86D0eFAfe0eE54dF7dB")
	inputBoxAddr  = common.HexToAddress("0xBa3Cf8fB82E43D370117A0b7296f91ED674E94e3")
	consensusAddr = common.HexToAddress("0xdeadbeef")
)

// Test headers — only Number is used by production code.
var (
	header0 = makeHeader(0x11)
	header1 = makeHeader(0x12)
	header2 = makeHeader(0x13)
	header3 = makeHeader(0x33)
)

// Test input events — all target app1.
var (
	inputAddedEvent0 = makeInputEvent(app1Addr, 0, 0x11)
	inputAddedEvent1 = makeInputEvent(app1Addr, 1, 0x12)
	inputAddedEvent2 = makeInputEvent(app1Addr, 2, 0x13)
	inputAddedEvent3 = makeInputEvent(app1Addr, 3, 0x13)
)

// applications defines the two-app setup used by most tests.
// app1: InputBox DA (inputs are read), app2: non-InputBox DA (inputs filtered out).
var applications = []*Application{{
	Name:                 "my-app-1",
	IApplicationAddress:  app1Addr,
	IConsensusAddress:    consensusAddr,
	IInputBoxAddress:     inputBoxAddr,
	DataAvailability:     DataAvailability_InputBox[:],
	Enabled:              true,
	Status:               ApplicationStatus_OK,
	IInputBoxBlock:       0x01,
	EpochLength:          10,
	LastInputCheckBlock:  0x00,
	LastOutputCheckBlock: 0x00,
}, {
	Name:                 "my-app-2",
	IApplicationAddress:  app2Addr,
	IConsensusAddress:    consensusAddr,
	IInputBoxAddress:     inputBoxAddr,
	DataAvailability:     []byte{0x11, 0x32, 0x45, 0x56},
	Enabled:              true,
	Status:               ApplicationStatus_OK,
	IInputBoxBlock:       0x01,
	EpochLength:          10,
	LastInputCheckBlock:  0x00,
	LastOutputCheckBlock: 0x00,
}}

// makeHeader creates a types.Header with the given block number.
func makeHeader(blockNum uint64) types.Header {
	return types.Header{
		Number: new(big.Int).SetUint64(blockNum),
	}
}

// makeInputEvent creates an IInputBoxInputAdded event for testing.
func makeInputEvent(
	appAddr common.Address,
	index uint64,
	blockNum uint64,
) iinputbox.IInputBoxInputAdded {
	return iinputbox.IInputBoxInputAdded{
		AppContract: appAddr,
		Index:       new(big.Int).SetUint64(index),
		Input:       []byte{0xde, 0xad, byte(index & 0xFF)},
		Raw: types.Log{
			BlockNumber: blockNum,
			TxHash:      common.BigToHash(new(big.Int).SetUint64(blockNum*1000 + index)),
			Index:       uint(index),
			BlockHash:   common.BigToHash(new(big.Int).SetUint64(blockNum)),
		},
	}
}
