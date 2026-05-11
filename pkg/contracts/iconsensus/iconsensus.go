// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package iconsensus

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

// IConsensusClaim is an auto generated low-level Go binding around an user-defined struct.
type IConsensusClaim struct {
	Status                  uint8
	StagingBlockNumber      *big.Int
	StagedOutputsMerkleRoot [32]byte
}

// IConsensusMetaData contains all meta data concerning the IConsensus contract.
var IConsensusMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"acceptClaim\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getClaim\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"claim\",\"type\":\"tuple\",\"internalType\":\"structIConsensus.Claim\",\"components\":[{\"name\":\"status\",\"type\":\"uint8\",\"internalType\":\"enumIConsensus.ClaimStatus\"},{\"name\":\"stagingBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"stagedOutputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getClaimStagingPeriod\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getEpochLength\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getLastFinalizedMachineMerkleRoot\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNumberOfAcceptedClaims\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNumberOfStagedClaims\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNumberOfSubmittedClaims\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isOutputsMerkleRootValid\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"submitClaim\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"proof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"supportsInterface\",\"inputs\":[{\"name\":\"interfaceId\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"major\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"minor\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"patch\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"preRelease\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"buildMetadata\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"ClaimAccepted\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ClaimStaged\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ClaimSubmitted\",\"inputs\":[{\"name\":\"submitter\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"ApplicationForeclosed\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ApplicationNotDeployed\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ApplicationReverted\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"error\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"ClaimNotStaged\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"claimStatus\",\"type\":\"uint8\",\"internalType\":\"enumIConsensus.ClaimStatus\"}]},{\"type\":\"error\",\"name\":\"ClaimStagingPeriodNotOverYet\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"numberOfBlocksAfterStaging\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"claimStagingPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"IllformedApplicationReturnData\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"InvalidOutputsMerkleRootProofSize\",\"inputs\":[{\"name\":\"suppliedProofSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedProofSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NotEpochFinalBlock\",\"inputs\":[{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"epochLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NotFirstClaim\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NotPastBlock\",\"inputs\":[{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]",
}

// IConsensusABI is the input ABI used to generate the binding from.
// Deprecated: Use IConsensusMetaData.ABI instead.
var IConsensusABI = IConsensusMetaData.ABI

// IConsensus is an auto generated Go binding around an Ethereum contract.
type IConsensus struct {
	IConsensusCaller     // Read-only binding to the contract
	IConsensusTransactor // Write-only binding to the contract
	IConsensusFilterer   // Log filterer for contract events
}

// IConsensusCaller is an auto generated read-only Go binding around an Ethereum contract.
type IConsensusCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IConsensusTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IConsensusTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IConsensusFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IConsensusFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IConsensusSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IConsensusSession struct {
	Contract     *IConsensus       // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IConsensusCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IConsensusCallerSession struct {
	Contract *IConsensusCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts     // Call options to use throughout this session
}

// IConsensusTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IConsensusTransactorSession struct {
	Contract     *IConsensusTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// IConsensusRaw is an auto generated low-level Go binding around an Ethereum contract.
type IConsensusRaw struct {
	Contract *IConsensus // Generic contract binding to access the raw methods on
}

// IConsensusCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IConsensusCallerRaw struct {
	Contract *IConsensusCaller // Generic read-only contract binding to access the raw methods on
}

// IConsensusTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IConsensusTransactorRaw struct {
	Contract *IConsensusTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIConsensus creates a new instance of IConsensus, bound to a specific deployed contract.
