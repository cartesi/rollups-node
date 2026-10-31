// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package cartesistatetransition

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

// CartesiStateTransitionMetaData contains all meta data concerning the CartesiStateTransition contract.
var CartesiStateTransitionMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"CM_MARCHID\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"UARCH_PRISTINE_STATE_HASH\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"transitionState\",\"inputs\":[{\"name\":\"machineState\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"counter\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"proofs\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"provider\",\"type\":\"address\",\"internalType\":\"contractIDataProvider\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"error\",\"name\":\"CounterOutsideEpoch\",\"inputs\":[{\"name\":\"counter\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InputBytesWithoutInput\",\"inputs\":[{\"name\":\"inputLength\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]},{\"type\":\"error\",\"name\":\"InvalidAccessLogProofLength\",\"inputs\":[{\"name\":\"consumed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"provided\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"SafeCastOverflowedUintDowncast\",\"inputs\":[{\"name\":\"bits\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"value\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]",
}

// CartesiStateTransitionABI is the input ABI used to generate the binding from.
// Deprecated: Use CartesiStateTransitionMetaData.ABI instead.
var CartesiStateTransitionABI = CartesiStateTransitionMetaData.ABI

// CartesiStateTransition is an auto generated Go binding around an Ethereum contract.
type CartesiStateTransition struct {
	CartesiStateTransitionCaller     // Read-only binding to the contract
	CartesiStateTransitionTransactor // Write-only binding to the contract
	CartesiStateTransitionFilterer   // Log filterer for contract events
}

// CartesiStateTransitionCaller is an auto generated read-only Go binding around an Ethereum contract.
type CartesiStateTransitionCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CartesiStateTransitionTransactor is an auto generated write-only Go binding around an Ethereum contract.
type CartesiStateTransitionTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CartesiStateTransitionFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type CartesiStateTransitionFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CartesiStateTransitionSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type CartesiStateTransitionSession struct {
	Contract     *CartesiStateTransition // Generic contract binding to set the session for
	CallOpts     bind.CallOpts           // Call options to use throughout this session
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// CartesiStateTransitionCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type CartesiStateTransitionCallerSession struct {
	Contract *CartesiStateTransitionCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                 // Call options to use throughout this session
}

// CartesiStateTransitionTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type CartesiStateTransitionTransactorSession struct {
	Contract     *CartesiStateTransitionTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                 // Transaction auth options to use throughout this session
}

// CartesiStateTransitionRaw is an auto generated low-level Go binding around an Ethereum contract.
type CartesiStateTransitionRaw struct {
	Contract *CartesiStateTransition // Generic contract binding to access the raw methods on
}

// CartesiStateTransitionCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type CartesiStateTransitionCallerRaw struct {
	Contract *CartesiStateTransitionCaller // Generic read-only contract binding to access the raw methods on
}

// CartesiStateTransitionTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type CartesiStateTransitionTransactorRaw struct {
	Contract *CartesiStateTransitionTransactor // Generic write-only contract binding to access the raw methods on
}

