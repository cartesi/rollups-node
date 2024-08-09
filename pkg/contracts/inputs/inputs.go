// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package inputs

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

// InputsMetaData contains all meta data concerning the Inputs contract.
var InputsMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"chainId\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"appContract\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"msgSender\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"blockNumber\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"blockTimestamp\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"prevRandao\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"index\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"payload\",\"type\":\"bytes\"}],\"name\":\"EvmAdvance\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// InputsABI is the input ABI used to generate the binding from.
// Deprecated: Use InputsMetaData.ABI instead.
var InputsABI = InputsMetaData.ABI

// Inputs is an auto generated Go binding around an Ethereum contract.
type Inputs struct {
	InputsCaller     // Read-only binding to the contract
	InputsTransactor // Write-only binding to the contract
	InputsFilterer   // Log filterer for contract events
}

// InputsCaller is an auto generated read-only Go binding around an Ethereum contract.
type InputsCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// InputsTransactor is an auto generated write-only Go binding around an Ethereum contract.
type InputsTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// InputsFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type InputsFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// InputsSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type InputsSession struct {
	Contract     *Inputs           // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// InputsCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type InputsCallerSession struct {
	Contract *InputsCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts // Call options to use throughout this session
}

// InputsTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type InputsTransactorSession struct {
	Contract     *InputsTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// InputsRaw is an auto generated low-level Go binding around an Ethereum contract.
type InputsRaw struct {
	Contract *Inputs // Generic contract binding to access the raw methods on
}

// InputsCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type InputsCallerRaw struct {
	Contract *InputsCaller // Generic read-only contract binding to access the raw methods on
}

// InputsTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type InputsTransactorRaw struct {
	Contract *InputsTransactor // Generic write-only contract binding to access the raw methods on
}

// NewInputs creates a new instance of Inputs, bound to a specific deployed contract.
func NewInputs(address common.Address, backend bind.ContractBackend) (*Inputs, error) {
	contract, err := bindInputs(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Inputs{InputsCaller: InputsCaller{contract: contract}, InputsTransactor: InputsTransactor{contract: contract}, InputsFilterer: InputsFilterer{contract: contract}}, nil
}

// NewInputsCaller creates a new read-only instance of Inputs, bound to a specific deployed contract.
func NewInputsCaller(address common.Address, caller bind.ContractCaller) (*InputsCaller, error) {
	contract, err := bindInputs(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &InputsCaller{contract: contract}, nil
}

// NewInputsTransactor creates a new write-only instance of Inputs, bound to a specific deployed contract.
func NewInputsTransactor(address common.Address, transactor bind.ContractTransactor) (*InputsTransactor, error) {
	contract, err := bindInputs(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &InputsTransactor{contract: contract}, nil
}

// NewInputsFilterer creates a new log filterer instance of Inputs, bound to a specific deployed contract.
func NewInputsFilterer(address common.Address, filterer bind.ContractFilterer) (*InputsFilterer, error) {
	contract, err := bindInputs(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &InputsFilterer{contract: contract}, nil
}

// bindInputs binds a generic wrapper to an already deployed contract.
func bindInputs(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := InputsMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Inputs *InputsRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Inputs.Contract.InputsCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Inputs *InputsRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Inputs.Contract.InputsTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Inputs *InputsRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Inputs.Contract.InputsTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Inputs *InputsCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Inputs.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Inputs *InputsTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Inputs.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Inputs *InputsTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Inputs.Contract.contract.Transact(opts, method, params...)
}

// EvmAdvance is a paid mutator transaction binding the contract method 0x415bf363.
//
// Solidity: function EvmAdvance(uint256 chainId, address appContract, address msgSender, uint256 blockNumber, uint256 blockTimestamp, uint256 prevRandao, uint256 index, bytes payload) returns()
func (_Inputs *InputsTransactor) EvmAdvance(opts *bind.TransactOpts, chainId *big.Int, appContract common.Address, msgSender common.Address, blockNumber *big.Int, blockTimestamp *big.Int, prevRandao *big.Int, index *big.Int, payload []byte) (*types.Transaction, error) {
	return _Inputs.contract.Transact(opts, "EvmAdvance", chainId, appContract, msgSender, blockNumber, blockTimestamp, prevRandao, index, payload)
}

// EvmAdvance is a paid mutator transaction binding the contract method 0x415bf363.
//
// Solidity: function EvmAdvance(uint256 chainId, address appContract, address msgSender, uint256 blockNumber, uint256 blockTimestamp, uint256 prevRandao, uint256 index, bytes payload) returns()
func (_Inputs *InputsSession) EvmAdvance(chainId *big.Int, appContract common.Address, msgSender common.Address, blockNumber *big.Int, blockTimestamp *big.Int, prevRandao *big.Int, index *big.Int, payload []byte) (*types.Transaction, error) {
	return _Inputs.Contract.EvmAdvance(&_Inputs.TransactOpts, chainId, appContract, msgSender, blockNumber, blockTimestamp, prevRandao, index, payload)
}

// EvmAdvance is a paid mutator transaction binding the contract method 0x415bf363.
//
// Solidity: function EvmAdvance(uint256 chainId, address appContract, address msgSender, uint256 blockNumber, uint256 blockTimestamp, uint256 prevRandao, uint256 index, bytes payload) returns()
func (_Inputs *InputsTransactorSession) EvmAdvance(chainId *big.Int, appContract common.Address, msgSender common.Address, blockNumber *big.Int, blockTimestamp *big.Int, prevRandao *big.Int, index *big.Int, payload []byte) (*types.Transaction, error) {
	return _Inputs.Contract.EvmAdvance(&_Inputs.TransactOpts, chainId, appContract, msgSender, blockNumber, blockTimestamp, prevRandao, index, payload)
}