func NewIConsensus(address common.Address, backend bind.ContractBackend) (*IConsensus, error) {
	contract, err := bindIConsensus(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IConsensus{IConsensusCaller: IConsensusCaller{contract: contract}, IConsensusTransactor: IConsensusTransactor{contract: contract}, IConsensusFilterer: IConsensusFilterer{contract: contract}}, nil
}

// NewIConsensusCaller creates a new read-only instance of IConsensus, bound to a specific deployed contract.
func NewIConsensusCaller(address common.Address, caller bind.ContractCaller) (*IConsensusCaller, error) {
	contract, err := bindIConsensus(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IConsensusCaller{contract: contract}, nil
}

// NewIConsensusTransactor creates a new write-only instance of IConsensus, bound to a specific deployed contract.
func NewIConsensusTransactor(address common.Address, transactor bind.ContractTransactor) (*IConsensusTransactor, error) {
	contract, err := bindIConsensus(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IConsensusTransactor{contract: contract}, nil
}

// NewIConsensusFilterer creates a new log filterer instance of IConsensus, bound to a specific deployed contract.
func NewIConsensusFilterer(address common.Address, filterer bind.ContractFilterer) (*IConsensusFilterer, error) {
	contract, err := bindIConsensus(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IConsensusFilterer{contract: contract}, nil
}

// bindIConsensus binds a generic wrapper to an already deployed contract.
func bindIConsensus(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IConsensusMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IConsensus *IConsensusRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IConsensus.Contract.IConsensusCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IConsensus *IConsensusRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IConsensus.Contract.IConsensusTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IConsensus *IConsensusRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IConsensus.Contract.IConsensusTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IConsensus *IConsensusCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IConsensus.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IConsensus *IConsensusTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IConsensus.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IConsensus *IConsensusTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IConsensus.Contract.contract.Transact(opts, method, params...)
}

// GetClaim is a free data retrieval call binding the contract method 0xa1abc0ae.
//
// Solidity: function getClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) view returns((uint8,uint256,bytes32) claim)
func (_IConsensus *IConsensusCaller) GetClaim(opts *bind.CallOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (IConsensusClaim, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "getClaim", appContract, lastProcessedBlockNumber, machineMerkleRoot)

	if err != nil {
		return *new(IConsensusClaim), err
	}

	out0 := *abi.ConvertType(out[0], new(IConsensusClaim)).(*IConsensusClaim)

	return out0, err

}

// GetClaim is a free data retrieval call binding the contract method 0xa1abc0ae.
//
// Solidity: function getClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) view returns((uint8,uint256,bytes32) claim)
func (_IConsensus *IConsensusSession) GetClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (IConsensusClaim, error) {
	return _IConsensus.Contract.GetClaim(&_IConsensus.CallOpts, appContract, lastProcessedBlockNumber, machineMerkleRoot)
}

// GetClaim is a free data retrieval call binding the contract method 0xa1abc0ae.
//
// Solidity: function getClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) view returns((uint8,uint256,bytes32) claim)
func (_IConsensus *IConsensusCallerSession) GetClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (IConsensusClaim, error) {
	return _IConsensus.Contract.GetClaim(&_IConsensus.CallOpts, appContract, lastProcessedBlockNumber, machineMerkleRoot)
}

// GetClaimStagingPeriod is a free data retrieval call binding the contract method 0xa04c6564.
//
// Solidity: function getClaimStagingPeriod() view returns(uint256)
func (_IConsensus *IConsensusCaller) GetClaimStagingPeriod(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "getClaimStagingPeriod")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetClaimStagingPeriod is a free data retrieval call binding the contract method 0xa04c6564.
//
// Solidity: function getClaimStagingPeriod() view returns(uint256)
func (_IConsensus *IConsensusSession) GetClaimStagingPeriod() (*big.Int, error) {
	return _IConsensus.Contract.GetClaimStagingPeriod(&_IConsensus.CallOpts)
}

// GetClaimStagingPeriod is a free data retrieval call binding the contract method 0xa04c6564.
//
// Solidity: function getClaimStagingPeriod() view returns(uint256)
func (_IConsensus *IConsensusCallerSession) GetClaimStagingPeriod() (*big.Int, error) {
	return _IConsensus.Contract.GetClaimStagingPeriod(&_IConsensus.CallOpts)
}

// GetEpochLength is a free data retrieval call binding the contract method 0xcfe8a73b.
//
// Solidity: function getEpochLength() view returns(uint256)
func (_IConsensus *IConsensusCaller) GetEpochLength(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "getEpochLength")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetEpochLength is a free data retrieval call binding the contract method 0xcfe8a73b.
//
// Solidity: function getEpochLength() view returns(uint256)
func (_IConsensus *IConsensusSession) GetEpochLength() (*big.Int, error) {
	return _IConsensus.Contract.GetEpochLength(&_IConsensus.CallOpts)
}

// GetEpochLength is a free data retrieval call binding the contract method 0xcfe8a73b.
//
// Solidity: function getEpochLength() view returns(uint256)
func (_IConsensus *IConsensusCallerSession) GetEpochLength() (*big.Int, error) {
	return _IConsensus.Contract.GetEpochLength(&_IConsensus.CallOpts)
}

// GetLastFinalizedMachineMerkleRoot is a free data retrieval call binding the contract method 0x5ac9cfbf.
//
// Solidity: function getLastFinalizedMachineMerkleRoot(address appContract) view returns(bytes32)
func (_IConsensus *IConsensusCaller) GetLastFinalizedMachineMerkleRoot(opts *bind.CallOpts, appContract common.Address) ([32]byte, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "getLastFinalizedMachineMerkleRoot", appContract)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetLastFinalizedMachineMerkleRoot is a free data retrieval call binding the contract method 0x5ac9cfbf.
//
// Solidity: function getLastFinalizedMachineMerkleRoot(address appContract) view returns(bytes32)
func (_IConsensus *IConsensusSession) GetLastFinalizedMachineMerkleRoot(appContract common.Address) ([32]byte, error) {
	return _IConsensus.Contract.GetLastFinalizedMachineMerkleRoot(&_IConsensus.CallOpts, appContract)
}

// GetLastFinalizedMachineMerkleRoot is a free data retrieval call binding the contract method 0x5ac9cfbf.
//
// Solidity: function getLastFinalizedMachineMerkleRoot(address appContract) view returns(bytes32)
func (_IConsensus *IConsensusCallerSession) GetLastFinalizedMachineMerkleRoot(appContract common.Address) ([32]byte, error) {
	return _IConsensus.Contract.GetLastFinalizedMachineMerkleRoot(&_IConsensus.CallOpts, appContract)
}

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0x80a80953.
//
// Solidity: function getNumberOfAcceptedClaims(address appContract) view returns(uint256)
func (_IConsensus *IConsensusCaller) GetNumberOfAcceptedClaims(opts *bind.CallOpts, appContract common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "getNumberOfAcceptedClaims", appContract)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0x80a80953.
//
// Solidity: function getNumberOfAcceptedClaims(address appContract) view returns(uint256)
func (_IConsensus *IConsensusSession) GetNumberOfAcceptedClaims(appContract common.Address) (*big.Int, error) {
	return _IConsensus.Contract.GetNumberOfAcceptedClaims(&_IConsensus.CallOpts, appContract)
}

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0x80a80953.
//
// Solidity: function getNumberOfAcceptedClaims(address appContract) view returns(uint256)
func (_IConsensus *IConsensusCallerSession) GetNumberOfAcceptedClaims(appContract common.Address) (*big.Int, error) {
	return _IConsensus.Contract.GetNumberOfAcceptedClaims(&_IConsensus.CallOpts, appContract)
}

// GetNumberOfStagedClaims is a free data retrieval call binding the contract method 0x02c657a4.
//
// Solidity: function getNumberOfStagedClaims(address appContract) view returns(uint256)
func (_IConsensus *IConsensusCaller) GetNumberOfStagedClaims(opts *bind.CallOpts, appContract common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "getNumberOfStagedClaims", appContract)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNumberOfStagedClaims is a free data retrieval call binding the contract method 0x02c657a4.
//
// Solidity: function getNumberOfStagedClaims(address appContract) view returns(uint256)
func (_IConsensus *IConsensusSession) GetNumberOfStagedClaims(appContract common.Address) (*big.Int, error) {
	return _IConsensus.Contract.GetNumberOfStagedClaims(&_IConsensus.CallOpts, appContract)
}

// GetNumberOfStagedClaims is a free data retrieval call binding the contract method 0x02c657a4.
//
// Solidity: function getNumberOfStagedClaims(address appContract) view returns(uint256)
func (_IConsensus *IConsensusCallerSession) GetNumberOfStagedClaims(appContract common.Address) (*big.Int, error) {
	return _IConsensus.Contract.GetNumberOfStagedClaims(&_IConsensus.CallOpts, appContract)
}

// GetNumberOfSubmittedClaims is a free data retrieval call binding the contract method 0x43aacc77.
//
// Solidity: function getNumberOfSubmittedClaims(address appContract) view returns(uint256)
func (_IConsensus *IConsensusCaller) GetNumberOfSubmittedClaims(opts *bind.CallOpts, appContract common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "getNumberOfSubmittedClaims", appContract)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNumberOfSubmittedClaims is a free data retrieval call binding the contract method 0x43aacc77.
//
// Solidity: function getNumberOfSubmittedClaims(address appContract) view returns(uint256)
func (_IConsensus *IConsensusSession) GetNumberOfSubmittedClaims(appContract common.Address) (*big.Int, error) {
	return _IConsensus.Contract.GetNumberOfSubmittedClaims(&_IConsensus.CallOpts, appContract)
}

// GetNumberOfSubmittedClaims is a free data retrieval call binding the contract method 0x43aacc77.
//
// Solidity: function getNumberOfSubmittedClaims(address appContract) view returns(uint256)
func (_IConsensus *IConsensusCallerSession) GetNumberOfSubmittedClaims(appContract common.Address) (*big.Int, error) {
	return _IConsensus.Contract.GetNumberOfSubmittedClaims(&_IConsensus.CallOpts, appContract)
}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IConsensus *IConsensusCaller) IsOutputsMerkleRootValid(opts *bind.CallOpts, appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "isOutputsMerkleRootValid", appContract, outputsMerkleRoot)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IConsensus *IConsensusSession) IsOutputsMerkleRootValid(appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	return _IConsensus.Contract.IsOutputsMerkleRootValid(&_IConsensus.CallOpts, appContract, outputsMerkleRoot)
}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IConsensus *IConsensusCallerSession) IsOutputsMerkleRootValid(appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	return _IConsensus.Contract.IsOutputsMerkleRootValid(&_IConsensus.CallOpts, appContract, outputsMerkleRoot)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IConsensus *IConsensusCaller) SupportsInterface(opts *bind.CallOpts, interfaceId [4]byte) (bool, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "supportsInterface", interfaceId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IConsensus *IConsensusSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _IConsensus.Contract.SupportsInterface(&_IConsensus.CallOpts, interfaceId)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IConsensus *IConsensusCallerSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _IConsensus.Contract.SupportsInterface(&_IConsensus.CallOpts, interfaceId)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IConsensus *IConsensusCaller) Version(opts *bind.CallOpts) (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "version")

	outstruct := new(struct {
		Major         uint64
		Minor         uint64
		Patch         uint64
		PreRelease    string
		BuildMetadata string
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Major = *abi.ConvertType(out[0], new(uint64)).(*uint64)
	outstruct.Minor = *abi.ConvertType(out[1], new(uint64)).(*uint64)
	outstruct.Patch = *abi.ConvertType(out[2], new(uint64)).(*uint64)
	outstruct.PreRelease = *abi.ConvertType(out[3], new(string)).(*string)
	outstruct.BuildMetadata = *abi.ConvertType(out[4], new(string)).(*string)

	return *outstruct, err

}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IConsensus *IConsensusSession) Version() (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	return _IConsensus.Contract.Version(&_IConsensus.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IConsensus *IConsensusCallerSession) Version() (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	return _IConsensus.Contract.Version(&_IConsensus.CallOpts)
}

// AcceptClaim is a paid mutator transaction binding the contract method 0x8e2c381c.
//
// Solidity: function acceptClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) returns()
func (_IConsensus *IConsensusTransactor) AcceptClaim(opts *bind.TransactOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IConsensus.contract.Transact(opts, "acceptClaim", appContract, lastProcessedBlockNumber, machineMerkleRoot)
}

// AcceptClaim is a paid mutator transaction binding the contract method 0x8e2c381c.
//
// Solidity: function acceptClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) returns()
func (_IConsensus *IConsensusSession) AcceptClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IConsensus.Contract.AcceptClaim(&_IConsensus.TransactOpts, appContract, lastProcessedBlockNumber, machineMerkleRoot)
}

// AcceptClaim is a paid mutator transaction binding the contract method 0x8e2c381c.
//
// Solidity: function acceptClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) returns()
func (_IConsensus *IConsensusTransactorSession) AcceptClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IConsensus.Contract.AcceptClaim(&_IConsensus.TransactOpts, appContract, lastProcessedBlockNumber, machineMerkleRoot)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x9a00db83.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32[] proof) returns()
func (_IConsensus *IConsensusTransactor) SubmitClaim(opts *bind.TransactOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _IConsensus.contract.Transact(opts, "submitClaim", appContract, lastProcessedBlockNumber, outputsMerkleRoot, proof)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x9a00db83.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32[] proof) returns()
func (_IConsensus *IConsensusSession) SubmitClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _IConsensus.Contract.SubmitClaim(&_IConsensus.TransactOpts, appContract, lastProcessedBlockNumber, outputsMerkleRoot, proof)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x9a00db83.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32[] proof) returns()
func (_IConsensus *IConsensusTransactorSession) SubmitClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _IConsensus.Contract.SubmitClaim(&_IConsensus.TransactOpts, appContract, lastProcessedBlockNumber, outputsMerkleRoot, proof)
}

