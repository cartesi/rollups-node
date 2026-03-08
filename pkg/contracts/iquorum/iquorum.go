// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package iquorum

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

// IQuorumMetaData contains all meta data concerning the IQuorum contract.
var IQuorumMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"getEpochLength\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNumberOfAcceptedClaims\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isOutputsMerkleRootValid\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isValidatorInFavorOf\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"id\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isValidatorInFavorOfAnyClaimInEpoch\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"id\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"numOfValidators\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"numOfValidatorsInFavorOf\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"numOfValidatorsInFavorOfAnyClaimInEpoch\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"submitClaim\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"supportsInterface\",\"inputs\":[{\"name\":\"interfaceId\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"validatorById\",\"inputs\":[{\"name\":\"id\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"validatorId\",\"inputs\":[{\"name\":\"validator\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"ClaimAccepted\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ClaimSubmitted\",\"inputs\":[{\"name\":\"submitter\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"NotEpochFinalBlock\",\"inputs\":[{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"epochLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NotFirstClaim\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NotPastBlock\",\"inputs\":[{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]",
}

// IQuorumABI is the input ABI used to generate the binding from.
// Deprecated: Use IQuorumMetaData.ABI instead.
var IQuorumABI = IQuorumMetaData.ABI

// IQuorum is an auto generated Go binding around an Ethereum contract.
type IQuorum struct {
	IQuorumCaller     // Read-only binding to the contract
	IQuorumTransactor // Write-only binding to the contract
	IQuorumFilterer   // Log filterer for contract events
}

// IQuorumCaller is an auto generated read-only Go binding around an Ethereum contract.
type IQuorumCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IQuorumTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IQuorumTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IQuorumFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IQuorumFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IQuorumSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IQuorumSession struct {
	Contract     *IQuorum          // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IQuorumCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IQuorumCallerSession struct {
	Contract *IQuorumCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts  // Call options to use throughout this session
}

// IQuorumTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IQuorumTransactorSession struct {
	Contract     *IQuorumTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts  // Transaction auth options to use throughout this session
}

// IQuorumRaw is an auto generated low-level Go binding around an Ethereum contract.
type IQuorumRaw struct {
	Contract *IQuorum // Generic contract binding to access the raw methods on
}

// IQuorumCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IQuorumCallerRaw struct {
	Contract *IQuorumCaller // Generic read-only contract binding to access the raw methods on
}

// IQuorumTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IQuorumTransactorRaw struct {
	Contract *IQuorumTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIQuorum creates a new instance of IQuorum, bound to a specific deployed contract.
