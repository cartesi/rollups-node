// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package dataavailability

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
	_ = time.Tick
	_ = context.Background
)

// DataAvailabilityMetaData contains all meta data concerning the DataAvailability contract.
var DataAvailabilityMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"InputBox\",\"inputs\":[{\"name\":\"inputBox\",\"type\":\"address\",\"internalType\":\"contractIInputBox\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"InputBoxAndEspresso\",\"inputs\":[{\"name\":\"inputBox\",\"type\":\"address\",\"internalType\":\"contractIInputBox\"},{\"name\":\"fromBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"namespaceId\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"}]",
}

// DataAvailabilityABI is the input ABI used to generate the binding from.
// Deprecated: Use DataAvailabilityMetaData.ABI instead.
var DataAvailabilityABI = DataAvailabilityMetaData.ABI

// DataAvailability is an auto generated Go binding around an Ethereum contract.
type DataAvailability struct {
	DataAvailabilityCaller     // Read-only binding to the contract
	DataAvailabilityTransactor // Write-only binding to the contract
	DataAvailabilityFilterer   // Log filterer for contract events
}

// DataAvailabilityCaller is an auto generated read-only Go binding around an Ethereum contract.
type DataAvailabilityCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DataAvailabilityTransactor is an auto generated write-only Go binding around an Ethereum contract.
type DataAvailabilityTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DataAvailabilityFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type DataAvailabilityFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DataAvailabilitySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type DataAvailabilitySession struct {
	Contract     *DataAvailability // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// DataAvailabilityCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type DataAvailabilityCallerSession struct {
	Contract *DataAvailabilityCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts           // Call options to use throughout this session
}

// DataAvailabilityTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type DataAvailabilityTransactorSession struct {
	Contract     *DataAvailabilityTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts           // Transaction auth options to use throughout this session
}

// DataAvailabilityRaw is an auto generated low-level Go binding around an Ethereum contract.
type DataAvailabilityRaw struct {
	Contract *DataAvailability // Generic contract binding to access the raw methods on
}

// DataAvailabilityCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type DataAvailabilityCallerRaw struct {
	Contract *DataAvailabilityCaller // Generic read-only contract binding to access the raw methods on
}

// DataAvailabilityTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type DataAvailabilityTransactorRaw struct {
	Contract *DataAvailabilityTransactor // Generic write-only contract binding to access the raw methods on
}

// NewDataAvailability creates a new instance of DataAvailability, bound to a specific deployed contract.
func NewDataAvailability(address common.Address, backend bind.ContractBackend) (*DataAvailability, error) {
	contract, err := bindDataAvailability(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &DataAvailability{DataAvailabilityCaller: DataAvailabilityCaller{contract: contract}, DataAvailabilityTransactor: DataAvailabilityTransactor{contract: contract}, DataAvailabilityFilterer: DataAvailabilityFilterer{contract: contract}}, nil
}

// NewDataAvailabilityCaller creates a new read-only instance of DataAvailability, bound to a specific deployed contract.
func NewDataAvailabilityCaller(address common.Address, caller bind.ContractCaller) (*DataAvailabilityCaller, error) {
	contract, err := bindDataAvailability(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &DataAvailabilityCaller{contract: contract}, nil
}

// NewDataAvailabilityTransactor creates a new write-only instance of DataAvailability, bound to a specific deployed contract.
func NewDataAvailabilityTransactor(address common.Address, transactor bind.ContractTransactor) (*DataAvailabilityTransactor, error) {
	contract, err := bindDataAvailability(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &DataAvailabilityTransactor{contract: contract}, nil
}

// NewDataAvailabilityFilterer creates a new log filterer instance of DataAvailability, bound to a specific deployed contract.
func NewDataAvailabilityFilterer(address common.Address, filterer bind.ContractFilterer) (*DataAvailabilityFilterer, error) {
	contract, err := bindDataAvailability(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &DataAvailabilityFilterer{contract: contract}, nil
}

// bindDataAvailability binds a generic wrapper to an already deployed contract.
func bindDataAvailability(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := DataAvailabilityMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_DataAvailability *DataAvailabilityRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _DataAvailability.Contract.DataAvailabilityCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_DataAvailability *DataAvailabilityRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DataAvailability.Contract.DataAvailabilityTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_DataAvailability *DataAvailabilityRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _DataAvailability.Contract.DataAvailabilityTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_DataAvailability *DataAvailabilityCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _DataAvailability.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_DataAvailability *DataAvailabilityTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DataAvailability.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_DataAvailability *DataAvailabilityTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _DataAvailability.Contract.contract.Transact(opts, method, params...)
}

// InputBox is a paid mutator transaction binding the contract method 0xb12c9ede.
//
// Solidity: function InputBox(address inputBox) returns()
func (_DataAvailability *DataAvailabilityTransactor) InputBox(opts *bind.TransactOpts, inputBox common.Address) (*types.Transaction, error) {
	return _DataAvailability.contract.Transact(opts, "InputBox", inputBox)
}

// InputBox is a paid mutator transaction binding the contract method 0xb12c9ede.
//
// Solidity: function InputBox(address inputBox) returns()
func (_DataAvailability *DataAvailabilitySession) InputBox(inputBox common.Address) (*types.Transaction, error) {
	return _DataAvailability.Contract.InputBox(&_DataAvailability.TransactOpts, inputBox)
}

// InputBox is a paid mutator transaction binding the contract method 0xb12c9ede.
//
// Solidity: function InputBox(address inputBox) returns()
func (_DataAvailability *DataAvailabilityTransactorSession) InputBox(inputBox common.Address) (*types.Transaction, error) {
	return _DataAvailability.Contract.InputBox(&_DataAvailability.TransactOpts, inputBox)
}

// InputBoxAndEspresso is a paid mutator transaction binding the contract method 0x8579fd0c.
//
// Solidity: function InputBoxAndEspresso(address inputBox, uint256 fromBlock, uint32 namespaceId) returns()
func (_DataAvailability *DataAvailabilityTransactor) InputBoxAndEspresso(opts *bind.TransactOpts, inputBox common.Address, fromBlock *big.Int, namespaceId uint32) (*types.Transaction, error) {
	return _DataAvailability.contract.Transact(opts, "InputBoxAndEspresso", inputBox, fromBlock, namespaceId)
}

// InputBoxAndEspresso is a paid mutator transaction binding the contract method 0x8579fd0c.
//
// Solidity: function InputBoxAndEspresso(address inputBox, uint256 fromBlock, uint32 namespaceId) returns()
func (_DataAvailability *DataAvailabilitySession) InputBoxAndEspresso(inputBox common.Address, fromBlock *big.Int, namespaceId uint32) (*types.Transaction, error) {
	return _DataAvailability.Contract.InputBoxAndEspresso(&_DataAvailability.TransactOpts, inputBox, fromBlock, namespaceId)
}

// InputBoxAndEspresso is a paid mutator transaction binding the contract method 0x8579fd0c.
//
// Solidity: function InputBoxAndEspresso(address inputBox, uint256 fromBlock, uint32 namespaceId) returns()
func (_DataAvailability *DataAvailabilityTransactorSession) InputBoxAndEspresso(inputBox common.Address, fromBlock *big.Int, namespaceId uint32) (*types.Transaction, error) {
	return _DataAvailability.Contract.InputBoxAndEspresso(&_DataAvailability.TransactOpts, inputBox, fromBlock, namespaceId)
}