// NewCartesiStateTransition creates a new instance of CartesiStateTransition, bound to a specific deployed contract.
func NewCartesiStateTransition(address common.Address, backend bind.ContractBackend) (*CartesiStateTransition, error) {
	contract, err := bindCartesiStateTransition(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &CartesiStateTransition{CartesiStateTransitionCaller: CartesiStateTransitionCaller{contract: contract}, CartesiStateTransitionTransactor: CartesiStateTransitionTransactor{contract: contract}, CartesiStateTransitionFilterer: CartesiStateTransitionFilterer{contract: contract}}, nil
}

// NewCartesiStateTransitionCaller creates a new read-only instance of CartesiStateTransition, bound to a specific deployed contract.
func NewCartesiStateTransitionCaller(address common.Address, caller bind.ContractCaller) (*CartesiStateTransitionCaller, error) {
	contract, err := bindCartesiStateTransition(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &CartesiStateTransitionCaller{contract: contract}, nil
}

// NewCartesiStateTransitionTransactor creates a new write-only instance of CartesiStateTransition, bound to a specific deployed contract.
func NewCartesiStateTransitionTransactor(address common.Address, transactor bind.ContractTransactor) (*CartesiStateTransitionTransactor, error) {
	contract, err := bindCartesiStateTransition(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &CartesiStateTransitionTransactor{contract: contract}, nil
}

// NewCartesiStateTransitionFilterer creates a new log filterer instance of CartesiStateTransition, bound to a specific deployed contract.
func NewCartesiStateTransitionFilterer(address common.Address, filterer bind.ContractFilterer) (*CartesiStateTransitionFilterer, error) {
	contract, err := bindCartesiStateTransition(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &CartesiStateTransitionFilterer{contract: contract}, nil
}

// bindCartesiStateTransition binds a generic wrapper to an already deployed contract.
func bindCartesiStateTransition(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := CartesiStateTransitionMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_CartesiStateTransition *CartesiStateTransitionRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _CartesiStateTransition.Contract.CartesiStateTransitionCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_CartesiStateTransition *CartesiStateTransitionRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CartesiStateTransition.Contract.CartesiStateTransitionTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_CartesiStateTransition *CartesiStateTransitionRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _CartesiStateTransition.Contract.CartesiStateTransitionTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_CartesiStateTransition *CartesiStateTransitionCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _CartesiStateTransition.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_CartesiStateTransition *CartesiStateTransitionTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CartesiStateTransition.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_CartesiStateTransition *CartesiStateTransitionTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _CartesiStateTransition.Contract.contract.Transact(opts, method, params...)
}

// CMMARCHID is a free data retrieval call binding the contract method 0xae99a0db.
//
// Solidity: function CM_MARCHID() view returns(uint64)
func (_CartesiStateTransition *CartesiStateTransitionCaller) CMMARCHID(opts *bind.CallOpts) (uint64, error) {
	var out []interface{}
	err := _CartesiStateTransition.contract.Call(opts, &out, "CM_MARCHID")

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// CMMARCHID is a free data retrieval call binding the contract method 0xae99a0db.
//
// Solidity: function CM_MARCHID() view returns(uint64)
func (_CartesiStateTransition *CartesiStateTransitionSession) CMMARCHID() (uint64, error) {
	return _CartesiStateTransition.Contract.CMMARCHID(&_CartesiStateTransition.CallOpts)
}

// CMMARCHID is a free data retrieval call binding the contract method 0xae99a0db.
//
// Solidity: function CM_MARCHID() view returns(uint64)
func (_CartesiStateTransition *CartesiStateTransitionCallerSession) CMMARCHID() (uint64, error) {
	return _CartesiStateTransition.Contract.CMMARCHID(&_CartesiStateTransition.CallOpts)
}

// UARCHPRISTINESTATEHASH is a free data retrieval call binding the contract method 0x7587c22c.
//
// Solidity: function UARCH_PRISTINE_STATE_HASH() view returns(bytes32)
func (_CartesiStateTransition *CartesiStateTransitionCaller) UARCHPRISTINESTATEHASH(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _CartesiStateTransition.contract.Call(opts, &out, "UARCH_PRISTINE_STATE_HASH")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// UARCHPRISTINESTATEHASH is a free data retrieval call binding the contract method 0x7587c22c.
//
// Solidity: function UARCH_PRISTINE_STATE_HASH() view returns(bytes32)
func (_CartesiStateTransition *CartesiStateTransitionSession) UARCHPRISTINESTATEHASH() ([32]byte, error) {
	return _CartesiStateTransition.Contract.UARCHPRISTINESTATEHASH(&_CartesiStateTransition.CallOpts)
}

// UARCHPRISTINESTATEHASH is a free data retrieval call binding the contract method 0x7587c22c.
//
// Solidity: function UARCH_PRISTINE_STATE_HASH() view returns(bytes32)
func (_CartesiStateTransition *CartesiStateTransitionCallerSession) UARCHPRISTINESTATEHASH() ([32]byte, error) {
	return _CartesiStateTransition.Contract.UARCHPRISTINESTATEHASH(&_CartesiStateTransition.CallOpts)
}

// TransitionState is a free data retrieval call binding the contract method 0x115d0779.
//
// Solidity: function transitionState(bytes32 machineState, uint256 counter, bytes proofs, address provider) view returns(bytes32)
func (_CartesiStateTransition *CartesiStateTransitionCaller) TransitionState(opts *bind.CallOpts, machineState [32]byte, counter *big.Int, proofs []byte, provider common.Address) ([32]byte, error) {
	var out []interface{}
	err := _CartesiStateTransition.contract.Call(opts, &out, "transitionState", machineState, counter, proofs, provider)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// TransitionState is a free data retrieval call binding the contract method 0x115d0779.
//
// Solidity: function transitionState(bytes32 machineState, uint256 counter, bytes proofs, address provider) view returns(bytes32)
func (_CartesiStateTransition *CartesiStateTransitionSession) TransitionState(machineState [32]byte, counter *big.Int, proofs []byte, provider common.Address) ([32]byte, error) {
	return _CartesiStateTransition.Contract.TransitionState(&_CartesiStateTransition.CallOpts, machineState, counter, proofs, provider)
}

// TransitionState is a free data retrieval call binding the contract method 0x115d0779.
//
// Solidity: function transitionState(bytes32 machineState, uint256 counter, bytes proofs, address provider) view returns(bytes32)
func (_CartesiStateTransition *CartesiStateTransitionCallerSession) TransitionState(machineState [32]byte, counter *big.Int, proofs []byte, provider common.Address) ([32]byte, error) {
	return _CartesiStateTransition.Contract.TransitionState(&_CartesiStateTransition.CallOpts, machineState, counter, proofs, provider)
}
