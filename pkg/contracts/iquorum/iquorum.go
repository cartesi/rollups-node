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

// IConsensusClaim is an auto generated low-level Go binding around an user-defined struct.
type IConsensusClaim struct {
	Status                  uint8
	StagingBlockNumber      *big.Int
	StagedOutputsMerkleRoot [32]byte
}

// IQuorumMetaData contains all meta data concerning the IQuorum contract.
var IQuorumMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"acceptClaim\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getClaim\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"claim\",\"type\":\"tuple\",\"internalType\":\"structIConsensus.Claim\",\"components\":[{\"name\":\"status\",\"type\":\"uint8\",\"internalType\":\"enumIConsensus.ClaimStatus\"},{\"name\":\"stagingBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"stagedOutputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getClaimStagingPeriod\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getEpochLength\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getLastFinalizedMachineMerkleRoot\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNumberOfAcceptedClaims\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNumberOfStagedClaims\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNumberOfSubmittedClaims\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isOutputsMerkleRootValid\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isValidatorInFavorOf\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"id\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isValidatorInFavorOfAnyClaimInEpoch\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"id\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"numOfValidators\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"numOfValidatorsInFavorOf\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"numOfValidatorsInFavorOfAnyClaimInEpoch\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"submitClaim\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"proof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"supportsInterface\",\"inputs\":[{\"name\":\"interfaceId\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"validatorById\",\"inputs\":[{\"name\":\"id\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"validatorId\",\"inputs\":[{\"name\":\"validator\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"major\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"minor\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"patch\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"preRelease\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"buildMetadata\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"ClaimAccepted\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ClaimStaged\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ClaimSubmitted\",\"inputs\":[{\"name\":\"submitter\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"ApplicationForeclosed\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ApplicationNotDeployed\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ApplicationReverted\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"error\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"CallerIsNotValidator\",\"inputs\":[{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ClaimNotStaged\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"claimStatus\",\"type\":\"uint8\",\"internalType\":\"enumIConsensus.ClaimStatus\"}]},{\"type\":\"error\",\"name\":\"ClaimStagingPeriodNotOverYet\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"numberOfBlocksAfterStaging\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"claimStagingPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"IllformedApplicationReturnData\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"InvalidOutputsMerkleRootProofSize\",\"inputs\":[{\"name\":\"suppliedProofSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedProofSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NotEpochFinalBlock\",\"inputs\":[{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"epochLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NotFirstClaim\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NotPastBlock\",\"inputs\":[{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]",
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

// GetClaim is a free data retrieval call binding the contract method 0xa1abc0ae.
//
// Solidity: function getClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) view returns((uint8,uint256,bytes32) claim)
func (_IQuorum *IQuorumCaller) GetClaim(opts *bind.CallOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (IConsensusClaim, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "getClaim", appContract, lastProcessedBlockNumber, machineMerkleRoot)

	if err != nil {
		return *new(IConsensusClaim), err
	}

	out0 := *abi.ConvertType(out[0], new(IConsensusClaim)).(*IConsensusClaim)

	return out0, err

}

// GetClaim is a free data retrieval call binding the contract method 0xa1abc0ae.
//
// Solidity: function getClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) view returns((uint8,uint256,bytes32) claim)
func (_IQuorum *IQuorumSession) GetClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (IConsensusClaim, error) {
	return _IQuorum.Contract.GetClaim(&_IQuorum.CallOpts, appContract, lastProcessedBlockNumber, machineMerkleRoot)
}

// GetClaim is a free data retrieval call binding the contract method 0xa1abc0ae.
//
// Solidity: function getClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) view returns((uint8,uint256,bytes32) claim)
func (_IQuorum *IQuorumCallerSession) GetClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (IConsensusClaim, error) {
	return _IQuorum.Contract.GetClaim(&_IQuorum.CallOpts, appContract, lastProcessedBlockNumber, machineMerkleRoot)
}