// IConsensusClaimAcceptedIterator is returned from FilterClaimAccepted and is used to iterate over the raw logs and unpacked data for ClaimAccepted events raised by the IConsensus contract.
type IConsensusClaimAcceptedIterator struct {
	Event *IConsensusClaimAccepted // Event containing the contract specifics and raw log

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
func (it *IConsensusClaimAcceptedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IConsensusClaimAccepted)
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
		it.Event = new(IConsensusClaimAccepted)
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
func (it *IConsensusClaimAcceptedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IConsensusClaimAcceptedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IConsensusClaimAccepted represents a ClaimAccepted event raised by the IConsensus contract.
type IConsensusClaimAccepted struct {
	AppContract              common.Address
	LastProcessedBlockNumber *big.Int
	OutputsMerkleRoot        [32]byte
	MachineMerkleRoot        [32]byte
	Raw                      types.Log // Blockchain specific contextual infos
}

// FilterClaimAccepted is a free log retrieval operation binding the contract event 0x8d40f6fff97997587f3d67c44cf2201ae7df0ef9a14ac7399b9a6f0fcaa3c46f.
//
// Solidity: event ClaimAccepted(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
func (_IConsensus *IConsensusFilterer) FilterClaimAccepted(opts *bind.FilterOpts, appContract []common.Address) (*IConsensusClaimAcceptedIterator, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IConsensus.contract.FilterLogs(opts, "ClaimAccepted", appContractRule)
	if err != nil {
		return nil, err
	}
	return &IConsensusClaimAcceptedIterator{contract: _IConsensus.contract, event: "ClaimAccepted", logs: logs, sub: sub}, nil
}

// WatchClaimAccepted is a free log subscription operation binding the contract event 0x8d40f6fff97997587f3d67c44cf2201ae7df0ef9a14ac7399b9a6f0fcaa3c46f.
//
// Solidity: event ClaimAccepted(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
func (_IConsensus *IConsensusFilterer) WatchClaimAccepted(opts *bind.WatchOpts, sink chan<- *IConsensusClaimAccepted, appContract []common.Address) (event.Subscription, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IConsensus.contract.WatchLogs(opts, "ClaimAccepted", appContractRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IConsensusClaimAccepted)
				if err := _IConsensus.contract.UnpackLog(event, "ClaimAccepted", log); err != nil {
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

// ParseClaimAccepted is a log parse operation binding the contract event 0x8d40f6fff97997587f3d67c44cf2201ae7df0ef9a14ac7399b9a6f0fcaa3c46f.
//
// Solidity: event ClaimAccepted(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
func (_IConsensus *IConsensusFilterer) ParseClaimAccepted(log types.Log) (*IConsensusClaimAccepted, error) {
	event := new(IConsensusClaimAccepted)
	if err := _IConsensus.contract.UnpackLog(event, "ClaimAccepted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IConsensusClaimStagedIterator is returned from FilterClaimStaged and is used to iterate over the raw logs and unpacked data for ClaimStaged events raised by the IConsensus contract.
type IConsensusClaimStagedIterator struct {
	Event *IConsensusClaimStaged // Event containing the contract specifics and raw log

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
func (it *IConsensusClaimStagedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IConsensusClaimStaged)
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
		it.Event = new(IConsensusClaimStaged)
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
func (it *IConsensusClaimStagedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IConsensusClaimStagedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IConsensusClaimStaged represents a ClaimStaged event raised by the IConsensus contract.
type IConsensusClaimStaged struct {
	AppContract              common.Address
	LastProcessedBlockNumber *big.Int
	OutputsMerkleRoot        [32]byte
	MachineMerkleRoot        [32]byte
	Raw                      types.Log // Blockchain specific contextual infos
}

// FilterClaimStaged is a free log retrieval operation binding the contract event 0x5bd3547877c38b4ee6fc63a44b7d7846debe64218c29e930faacba7fcfac1db9.
//
// Solidity: event ClaimStaged(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
func (_IConsensus *IConsensusFilterer) FilterClaimStaged(opts *bind.FilterOpts, appContract []common.Address) (*IConsensusClaimStagedIterator, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IConsensus.contract.FilterLogs(opts, "ClaimStaged", appContractRule)
	if err != nil {
		return nil, err
	}
	return &IConsensusClaimStagedIterator{contract: _IConsensus.contract, event: "ClaimStaged", logs: logs, sub: sub}, nil
}

// WatchClaimStaged is a free log subscription operation binding the contract event 0x5bd3547877c38b4ee6fc63a44b7d7846debe64218c29e930faacba7fcfac1db9.
//
// Solidity: event ClaimStaged(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
func (_IConsensus *IConsensusFilterer) WatchClaimStaged(opts *bind.WatchOpts, sink chan<- *IConsensusClaimStaged, appContract []common.Address) (event.Subscription, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IConsensus.contract.WatchLogs(opts, "ClaimStaged", appContractRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IConsensusClaimStaged)
				if err := _IConsensus.contract.UnpackLog(event, "ClaimStaged", log); err != nil {
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

// ParseClaimStaged is a log parse operation binding the contract event 0x5bd3547877c38b4ee6fc63a44b7d7846debe64218c29e930faacba7fcfac1db9.
//
// Solidity: event ClaimStaged(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
func (_IConsensus *IConsensusFilterer) ParseClaimStaged(log types.Log) (*IConsensusClaimStaged, error) {
	event := new(IConsensusClaimStaged)
	if err := _IConsensus.contract.UnpackLog(event, "ClaimStaged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IConsensusClaimSubmittedIterator is returned from FilterClaimSubmitted and is used to iterate over the raw logs and unpacked data for ClaimSubmitted events raised by the IConsensus contract.
type IConsensusClaimSubmittedIterator struct {
	Event *IConsensusClaimSubmitted // Event containing the contract specifics and raw log

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
func (it *IConsensusClaimSubmittedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IConsensusClaimSubmitted)
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
		it.Event = new(IConsensusClaimSubmitted)
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
func (it *IConsensusClaimSubmittedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IConsensusClaimSubmittedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IConsensusClaimSubmitted represents a ClaimSubmitted event raised by the IConsensus contract.
type IConsensusClaimSubmitted struct {
	Submitter                common.Address
	AppContract              common.Address
	LastProcessedBlockNumber *big.Int
	OutputsMerkleRoot        [32]byte
	MachineMerkleRoot        [32]byte
	Raw                      types.Log // Blockchain specific contextual infos
}

// FilterClaimSubmitted is a free log retrieval operation binding the contract event 0x9d98f38c9329d29c2204350787eae2783da0002dbe80097dab5a71057c6573bb.
//
// Solidity: event ClaimSubmitted(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
func (_IConsensus *IConsensusFilterer) FilterClaimSubmitted(opts *bind.FilterOpts, submitter []common.Address, appContract []common.Address) (*IConsensusClaimSubmittedIterator, error) {

	var submitterRule []interface{}
	for _, submitterItem := range submitter {
		submitterRule = append(submitterRule, submitterItem)
	}
	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IConsensus.contract.FilterLogs(opts, "ClaimSubmitted", submitterRule, appContractRule)
	if err != nil {
		return nil, err
	}
	return &IConsensusClaimSubmittedIterator{contract: _IConsensus.contract, event: "ClaimSubmitted", logs: logs, sub: sub}, nil
}

// WatchClaimSubmitted is a free log subscription operation binding the contract event 0x9d98f38c9329d29c2204350787eae2783da0002dbe80097dab5a71057c6573bb.
//
// Solidity: event ClaimSubmitted(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
func (_IConsensus *IConsensusFilterer) WatchClaimSubmitted(opts *bind.WatchOpts, sink chan<- *IConsensusClaimSubmitted, submitter []common.Address, appContract []common.Address) (event.Subscription, error) {

	var submitterRule []interface{}
	for _, submitterItem := range submitter {
		submitterRule = append(submitterRule, submitterItem)
	}
	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IConsensus.contract.WatchLogs(opts, "ClaimSubmitted", submitterRule, appContractRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IConsensusClaimSubmitted)
				if err := _IConsensus.contract.UnpackLog(event, "ClaimSubmitted", log); err != nil {
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

// ParseClaimSubmitted is a log parse operation binding the contract event 0x9d98f38c9329d29c2204350787eae2783da0002dbe80097dab5a71057c6573bb.
//
// Solidity: event ClaimSubmitted(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
func (_IConsensus *IConsensusFilterer) ParseClaimSubmitted(log types.Log) (*IConsensusClaimSubmitted, error) {
	event := new(IConsensusClaimSubmitted)
	if err := _IConsensus.contract.UnpackLog(event, "ClaimSubmitted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
