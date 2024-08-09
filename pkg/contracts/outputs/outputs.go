// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package outputs

import (
	"errors"
	"math/big"
	"strings"

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
)

// OutputsMetaData contains all meta data concerning the Outputs contract.
var OutputsMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"destination\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"payload\",\"type\":\"bytes\"}],\"name\":\"DelegateCallVoucher\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"payload\",\"type\":\"bytes\"}],\"name\":\"Notice\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"destination\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"payload\",\"type\":\"bytes\"}],\"name\":\"Voucher\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// OutputsABI is the input ABI used to generate the binding from.
// Deprecated: Use OutputsMetaData.ABI instead.
var OutputsABI = OutputsMetaData.ABI

// Outputs is an auto generated Go binding around an Ethereum contract.
type Outputs struct {
	OutputsCaller     // Read-only binding to the contract
	OutputsTransactor // Write-only binding to the contract
	OutputsFilterer   // Log filterer for contract events
}

// OutputsCaller is an auto generated read-only Go binding around an Ethereum contract.
type OutputsCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OutputsTransactor is an auto generated write-only Go binding around an Ethereum contract.
type OutputsTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OutputsFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type OutputsFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// OutputsSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type OutputsSession struct {
	Contract     *Outputs          // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// OutputsCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type OutputsCallerSession struct {
	Contract *OutputsCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts  // Call options to use throughout this session
}

// OutputsTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type OutputsTransactorSession struct {
	Contract     *OutputsTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts  // Transaction auth options to use throughout this session
}

// OutputsRaw is an auto generated low-level Go binding around an Ethereum contract.
type OutputsRaw struct {
	Contract *Outputs // Generic contract binding to access the raw methods on
}

// OutputsCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type OutputsCallerRaw struct {
	Contract *OutputsCaller // Generic read-only contract binding to access the raw methods on
}

// OutputsTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type OutputsTransactorRaw struct {
	Contract *OutputsTransactor // Generic write-only contract binding to access the raw methods on
}

// NewOutputs creates a new instance of Outputs, bound to a specific deployed contract.
func NewOutputs(address common.Address, backend bind.ContractBackend) (*Outputs, error) {
	contract, err := bindOutputs(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Outputs{OutputsCaller: OutputsCaller{contract: contract}, OutputsTransactor: OutputsTransactor{contract: contract}, OutputsFilterer: OutputsFilterer{contract: contract}}, nil
}

// NewOutputsCaller creates a new read-only instance of Outputs, bound to a specific deployed contract.
func NewOutputsCaller(address common.Address, caller bind.ContractCaller) (*OutputsCaller, error) {
	contract, err := bindOutputs(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &OutputsCaller{contract: contract}, nil
}

// NewOutputsTransactor creates a new write-only instance of Outputs, bound to a specific deployed contract.
func NewOutputsTransactor(address common.Address, transactor bind.ContractTransactor) (*OutputsTransactor, error) {
	contract, err := bindOutputs(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &OutputsTransactor{contract: contract}, nil
}

// NewOutputsFilterer creates a new log filterer instance of Outputs, bound to a specific deployed contract.
func NewOutputsFilterer(address common.Address, filterer bind.ContractFilterer) (*OutputsFilterer, error) {
	contract, err := bindOutputs(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &OutputsFilterer{contract: contract}, nil
}

// bindOutputs binds a generic wrapper to an already deployed contract.
func bindOutputs(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := OutputsMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Outputs *OutputsRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Outputs.Contract.OutputsCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Outputs *OutputsRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Outputs.Contract.OutputsTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Outputs *OutputsRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Outputs.Contract.OutputsTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Outputs *OutputsCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Outputs.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Outputs *OutputsTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Outputs.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Outputs *OutputsTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Outputs.Contract.contract.Transact(opts, method, params...)
}

// DelegateCallVoucher is a paid mutator transaction binding the contract method 0x10321e8b.
//
// Solidity: function DelegateCallVoucher(address destination, bytes payload) returns()
func (_Outputs *OutputsTransactor) DelegateCallVoucher(opts *bind.TransactOpts, destination common.Address, payload []byte) (*types.Transaction, error) {
	return _Outputs.contract.Transact(opts, "DelegateCallVoucher", destination, payload)
}

// DelegateCallVoucher is a paid mutator transaction binding the contract method 0x10321e8b.
//
// Solidity: function DelegateCallVoucher(address destination, bytes payload) returns()
func (_Outputs *OutputsSession) DelegateCallVoucher(destination common.Address, payload []byte) (*types.Transaction, error) {
	return _Outputs.Contract.DelegateCallVoucher(&_Outputs.TransactOpts, destination, payload)
}

// DelegateCallVoucher is a paid mutator transaction binding the contract method 0x10321e8b.
//
// Solidity: function DelegateCallVoucher(address destination, bytes payload) returns()
func (_Outputs *OutputsTransactorSession) DelegateCallVoucher(destination common.Address, payload []byte) (*types.Transaction, error) {
	return _Outputs.Contract.DelegateCallVoucher(&_Outputs.TransactOpts, destination, payload)
}

// Notice is a paid mutator transaction binding the contract method 0xc258d6e5.
//
// Solidity: function Notice(bytes payload) returns()
func (_Outputs *OutputsTransactor) Notice(opts *bind.TransactOpts, payload []byte) (*types.Transaction, error) {
	return _Outputs.contract.Transact(opts, "Notice", payload)
}

// Notice is a paid mutator transaction binding the contract method 0xc258d6e5.
//
// Solidity: function Notice(bytes payload) returns()
func (_Outputs *OutputsSession) Notice(payload []byte) (*types.Transaction, error) {
	return _Outputs.Contract.Notice(&_Outputs.TransactOpts, payload)
}

// Notice is a paid mutator transaction binding the contract method 0xc258d6e5.
//
// Solidity: function Notice(bytes payload) returns()
func (_Outputs *OutputsTransactorSession) Notice(payload []byte) (*types.Transaction, error) {
	return _Outputs.Contract.Notice(&_Outputs.TransactOpts, payload)
}

// Voucher is a paid mutator transaction binding the contract method 0x237a816f.
//
// Solidity: function Voucher(address destination, uint256 value, bytes payload) returns()
func (_Outputs *OutputsTransactor) Voucher(opts *bind.TransactOpts, destination common.Address, value *big.Int, payload []byte) (*types.Transaction, error) {
	return _Outputs.contract.Transact(opts, "Voucher", destination, value, payload)
}

// Voucher is a paid mutator transaction binding the contract method 0x237a816f.
//
// Solidity: function Voucher(address destination, uint256 value, bytes payload) returns()
func (_Outputs *OutputsSession) Voucher(destination common.Address, value *big.Int, payload []byte) (*types.Transaction, error) {
	return _Outputs.Contract.Voucher(&_Outputs.TransactOpts, destination, value, payload)
}

// Voucher is a paid mutator transaction binding the contract method 0x237a816f.
//
// Solidity: function Voucher(address destination, uint256 value, bytes payload) returns()
func (_Outputs *OutputsTransactorSession) Voucher(destination common.Address, value *big.Int, payload []byte) (*types.Transaction, error) {
	return _Outputs.Contract.Voucher(&_Outputs.TransactOpts, destination, value, payload)
}