// GetClaimStagingPeriod is a free data retrieval call binding the contract method 0xa04c6564.
//
// Solidity: function getClaimStagingPeriod() view returns(uint256)
func (_IQuorum *IQuorumCaller) GetClaimStagingPeriod(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "getClaimStagingPeriod")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetClaimStagingPeriod is a free data retrieval call binding the contract method 0xa04c6564.
//
// Solidity: function getClaimStagingPeriod() view returns(uint256)
func (_IQuorum *IQuorumSession) GetClaimStagingPeriod() (*big.Int, error) {
	return _IQuorum.Contract.GetClaimStagingPeriod(&_IQuorum.CallOpts)
}

// GetClaimStagingPeriod is a free data retrieval call binding the contract method 0xa04c6564.
//
// Solidity: function getClaimStagingPeriod() view returns(uint256)
func (_IQuorum *IQuorumCallerSession) GetClaimStagingPeriod() (*big.Int, error) {
	return _IQuorum.Contract.GetClaimStagingPeriod(&_IQuorum.CallOpts)
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

// GetLastFinalizedMachineMerkleRoot is a free data retrieval call binding the contract method 0x5ac9cfbf.
//
// Solidity: function getLastFinalizedMachineMerkleRoot(address appContract) view returns(bytes32)
func (_IQuorum *IQuorumCaller) GetLastFinalizedMachineMerkleRoot(opts *bind.CallOpts, appContract common.Address) ([32]byte, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "getLastFinalizedMachineMerkleRoot", appContract)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetLastFinalizedMachineMerkleRoot is a free data retrieval call binding the contract method 0x5ac9cfbf.
//
// Solidity: function getLastFinalizedMachineMerkleRoot(address appContract) view returns(bytes32)
func (_IQuorum *IQuorumSession) GetLastFinalizedMachineMerkleRoot(appContract common.Address) ([32]byte, error) {
	return _IQuorum.Contract.GetLastFinalizedMachineMerkleRoot(&_IQuorum.CallOpts, appContract)
}

// GetLastFinalizedMachineMerkleRoot is a free data retrieval call binding the contract method 0x5ac9cfbf.
//
// Solidity: function getLastFinalizedMachineMerkleRoot(address appContract) view returns(bytes32)
func (_IQuorum *IQuorumCallerSession) GetLastFinalizedMachineMerkleRoot(appContract common.Address) ([32]byte, error) {
	return _IQuorum.Contract.GetLastFinalizedMachineMerkleRoot(&_IQuorum.CallOpts, appContract)
}

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0x80a80953.
//
// Solidity: function getNumberOfAcceptedClaims(address appContract) view returns(uint256)
func (_IQuorum *IQuorumCaller) GetNumberOfAcceptedClaims(opts *bind.CallOpts, appContract common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "getNumberOfAcceptedClaims", appContract)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0x80a80953.
//
// Solidity: function getNumberOfAcceptedClaims(address appContract) view returns(uint256)
func (_IQuorum *IQuorumSession) GetNumberOfAcceptedClaims(appContract common.Address) (*big.Int, error) {
	return _IQuorum.Contract.GetNumberOfAcceptedClaims(&_IQuorum.CallOpts, appContract)
}

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0x80a80953.
//
// Solidity: function getNumberOfAcceptedClaims(address appContract) view returns(uint256)
func (_IQuorum *IQuorumCallerSession) GetNumberOfAcceptedClaims(appContract common.Address) (*big.Int, error) {
	return _IQuorum.Contract.GetNumberOfAcceptedClaims(&_IQuorum.CallOpts, appContract)
}

// GetNumberOfStagedClaims is a free data retrieval call binding the contract method 0x02c657a4.
//
// Solidity: function getNumberOfStagedClaims(address appContract) view returns(uint256)
func (_IQuorum *IQuorumCaller) GetNumberOfStagedClaims(opts *bind.CallOpts, appContract common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "getNumberOfStagedClaims", appContract)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNumberOfStagedClaims is a free data retrieval call binding the contract method 0x02c657a4.
//
// Solidity: function getNumberOfStagedClaims(address appContract) view returns(uint256)
func (_IQuorum *IQuorumSession) GetNumberOfStagedClaims(appContract common.Address) (*big.Int, error) {
	return _IQuorum.Contract.GetNumberOfStagedClaims(&_IQuorum.CallOpts, appContract)
}

// GetNumberOfStagedClaims is a free data retrieval call binding the contract method 0x02c657a4.
//
// Solidity: function getNumberOfStagedClaims(address appContract) view returns(uint256)
func (_IQuorum *IQuorumCallerSession) GetNumberOfStagedClaims(appContract common.Address) (*big.Int, error) {
	return _IQuorum.Contract.GetNumberOfStagedClaims(&_IQuorum.CallOpts, appContract)
}

// GetNumberOfSubmittedClaims is a free data retrieval call binding the contract method 0x43aacc77.
//
// Solidity: function getNumberOfSubmittedClaims(address appContract) view returns(uint256)
func (_IQuorum *IQuorumCaller) GetNumberOfSubmittedClaims(opts *bind.CallOpts, appContract common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "getNumberOfSubmittedClaims", appContract)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNumberOfSubmittedClaims is a free data retrieval call binding the contract method 0x43aacc77.
//
// Solidity: function getNumberOfSubmittedClaims(address appContract) view returns(uint256)
func (_IQuorum *IQuorumSession) GetNumberOfSubmittedClaims(appContract common.Address) (*big.Int, error) {
	return _IQuorum.Contract.GetNumberOfSubmittedClaims(&_IQuorum.CallOpts, appContract)
}

// GetNumberOfSubmittedClaims is a free data retrieval call binding the contract method 0x43aacc77.
//
// Solidity: function getNumberOfSubmittedClaims(address appContract) view returns(uint256)
func (_IQuorum *IQuorumCallerSession) GetNumberOfSubmittedClaims(appContract common.Address) (*big.Int, error) {
	return _IQuorum.Contract.GetNumberOfSubmittedClaims(&_IQuorum.CallOpts, appContract)
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
// Solidity: function isValidatorInFavorOf(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot, uint256 id) view returns(bool)
func (_IQuorum *IQuorumCaller) IsValidatorInFavorOf(opts *bind.CallOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte, id *big.Int) (bool, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "isValidatorInFavorOf", appContract, lastProcessedBlockNumber, machineMerkleRoot, id)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsValidatorInFavorOf is a free data retrieval call binding the contract method 0x4b84231c.
//
// Solidity: function isValidatorInFavorOf(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot, uint256 id) view returns(bool)
func (_IQuorum *IQuorumSession) IsValidatorInFavorOf(appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte, id *big.Int) (bool, error) {
	return _IQuorum.Contract.IsValidatorInFavorOf(&_IQuorum.CallOpts, appContract, lastProcessedBlockNumber, machineMerkleRoot, id)
}

// IsValidatorInFavorOf is a free data retrieval call binding the contract method 0x4b84231c.
//
// Solidity: function isValidatorInFavorOf(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot, uint256 id) view returns(bool)
func (_IQuorum *IQuorumCallerSession) IsValidatorInFavorOf(appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte, id *big.Int) (bool, error) {
	return _IQuorum.Contract.IsValidatorInFavorOf(&_IQuorum.CallOpts, appContract, lastProcessedBlockNumber, machineMerkleRoot, id)
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
// Solidity: function numOfValidatorsInFavorOf(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) view returns(uint256)
func (_IQuorum *IQuorumCaller) NumOfValidatorsInFavorOf(opts *bind.CallOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "numOfValidatorsInFavorOf", appContract, lastProcessedBlockNumber, machineMerkleRoot)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// NumOfValidatorsInFavorOf is a free data retrieval call binding the contract method 0x7051bfd5.
//
// Solidity: function numOfValidatorsInFavorOf(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) view returns(uint256)
func (_IQuorum *IQuorumSession) NumOfValidatorsInFavorOf(appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (*big.Int, error) {
	return _IQuorum.Contract.NumOfValidatorsInFavorOf(&_IQuorum.CallOpts, appContract, lastProcessedBlockNumber, machineMerkleRoot)
}

// NumOfValidatorsInFavorOf is a free data retrieval call binding the contract method 0x7051bfd5.
//
// Solidity: function numOfValidatorsInFavorOf(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) view returns(uint256)
func (_IQuorum *IQuorumCallerSession) NumOfValidatorsInFavorOf(appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (*big.Int, error) {
	return _IQuorum.Contract.NumOfValidatorsInFavorOf(&_IQuorum.CallOpts, appContract, lastProcessedBlockNumber, machineMerkleRoot)
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

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IQuorum *IQuorumCaller) Version(opts *bind.CallOpts) (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	var out []interface{}
	err := _IQuorum.contract.Call(opts, &out, "version")

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
func (_IQuorum *IQuorumSession) Version() (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	return _IQuorum.Contract.Version(&_IQuorum.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IQuorum *IQuorumCallerSession) Version() (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	return _IQuorum.Contract.Version(&_IQuorum.CallOpts)
}

// AcceptClaim is a paid mutator transaction binding the contract method 0x8e2c381c.
//
// Solidity: function acceptClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) returns()
func (_IQuorum *IQuorumTransactor) AcceptClaim(opts *bind.TransactOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IQuorum.contract.Transact(opts, "acceptClaim", appContract, lastProcessedBlockNumber, machineMerkleRoot)
}

// AcceptClaim is a paid mutator transaction binding the contract method 0x8e2c381c.
//
// Solidity: function acceptClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) returns()
func (_IQuorum *IQuorumSession) AcceptClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IQuorum.Contract.AcceptClaim(&_IQuorum.TransactOpts, appContract, lastProcessedBlockNumber, machineMerkleRoot)
}

// AcceptClaim is a paid mutator transaction binding the contract method 0x8e2c381c.
//
// Solidity: function acceptClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 machineMerkleRoot) returns()
func (_IQuorum *IQuorumTransactorSession) AcceptClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, machineMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IQuorum.Contract.AcceptClaim(&_IQuorum.TransactOpts, appContract, lastProcessedBlockNumber, machineMerkleRoot)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x9a00db83.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32[] proof) returns()
func (_IQuorum *IQuorumTransactor) SubmitClaim(opts *bind.TransactOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _IQuorum.contract.Transact(opts, "submitClaim", appContract, lastProcessedBlockNumber, outputsMerkleRoot, proof)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x9a00db83.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32[] proof) returns()
func (_IQuorum *IQuorumSession) SubmitClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _IQuorum.Contract.SubmitClaim(&_IQuorum.TransactOpts, appContract, lastProcessedBlockNumber, outputsMerkleRoot, proof)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x9a00db83.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32[] proof) returns()
func (_IQuorum *IQuorumTransactorSession) SubmitClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _IQuorum.Contract.SubmitClaim(&_IQuorum.TransactOpts, appContract, lastProcessedBlockNumber, outputsMerkleRoot, proof)
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
	MachineMerkleRoot        [32]byte
	Raw                      types.Log // Blockchain specific contextual infos
}

// FilterClaimAccepted is a free log retrieval operation binding the contract event 0x8d40f6fff97997587f3d67c44cf2201ae7df0ef9a14ac7399b9a6f0fcaa3c46f.
//
// Solidity: event ClaimAccepted(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
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

// WatchClaimAccepted is a free log subscription operation binding the contract event 0x8d40f6fff97997587f3d67c44cf2201ae7df0ef9a14ac7399b9a6f0fcaa3c46f.
//
// Solidity: event ClaimAccepted(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
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

// ParseClaimAccepted is a log parse operation binding the contract event 0x8d40f6fff97997587f3d67c44cf2201ae7df0ef9a14ac7399b9a6f0fcaa3c46f.
//
// Solidity: event ClaimAccepted(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
func (_IQuorum *IQuorumFilterer) ParseClaimAccepted(log types.Log) (*IQuorumClaimAccepted, error) {
	event := new(IQuorumClaimAccepted)
	if err := _IQuorum.contract.UnpackLog(event, "ClaimAccepted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IQuorumClaimStagedIterator is returned from FilterClaimStaged and is used to iterate over the raw logs and unpacked data for ClaimStaged events raised by the IQuorum contract.
type IQuorumClaimStagedIterator struct {
	Event *IQuorumClaimStaged // Event containing the contract specifics and raw log

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
func (it *IQuorumClaimStagedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IQuorumClaimStaged)
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
		it.Event = new(IQuorumClaimStaged)
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
func (it *IQuorumClaimStagedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IQuorumClaimStagedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IQuorumClaimStaged represents a ClaimStaged event raised by the IQuorum contract.
type IQuorumClaimStaged struct {
	AppContract              common.Address
	LastProcessedBlockNumber *big.Int
	OutputsMerkleRoot        [32]byte
	MachineMerkleRoot        [32]byte
	Raw                      types.Log // Blockchain specific contextual infos
}

// FilterClaimStaged is a free log retrieval operation binding the contract event 0x5bd3547877c38b4ee6fc63a44b7d7846debe64218c29e930faacba7fcfac1db9.
//
// Solidity: event ClaimStaged(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
func (_IQuorum *IQuorumFilterer) FilterClaimStaged(opts *bind.FilterOpts, appContract []common.Address) (*IQuorumClaimStagedIterator, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IQuorum.contract.FilterLogs(opts, "ClaimStaged", appContractRule)
	if err != nil {
		return nil, err
	}
	return &IQuorumClaimStagedIterator{contract: _IQuorum.contract, event: "ClaimStaged", logs: logs, sub: sub}, nil
}

// WatchClaimStaged is a free log subscription operation binding the contract event 0x5bd3547877c38b4ee6fc63a44b7d7846debe64218c29e930faacba7fcfac1db9.
//
// Solidity: event ClaimStaged(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
func (_IQuorum *IQuorumFilterer) WatchClaimStaged(opts *bind.WatchOpts, sink chan<- *IQuorumClaimStaged, appContract []common.Address) (event.Subscription, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IQuorum.contract.WatchLogs(opts, "ClaimStaged", appContractRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IQuorumClaimStaged)
				if err := _IQuorum.contract.UnpackLog(event, "ClaimStaged", log); err != nil {
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
func (_IQuorum *IQuorumFilterer) ParseClaimStaged(log types.Log) (*IQuorumClaimStaged, error) {
	event := new(IQuorumClaimStaged)
	if err := _IQuorum.contract.UnpackLog(event, "ClaimStaged", log); err != nil {
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
	MachineMerkleRoot        [32]byte
	Raw                      types.Log // Blockchain specific contextual infos
}

// FilterClaimSubmitted is a free log retrieval operation binding the contract event 0x9d98f38c9329d29c2204350787eae2783da0002dbe80097dab5a71057c6573bb.
//
// Solidity: event ClaimSubmitted(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
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

// WatchClaimSubmitted is a free log subscription operation binding the contract event 0x9d98f38c9329d29c2204350787eae2783da0002dbe80097dab5a71057c6573bb.
//
// Solidity: event ClaimSubmitted(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
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

// ParseClaimSubmitted is a log parse operation binding the contract event 0x9d98f38c9329d29c2204350787eae2783da0002dbe80097dab5a71057c6573bb.
//
// Solidity: event ClaimSubmitted(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot, bytes32 machineMerkleRoot)
func (_IQuorum *IQuorumFilterer) ParseClaimSubmitted(log types.Log) (*IQuorumClaimSubmitted, error) {
	event := new(IQuorumClaimSubmitted)
	if err := _IQuorum.contract.UnpackLog(event, "ClaimSubmitted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