func NewIQuorum(address common.Address, backend bind.ContractBackend) (*IQuorum, error) {
	contract, err := bindIQuorum(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IQuorum{IQuorumCaller: IQuorumCaller{contract: contract}, IQuorumTransactor: IQuorumTransactor{contract: contract}, IQuorumFilterer: IQuorumFilterer{contract: contract}}, nil
}

// NewIQuorumCaller creates a new read-only instance of IQuorum, bound to a specific deployed contract.
func NewIQuorumCaller(address common.Address, caller bind.ContractCaller) (*IQuorumCaller, error) {
	contract, err := bindIQuorum(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IQuorumCaller{contract: contract}, nil
}

// NewIQuorumTransactor creates a new write-only instance of IQuorum, bound to a specific deployed contract.
func NewIQuorumTransactor(address common.Address, transactor bind.ContractTransactor) (*IQuorumTransactor, error) {
	contract, err := bindIQuorum(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IQuorumTransactor{contract: contract}, nil
}

// NewIQuorumFilterer creates a new log filterer instance of IQuorum, bound to a specific deployed contract.
func NewIQuorumFilterer(address common.Address, filterer bind.ContractFilterer) (*IQuorumFilterer, error) {
	contract, err := bindIQuorum(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IQuorumFilterer{contract: contract}, nil
}

// bindIQuorum binds a generic wrapper to an already deployed contract.
func bindIQuorum(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IQuorumMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IQuorum *IQuorumRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IQuorum.Contract.IQuorumCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IQuorum *IQuorumRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IQuorum.Contract.IQuorumTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IQuorum *IQuorumRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IQuorum.Contract.IQuorumTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IQuorum *IQuorumCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IQuorum.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IQuorum *IQuorumTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IQuorum.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IQuorum *IQuorumTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IQuorum.Contract.contract.Transact(opts, method, params...)
}

// GetEpochLength is a free data retrieval call binding the contract method 0xcfe8a73b.
//
// Solidity: function getEpochLength() view returns(uint256)
func (_IQuorum *IQuorumCaller) GetEpochLength(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "getEpochLength")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetEpochLength is a free data retrieval call binding the contract method 0xcfe8a73b.
//
// Solidity: function getEpochLength() view returns(uint256)
func (_IQuorum *IQuorumSession) GetEpochLength() (*big.Int, error) {
	return _IQuorum.Contract.GetEpochLength(&_IQuorum.CallOpts)
}

// GetEpochLength is a free data retrieval call binding the contract method 0xcfe8a73b.
//
// Solidity: function getEpochLength() view returns(uint256)
func (_IQuorum *IQuorumCallerSession) GetEpochLength() (*big.Int, error) {
	return _IQuorum.Contract.GetEpochLength(&_IQuorum.CallOpts)
}

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0xd574f4d7.
//
// Solidity: function getNumberOfAcceptedClaims() view returns(uint256)
func (_IQuorum *IQuorumCaller) GetNumberOfAcceptedClaims(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "getNumberOfAcceptedClaims")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0xd574f4d7.
//
// Solidity: function getNumberOfAcceptedClaims() view returns(uint256)
func (_IQuorum *IQuorumSession) GetNumberOfAcceptedClaims() (*big.Int, error) {
	return _IQuorum.Contract.GetNumberOfAcceptedClaims(&_IQuorum.CallOpts)
}

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0xd574f4d7.
//
// Solidity: function getNumberOfAcceptedClaims() view returns(uint256)
func (_IQuorum *IQuorumCallerSession) GetNumberOfAcceptedClaims() (*big.Int, error) {
	return _IQuorum.Contract.GetNumberOfAcceptedClaims(&_IQuorum.CallOpts)
}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IQuorum *IQuorumCaller) IsOutputsMerkleRootValid(opts *bind.CallOpts, appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "isOutputsMerkleRootValid", appContract, outputsMerkleRoot)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IQuorum *IQuorumSession) IsOutputsMerkleRootValid(appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	return _IQuorum.Contract.IsOutputsMerkleRootValid(&_IQuorum.CallOpts, appContract, outputsMerkleRoot)
}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IQuorum *IQuorumCallerSession) IsOutputsMerkleRootValid(appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	return _IQuorum.Contract.IsOutputsMerkleRootValid(&_IQuorum.CallOpts, appContract, outputsMerkleRoot)
}

// IsValidatorInFavorOf is a free data retrieval call binding the contract method 0x4b84231c.
//
// Solidity: function isValidatorInFavorOf(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, uint256 id) view returns(bool)
func (_IQuorum *IQuorumCaller) IsValidatorInFavorOf(opts *bind.CallOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte, id *big.Int) (bool, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "isValidatorInFavorOf", appContract, lastProcessedBlockNumber, outputsMerkleRoot, id)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsValidatorInFavorOf is a free data retrieval call binding the contract method 0x4b84231c.
//
// Solidity: function isValidatorInFavorOf(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, uint256 id) view returns(bool)
func (_IQuorum *IQuorumSession) IsValidatorInFavorOf(appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte, id *big.Int) (bool, error) {
	return _IQuorum.Contract.IsValidatorInFavorOf(&_IQuorum.CallOpts, appContract, lastProcessedBlockNumber, outputsMerkleRoot, id)
}

// IsValidatorInFavorOf is a free data retrieval call binding the contract method 0x4b84231c.
//
// Solidity: function isValidatorInFavorOf(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, uint256 id) view returns(bool)
func (_IQuorum *IQuorumCallerSession) IsValidatorInFavorOf(appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte, id *big.Int) (bool, error) {
	return _IQuorum.Contract.IsValidatorInFavorOf(&_IQuorum.CallOpts, appContract, lastProcessedBlockNumber, outputsMerkleRoot, id)
}

// IsValidatorInFavorOfAnyClaimInEpoch is a free data retrieval call binding the contract method 0x4b53459c.
//
// Solidity: function isValidatorInFavorOfAnyClaimInEpoch(address appContract, uint256 lastProcessedBlockNumber, uint256 id) view returns(bool)
func (_IQuorum *IQuorumCaller) IsValidatorInFavorOfAnyClaimInEpoch(opts *bind.CallOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, id *big.Int) (bool, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "isValidatorInFavorOfAnyClaimInEpoch", appContract, lastProcessedBlockNumber, id)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsValidatorInFavorOfAnyClaimInEpoch is a free data retrieval call binding the contract method 0x4b53459c.
//
// Solidity: function isValidatorInFavorOfAnyClaimInEpoch(address appContract, uint256 lastProcessedBlockNumber, uint256 id) view returns(bool)
func (_IQuorum *IQuorumSession) IsValidatorInFavorOfAnyClaimInEpoch(appContract common.Address, lastProcessedBlockNumber *big.Int, id *big.Int) (bool, error) {
	return _IQuorum.Contract.IsValidatorInFavorOfAnyClaimInEpoch(&_IQuorum.CallOpts, appContract, lastProcessedBlockNumber, id)
}

// IsValidatorInFavorOfAnyClaimInEpoch is a free data retrieval call binding the contract method 0x4b53459c.
//
// Solidity: function isValidatorInFavorOfAnyClaimInEpoch(address appContract, uint256 lastProcessedBlockNumber, uint256 id) view returns(bool)
func (_IQuorum *IQuorumCallerSession) IsValidatorInFavorOfAnyClaimInEpoch(appContract common.Address, lastProcessedBlockNumber *big.Int, id *big.Int) (bool, error) {
	return _IQuorum.Contract.IsValidatorInFavorOfAnyClaimInEpoch(&_IQuorum.CallOpts, appContract, lastProcessedBlockNumber, id)
}

// NumOfValidators is a free data retrieval call binding the contract method 0x1e526e45.
//
// Solidity: function numOfValidators() view returns(uint256)
func (_IQuorum *IQuorumCaller) NumOfValidators(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "numOfValidators")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// NumOfValidators is a free data retrieval call binding the contract method 0x1e526e45.
//
// Solidity: function numOfValidators() view returns(uint256)
func (_IQuorum *IQuorumSession) NumOfValidators() (*big.Int, error) {
	return _IQuorum.Contract.NumOfValidators(&_IQuorum.CallOpts)
}

// NumOfValidators is a free data retrieval call binding the contract method 0x1e526e45.
//
// Solidity: function numOfValidators() view returns(uint256)
func (_IQuorum *IQuorumCallerSession) NumOfValidators() (*big.Int, error) {
	return _IQuorum.Contract.NumOfValidators(&_IQuorum.CallOpts)
}

// NumOfValidatorsInFavorOf is a free data retrieval call binding the contract method 0x7051bfd5.
//
// Solidity: function numOfValidatorsInFavorOf(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot) view returns(uint256)
func (_IQuorum *IQuorumCaller) NumOfValidatorsInFavorOf(opts *bind.CallOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "numOfValidatorsInFavorOf", appContract, lastProcessedBlockNumber, outputsMerkleRoot)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// NumOfValidatorsInFavorOf is a free data retrieval call binding the contract method 0x7051bfd5.
//
// Solidity: function numOfValidatorsInFavorOf(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot) view returns(uint256)
func (_IQuorum *IQuorumSession) NumOfValidatorsInFavorOf(appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*big.Int, error) {
	return _IQuorum.Contract.NumOfValidatorsInFavorOf(&_IQuorum.CallOpts, appContract, lastProcessedBlockNumber, outputsMerkleRoot)
}

// NumOfValidatorsInFavorOf is a free data retrieval call binding the contract method 0x7051bfd5.
//
// Solidity: function numOfValidatorsInFavorOf(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot) view returns(uint256)
func (_IQuorum *IQuorumCallerSession) NumOfValidatorsInFavorOf(appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*big.Int, error) {
	return _IQuorum.Contract.NumOfValidatorsInFavorOf(&_IQuorum.CallOpts, appContract, lastProcessedBlockNumber, outputsMerkleRoot)
}

// NumOfValidatorsInFavorOfAnyClaimInEpoch is a free data retrieval call binding the contract method 0x446ccbf0.
//
// Solidity: function numOfValidatorsInFavorOfAnyClaimInEpoch(address appContract, uint256 lastProcessedBlockNumber) view returns(uint256)
func (_IQuorum *IQuorumCaller) NumOfValidatorsInFavorOfAnyClaimInEpoch(opts *bind.CallOpts, appContract common.Address, lastProcessedBlockNumber *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "numOfValidatorsInFavorOfAnyClaimInEpoch", appContract, lastProcessedBlockNumber)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// NumOfValidatorsInFavorOfAnyClaimInEpoch is a free data retrieval call binding the contract method 0x446ccbf0.
//
// Solidity: function numOfValidatorsInFavorOfAnyClaimInEpoch(address appContract, uint256 lastProcessedBlockNumber) view returns(uint256)
func (_IQuorum *IQuorumSession) NumOfValidatorsInFavorOfAnyClaimInEpoch(appContract common.Address, lastProcessedBlockNumber *big.Int) (*big.Int, error) {
	return _IQuorum.Contract.NumOfValidatorsInFavorOfAnyClaimInEpoch(&_IQuorum.CallOpts, appContract, lastProcessedBlockNumber)
}

// NumOfValidatorsInFavorOfAnyClaimInEpoch is a free data retrieval call binding the contract method 0x446ccbf0.
//
// Solidity: function numOfValidatorsInFavorOfAnyClaimInEpoch(address appContract, uint256 lastProcessedBlockNumber) view returns(uint256)
func (_IQuorum *IQuorumCallerSession) NumOfValidatorsInFavorOfAnyClaimInEpoch(appContract common.Address, lastProcessedBlockNumber *big.Int) (*big.Int, error) {
	return _IQuorum.Contract.NumOfValidatorsInFavorOfAnyClaimInEpoch(&_IQuorum.CallOpts, appContract, lastProcessedBlockNumber)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IQuorum *IQuorumCaller) SupportsInterface(opts *bind.CallOpts, interfaceId [4]byte) (bool, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "supportsInterface", interfaceId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IQuorum *IQuorumSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _IQuorum.Contract.SupportsInterface(&_IQuorum.CallOpts, interfaceId)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IQuorum *IQuorumCallerSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _IQuorum.Contract.SupportsInterface(&_IQuorum.CallOpts, interfaceId)
}

// ValidatorById is a free data retrieval call binding the contract method 0x1c45396a.
//
// Solidity: function validatorById(uint256 id) view returns(address)
func (_IQuorum *IQuorumCaller) ValidatorById(opts *bind.CallOpts, id *big.Int) (common.Address, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "validatorById", id)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// ValidatorById is a free data retrieval call binding the contract method 0x1c45396a.
//
// Solidity: function validatorById(uint256 id) view returns(address)
func (_IQuorum *IQuorumSession) ValidatorById(id *big.Int) (common.Address, error) {
	return _IQuorum.Contract.ValidatorById(&_IQuorum.CallOpts, id)
}

// ValidatorById is a free data retrieval call binding the contract method 0x1c45396a.
//
// Solidity: function validatorById(uint256 id) view returns(address)
func (_IQuorum *IQuorumCallerSession) ValidatorById(id *big.Int) (common.Address, error) {
	return _IQuorum.Contract.ValidatorById(&_IQuorum.CallOpts, id)
}

// ValidatorId is a free data retrieval call binding the contract method 0x0a6f1fe8.
//
// Solidity: function validatorId(address validator) view returns(uint256)
func (_IQuorum *IQuorumCaller) ValidatorId(opts *bind.CallOpts, validator common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "validatorId", validator)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ValidatorId is a free data retrieval call binding the contract method 0x0a6f1fe8.
//
// Solidity: function validatorId(address validator) view returns(uint256)
func (_IQuorum *IQuorumSession) ValidatorId(validator common.Address) (*big.Int, error) {
	return _IQuorum.Contract.ValidatorId(&_IQuorum.CallOpts, validator)
}

// ValidatorId is a free data retrieval call binding the contract method 0x0a6f1fe8.
//
// Solidity: function validatorId(address validator) view returns(uint256)
func (_IQuorum *IQuorumCallerSession) ValidatorId(validator common.Address) (*big.Int, error) {
	return _IQuorum.Contract.ValidatorId(&_IQuorum.CallOpts, validator)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x6470af00.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot) returns()
func (_IQuorum *IQuorumTransactor) SubmitClaim(opts *bind.TransactOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IQuorum.contract.Transact(opts, "submitClaim", appContract, lastProcessedBlockNumber, outputsMerkleRoot)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x6470af00.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot) returns()
func (_IQuorum *IQuorumSession) SubmitClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IQuorum.Contract.SubmitClaim(&_IQuorum.TransactOpts, appContract, lastProcessedBlockNumber, outputsMerkleRoot)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x6470af00.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot) returns()
func (_IQuorum *IQuorumTransactorSession) SubmitClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IQuorum.Contract.SubmitClaim(&_IQuorum.TransactOpts, appContract, lastProcessedBlockNumber, outputsMerkleRoot)
}

// IQuorumClaimAcceptedIterator is returned from FilterClaimAccepted and is used to iterate over the raw logs and unpacked data for ClaimAccepted events raised by the IQuorum contract.
type IQuorumClaimAcceptedIterator struct {
	Event *IQuorumClaimAccepted // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IQuorumClaimAcceptedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IQuorumClaimAccepted)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IQuorumClaimAccepted)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IQuorumClaimAcceptedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IQuorumClaimAcceptedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IQuorumClaimAccepted represents a ClaimAccepted event raised by the IQuorum contract.
type IQuorumClaimAccepted struct {
	AppContract              common.Address
	LastProcessedBlockNumber *big.Int
	OutputsMerkleRoot        [32]byte
	Raw                      types.Log // Blockchain specific contextual infos
}

// FilterClaimAccepted is a free log retrieval operation binding the contract event 0x0f2cd00a405c0d1a66050307b6722c4788db6ed57aa3589a5c38da535cc3ce63.
//
// Solidity: event ClaimAccepted(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IQuorum *IQuorumFilterer) FilterClaimAccepted(opts *bind.FilterOpts, appContract []common.Address) (*IQuorumClaimAcceptedIterator, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IQuorum.contract.FilterLogs(opts, "ClaimAccepted", appContractRule)
	if err != nil {
		return nil, err
	}
	return &IQuorumClaimAcceptedIterator{contract: _IQuorum.contract, event: "ClaimAccepted", logs: logs, sub: sub}, nil
}

// WatchClaimAccepted is a free log subscription operation binding the contract event 0x0f2cd00a405c0d1a66050307b6722c4788db6ed57aa3589a5c38da535cc3ce63.
//
// Solidity: event ClaimAccepted(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IQuorum *IQuorumFilterer) WatchClaimAccepted(opts *bind.WatchOpts, sink chan<- *IQuorumClaimAccepted, appContract []common.Address) (event.Subscription, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IQuorum.contract.WatchLogs(opts, "ClaimAccepted", appContractRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IQuorumClaimAccepted)
				if err := _IQuorum.contract.UnpackLog(event, "ClaimAccepted", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseClaimAccepted is a log parse operation binding the contract event 0x0f2cd00a405c0d1a66050307b6722c4788db6ed57aa3589a5c38da535cc3ce63.
//
// Solidity: event ClaimAccepted(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IQuorum *IQuorumFilterer) ParseClaimAccepted(log types.Log) (*IQuorumClaimAccepted, error) {
	event := new(IQuorumClaimAccepted)
	if err := _IQuorum.contract.UnpackLog(event, "ClaimAccepted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IQuorumClaimSubmittedIterator is returned from FilterClaimSubmitted and is used to iterate over the raw logs and unpacked data for ClaimSubmitted events raised by the IQuorum contract.
type IQuorumClaimSubmittedIterator struct {
	Event *IQuorumClaimSubmitted // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *IQuorumClaimSubmittedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IQuorumClaimSubmitted)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(IQuorumClaimSubmitted)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *IQuorumClaimSubmittedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IQuorumClaimSubmittedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IQuorumClaimSubmitted represents a ClaimSubmitted event raised by the IQuorum contract.
type IQuorumClaimSubmitted struct {
	Submitter                common.Address
	AppContract              common.Address
	LastProcessedBlockNumber *big.Int
	OutputsMerkleRoot        [32]byte
	Raw                      types.Log // Blockchain specific contextual infos
}

// FilterClaimSubmitted is a free log retrieval operation binding the contract event 0xf4ff953641f10e17dd93c0bc51334cb1f711fdcb4e37992021a5973f7a958f09.
//
// Solidity: event ClaimSubmitted(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IQuorum *IQuorumFilterer) FilterClaimSubmitted(opts *bind.FilterOpts, submitter []common.Address, appContract []common.Address) (*IQuorumClaimSubmittedIterator, error) {

	var submitterRule []interface{}
	for _, submitterItem := range submitter {
		submitterRule = append(submitterRule, submitterItem)
	}
	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IQuorum.contract.FilterLogs(opts, "ClaimSubmitted", submitterRule, appContractRule)
	if err != nil {
		return nil, err
	}
	return &IQuorumClaimSubmittedIterator{contract: _IQuorum.contract, event: "ClaimSubmitted", logs: logs, sub: sub}, nil
}

// WatchClaimSubmitted is a free log subscription operation binding the contract event 0xf4ff953641f10e17dd93c0bc51334cb1f711fdcb4e37992021a5973f7a958f09.
//
// Solidity: event ClaimSubmitted(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IQuorum *IQuorumFilterer) WatchClaimSubmitted(opts *bind.WatchOpts, sink chan<- *IQuorumClaimSubmitted, submitter []common.Address, appContract []common.Address) (event.Subscription, error) {

	var submitterRule []interface{}
	for _, submitterItem := range submitter {
		submitterRule = append(submitterRule, submitterItem)
	}
	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IQuorum.contract.WatchLogs(opts, "ClaimSubmitted", submitterRule, appContractRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IQuorumClaimSubmitted)
				if err := _IQuorum.contract.UnpackLog(event, "ClaimSubmitted", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseClaimSubmitted is a log parse operation binding the contract event 0xf4ff953641f10e17dd93c0bc51334cb1f711fdcb4e37992021a5973f7a958f09.
//
// Solidity: event ClaimSubmitted(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IQuorum *IQuorumFilterer) ParseClaimSubmitted(log types.Log) (*IQuorumClaimSubmitted, error) {
	event := new(IQuorumClaimSubmitted)
	if err := _IQuorum.contract.UnpackLog(event, "ClaimSubmitted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
