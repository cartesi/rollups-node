// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package idaveconsensus

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

// IDaveConsensusMetaData contains all meta data concerning the IDaveConsensus contract.
var IDaveConsensusMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"canSettle\",\"inputs\":[],\"outputs\":[{\"name\":\"isFinished\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"winnerCommitment\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getApplicationContract\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getCurrentSealedEpoch\",\"inputs\":[],\"outputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"inputIndexLowerBound\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"inputIndexUpperBound\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"tournament\",\"type\":\"address\",\"internalType\":\"contractITournament\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDeploymentBlockNumber\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getInputBox\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractIInputBox\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getLastFinalizedMachineMerkleRoot\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getTournamentFactory\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractITournamentFactory\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isOutputsMerkleRootValid\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"provideMerkleRootOfInput\",\"inputs\":[{\"name\":\"inputIndexWithinEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"input\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"settle\",\"inputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"proof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"supportsInterface\",\"inputs\":[{\"name\":\"interfaceId\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"ConsensusCreation\",\"inputs\":[{\"name\":\"inputBox\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractIInputBox\"},{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"tournamentFactory\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractITournamentFactory\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"EpochSealed\",\"inputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"inputIndexLowerBound\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"inputIndexUpperBound\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"initialMachineStateHash\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Machine.Hash\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"},{\"name\":\"tournament\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractITournament\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"ApplicationForeclosed\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ApplicationMismatch\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"received\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ApplicationNotDeployed\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ApplicationReverted\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"error\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"DataBlockTooLarge\",\"inputs\":[{\"name\":\"log2DataBlockSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxLog2DataBlockSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DriveSmallerThanData\",\"inputs\":[{\"name\":\"driveSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"dataSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DriveSmallerThanDataBlock\",\"inputs\":[{\"name\":\"log2DriveSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"log2DataBlockSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DriveTooLarge\",\"inputs\":[{\"name\":\"log2DriveSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxLog2DriveSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"IllformedApplicationReturnData\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"IncorrectEpochNumber\",\"inputs\":[{\"name\":\"received\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InputHashMismatch\",\"inputs\":[{\"name\":\"fromReceivedInput\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"fromInputBox\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"InvalidNodeIndex\",\"inputs\":[{\"name\":\"nodeIndex\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"height\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidOutputsMerkleRootProof\",\"inputs\":[{\"name\":\"settledState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}]},{\"type\":\"error\",\"name\":\"InvalidOutputsMerkleRootProofSize\",\"inputs\":[{\"name\":\"suppliedProofSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"TournamentNotFinishedYet\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"UnexpectedFinalStackDepth\",\"inputs\":[{\"name\":\"stackDepth\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]",
}

// IDaveConsensusABI is the input ABI used to generate the binding from.
// Deprecated: Use IDaveConsensusMetaData.ABI instead.
var IDaveConsensusABI = IDaveConsensusMetaData.ABI

// IDaveConsensus is an auto generated Go binding around an Ethereum contract.
type IDaveConsensus struct {
	IDaveConsensusCaller     // Read-only binding to the contract
	IDaveConsensusTransactor // Write-only binding to the contract
	IDaveConsensusFilterer   // Log filterer for contract events
}

// IDaveConsensusCaller is an auto generated read-only Go binding around an Ethereum contract.
type IDaveConsensusCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IDaveConsensusTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IDaveConsensusTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IDaveConsensusFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IDaveConsensusFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IDaveConsensusSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IDaveConsensusSession struct {
	Contract     *IDaveConsensus   // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IDaveConsensusCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IDaveConsensusCallerSession struct {
	Contract *IDaveConsensusCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts         // Call options to use throughout this session
}

// IDaveConsensusTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IDaveConsensusTransactorSession struct {
	Contract     *IDaveConsensusTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts         // Transaction auth options to use throughout this session
}

// IDaveConsensusRaw is an auto generated low-level Go binding around an Ethereum contract.
type IDaveConsensusRaw struct {
	Contract *IDaveConsensus // Generic contract binding to access the raw methods on
}

// IDaveConsensusCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IDaveConsensusCallerRaw struct {
	Contract *IDaveConsensusCaller // Generic read-only contract binding to access the raw methods on
}

// IDaveConsensusTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IDaveConsensusTransactorRaw struct {
	Contract *IDaveConsensusTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIDaveConsensus creates a new instance of IDaveConsensus, bound to a specific deployed contract.
func NewIDaveConsensus(address common.Address, backend bind.ContractBackend) (*IDaveConsensus, error) {
	contract, err := bindIDaveConsensus(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IDaveConsensus{IDaveConsensusCaller: IDaveConsensusCaller{contract: contract}, IDaveConsensusTransactor: IDaveConsensusTransactor{contract: contract}, IDaveConsensusFilterer: IDaveConsensusFilterer{contract: contract}}, nil
}

// NewIDaveConsensusCaller creates a new read-only instance of IDaveConsensus, bound to a specific deployed contract.
func NewIDaveConsensusCaller(address common.Address, caller bind.ContractCaller) (*IDaveConsensusCaller, error) {
	contract, err := bindIDaveConsensus(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IDaveConsensusCaller{contract: contract}, nil
}

// NewIDaveConsensusTransactor creates a new write-only instance of IDaveConsensus, bound to a specific deployed contract.
func NewIDaveConsensusTransactor(address common.Address, transactor bind.ContractTransactor) (*IDaveConsensusTransactor, error) {
	contract, err := bindIDaveConsensus(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IDaveConsensusTransactor{contract: contract}, nil
}

// NewIDaveConsensusFilterer creates a new log filterer instance of IDaveConsensus, bound to a specific deployed contract.
func NewIDaveConsensusFilterer(address common.Address, filterer bind.ContractFilterer) (*IDaveConsensusFilterer, error) {
	contract, err := bindIDaveConsensus(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IDaveConsensusFilterer{contract: contract}, nil
}

// bindIDaveConsensus binds a generic wrapper to an already deployed contract.
func bindIDaveConsensus(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IDaveConsensusMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IDaveConsensus *IDaveConsensusRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IDaveConsensus.Contract.IDaveConsensusCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IDaveConsensus *IDaveConsensusRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.IDaveConsensusTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IDaveConsensus *IDaveConsensusRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.IDaveConsensusTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IDaveConsensus *IDaveConsensusCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IDaveConsensus.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IDaveConsensus *IDaveConsensusTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IDaveConsensus *IDaveConsensusTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.contract.Transact(opts, method, params...)
}

// CanSettle is a free data retrieval call binding the contract method 0xfaf7ba6a.
//
// Solidity: function canSettle() view returns(bool isFinished, uint256 epochNumber, bytes32 winnerCommitment)
func (_IDaveConsensus *IDaveConsensusCaller) CanSettle(opts *bind.CallOpts) (struct {
	IsFinished       bool
	EpochNumber      *big.Int
	WinnerCommitment [32]byte
}, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "canSettle")

	outstruct := new(struct {
		IsFinished       bool
		EpochNumber      *big.Int
		WinnerCommitment [32]byte
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.IsFinished = *abi.ConvertType(out[0], new(bool)).(*bool)
	outstruct.EpochNumber = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)
	outstruct.WinnerCommitment = *abi.ConvertType(out[2], new([32]byte)).(*[32]byte)

	return *outstruct, err

}

// CanSettle is a free data retrieval call binding the contract method 0xfaf7ba6a.
//
// Solidity: function canSettle() view returns(bool isFinished, uint256 epochNumber, bytes32 winnerCommitment)
func (_IDaveConsensus *IDaveConsensusSession) CanSettle() (struct {
	IsFinished       bool
	EpochNumber      *big.Int
	WinnerCommitment [32]byte
}, error) {
	return _IDaveConsensus.Contract.CanSettle(&_IDaveConsensus.CallOpts)
}

// CanSettle is a free data retrieval call binding the contract method 0xfaf7ba6a.
//
// Solidity: function canSettle() view returns(bool isFinished, uint256 epochNumber, bytes32 winnerCommitment)
func (_IDaveConsensus *IDaveConsensusCallerSession) CanSettle() (struct {
	IsFinished       bool
	EpochNumber      *big.Int
	WinnerCommitment [32]byte
}, error) {
	return _IDaveConsensus.Contract.CanSettle(&_IDaveConsensus.CallOpts)
}

// GetApplicationContract is a free data retrieval call binding the contract method 0xc050be00.
//
// Solidity: function getApplicationContract() view returns(address)
func (_IDaveConsensus *IDaveConsensusCaller) GetApplicationContract(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getApplicationContract")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetApplicationContract is a free data retrieval call binding the contract method 0xc050be00.
//
// Solidity: function getApplicationContract() view returns(address)
func (_IDaveConsensus *IDaveConsensusSession) GetApplicationContract() (common.Address, error) {
	return _IDaveConsensus.Contract.GetApplicationContract(&_IDaveConsensus.CallOpts)
}

// GetApplicationContract is a free data retrieval call binding the contract method 0xc050be00.
//
// Solidity: function getApplicationContract() view returns(address)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetApplicationContract() (common.Address, error) {
	return _IDaveConsensus.Contract.GetApplicationContract(&_IDaveConsensus.CallOpts)
}

// GetCurrentSealedEpoch is a free data retrieval call binding the contract method 0x1239acd9.
//
// Solidity: function getCurrentSealedEpoch() view returns(uint256 epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, address tournament)
func (_IDaveConsensus *IDaveConsensusCaller) GetCurrentSealedEpoch(opts *bind.CallOpts) (struct {
	EpochNumber          *big.Int
	InputIndexLowerBound *big.Int
	InputIndexUpperBound *big.Int
	Tournament           common.Address
}, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getCurrentSealedEpoch")

	outstruct := new(struct {
		EpochNumber          *big.Int
		InputIndexLowerBound *big.Int
		InputIndexUpperBound *big.Int
		Tournament           common.Address
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.EpochNumber = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.InputIndexLowerBound = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)
	outstruct.InputIndexUpperBound = *abi.ConvertType(out[2], new(*big.Int)).(**big.Int)
	outstruct.Tournament = *abi.ConvertType(out[3], new(common.Address)).(*common.Address)

	return *outstruct, err

}

// GetCurrentSealedEpoch is a free data retrieval call binding the contract method 0x1239acd9.
//
// Solidity: function getCurrentSealedEpoch() view returns(uint256 epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, address tournament)
func (_IDaveConsensus *IDaveConsensusSession) GetCurrentSealedEpoch() (struct {
	EpochNumber          *big.Int
	InputIndexLowerBound *big.Int
	InputIndexUpperBound *big.Int
	Tournament           common.Address
}, error) {
	return _IDaveConsensus.Contract.GetCurrentSealedEpoch(&_IDaveConsensus.CallOpts)
}

// GetCurrentSealedEpoch is a free data retrieval call binding the contract method 0x1239acd9.
//
// Solidity: function getCurrentSealedEpoch() view returns(uint256 epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, address tournament)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetCurrentSealedEpoch() (struct {
	EpochNumber          *big.Int
	InputIndexLowerBound *big.Int
	InputIndexUpperBound *big.Int
	Tournament           common.Address
}, error) {
	return _IDaveConsensus.Contract.GetCurrentSealedEpoch(&_IDaveConsensus.CallOpts)
}

// GetDeploymentBlockNumber is a free data retrieval call binding the contract method 0xb3a1acd8.
//
// Solidity: function getDeploymentBlockNumber() view returns(uint256)
func (_IDaveConsensus *IDaveConsensusCaller) GetDeploymentBlockNumber(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getDeploymentBlockNumber")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetDeploymentBlockNumber is a free data retrieval call binding the contract method 0xb3a1acd8.
//
// Solidity: function getDeploymentBlockNumber() view returns(uint256)
func (_IDaveConsensus *IDaveConsensusSession) GetDeploymentBlockNumber() (*big.Int, error) {
	return _IDaveConsensus.Contract.GetDeploymentBlockNumber(&_IDaveConsensus.CallOpts)
}

// GetDeploymentBlockNumber is a free data retrieval call binding the contract method 0xb3a1acd8.
//
// Solidity: function getDeploymentBlockNumber() view returns(uint256)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetDeploymentBlockNumber() (*big.Int, error) {
	return _IDaveConsensus.Contract.GetDeploymentBlockNumber(&_IDaveConsensus.CallOpts)
}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_IDaveConsensus *IDaveConsensusCaller) GetInputBox(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getInputBox")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_IDaveConsensus *IDaveConsensusSession) GetInputBox() (common.Address, error) {
	return _IDaveConsensus.Contract.GetInputBox(&_IDaveConsensus.CallOpts)
}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetInputBox() (common.Address, error) {
	return _IDaveConsensus.Contract.GetInputBox(&_IDaveConsensus.CallOpts)
}

// GetLastFinalizedMachineMerkleRoot is a free data retrieval call binding the contract method 0x5ac9cfbf.
//
// Solidity: function getLastFinalizedMachineMerkleRoot(address appContract) view returns(bytes32)
func (_IDaveConsensus *IDaveConsensusCaller) GetLastFinalizedMachineMerkleRoot(opts *bind.CallOpts, appContract common.Address) ([32]byte, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getLastFinalizedMachineMerkleRoot", appContract)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetLastFinalizedMachineMerkleRoot is a free data retrieval call binding the contract method 0x5ac9cfbf.
//
// Solidity: function getLastFinalizedMachineMerkleRoot(address appContract) view returns(bytes32)
func (_IDaveConsensus *IDaveConsensusSession) GetLastFinalizedMachineMerkleRoot(appContract common.Address) ([32]byte, error) {
	return _IDaveConsensus.Contract.GetLastFinalizedMachineMerkleRoot(&_IDaveConsensus.CallOpts, appContract)
}

// GetLastFinalizedMachineMerkleRoot is a free data retrieval call binding the contract method 0x5ac9cfbf.
//
// Solidity: function getLastFinalizedMachineMerkleRoot(address appContract) view returns(bytes32)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetLastFinalizedMachineMerkleRoot(appContract common.Address) ([32]byte, error) {
	return _IDaveConsensus.Contract.GetLastFinalizedMachineMerkleRoot(&_IDaveConsensus.CallOpts, appContract)
}

// GetTournamentFactory is a free data retrieval call binding the contract method 0x813a1aaf.
//
// Solidity: function getTournamentFactory() view returns(address)
func (_IDaveConsensus *IDaveConsensusCaller) GetTournamentFactory(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "getTournamentFactory")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetTournamentFactory is a free data retrieval call binding the contract method 0x813a1aaf.
//
// Solidity: function getTournamentFactory() view returns(address)
func (_IDaveConsensus *IDaveConsensusSession) GetTournamentFactory() (common.Address, error) {
	return _IDaveConsensus.Contract.GetTournamentFactory(&_IDaveConsensus.CallOpts)
}

// GetTournamentFactory is a free data retrieval call binding the contract method 0x813a1aaf.
//
// Solidity: function getTournamentFactory() view returns(address)
func (_IDaveConsensus *IDaveConsensusCallerSession) GetTournamentFactory() (common.Address, error) {
	return _IDaveConsensus.Contract.GetTournamentFactory(&_IDaveConsensus.CallOpts)
}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IDaveConsensus *IDaveConsensusCaller) IsOutputsMerkleRootValid(opts *bind.CallOpts, appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "isOutputsMerkleRootValid", appContract, outputsMerkleRoot)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IDaveConsensus *IDaveConsensusSession) IsOutputsMerkleRootValid(appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	return _IDaveConsensus.Contract.IsOutputsMerkleRootValid(&_IDaveConsensus.CallOpts, appContract, outputsMerkleRoot)
}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IDaveConsensus *IDaveConsensusCallerSession) IsOutputsMerkleRootValid(appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	return _IDaveConsensus.Contract.IsOutputsMerkleRootValid(&_IDaveConsensus.CallOpts, appContract, outputsMerkleRoot)
}

// ProvideMerkleRootOfInput is a free data retrieval call binding the contract method 0x7a96f480.
//
// Solidity: function provideMerkleRootOfInput(uint256 inputIndexWithinEpoch, bytes input) view returns(bytes32)
func (_IDaveConsensus *IDaveConsensusCaller) ProvideMerkleRootOfInput(opts *bind.CallOpts, inputIndexWithinEpoch *big.Int, input []byte) ([32]byte, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "provideMerkleRootOfInput", inputIndexWithinEpoch, input)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// ProvideMerkleRootOfInput is a free data retrieval call binding the contract method 0x7a96f480.
//
// Solidity: function provideMerkleRootOfInput(uint256 inputIndexWithinEpoch, bytes input) view returns(bytes32)
func (_IDaveConsensus *IDaveConsensusSession) ProvideMerkleRootOfInput(inputIndexWithinEpoch *big.Int, input []byte) ([32]byte, error) {
	return _IDaveConsensus.Contract.ProvideMerkleRootOfInput(&_IDaveConsensus.CallOpts, inputIndexWithinEpoch, input)
}

// ProvideMerkleRootOfInput is a free data retrieval call binding the contract method 0x7a96f480.
//
// Solidity: function provideMerkleRootOfInput(uint256 inputIndexWithinEpoch, bytes input) view returns(bytes32)
func (_IDaveConsensus *IDaveConsensusCallerSession) ProvideMerkleRootOfInput(inputIndexWithinEpoch *big.Int, input []byte) ([32]byte, error) {
	return _IDaveConsensus.Contract.ProvideMerkleRootOfInput(&_IDaveConsensus.CallOpts, inputIndexWithinEpoch, input)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IDaveConsensus *IDaveConsensusCaller) SupportsInterface(opts *bind.CallOpts, interfaceId [4]byte) (bool, error) {
	var out []interface{}
	err := _IDaveConsensus.contract.Call(opts, &out, "supportsInterface", interfaceId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IDaveConsensus *IDaveConsensusSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _IDaveConsensus.Contract.SupportsInterface(&_IDaveConsensus.CallOpts, interfaceId)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IDaveConsensus *IDaveConsensusCallerSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _IDaveConsensus.Contract.SupportsInterface(&_IDaveConsensus.CallOpts, interfaceId)
}

// Settle is a paid mutator transaction binding the contract method 0x8bca2e0c.
//
// Solidity: function settle(uint256 epochNumber, bytes32 outputsMerkleRoot, bytes32[] proof) returns()
func (_IDaveConsensus *IDaveConsensusTransactor) Settle(opts *bind.TransactOpts, epochNumber *big.Int, outputsMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _IDaveConsensus.contract.Transact(opts, "settle", epochNumber, outputsMerkleRoot, proof)
}

// Settle is a paid mutator transaction binding the contract method 0x8bca2e0c.
//
// Solidity: function settle(uint256 epochNumber, bytes32 outputsMerkleRoot, bytes32[] proof) returns()
func (_IDaveConsensus *IDaveConsensusSession) Settle(epochNumber *big.Int, outputsMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.Settle(&_IDaveConsensus.TransactOpts, epochNumber, outputsMerkleRoot, proof)
}

// Settle is a paid mutator transaction binding the contract method 0x8bca2e0c.
//
// Solidity: function settle(uint256 epochNumber, bytes32 outputsMerkleRoot, bytes32[] proof) returns()
func (_IDaveConsensus *IDaveConsensusTransactorSession) Settle(epochNumber *big.Int, outputsMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _IDaveConsensus.Contract.Settle(&_IDaveConsensus.TransactOpts, epochNumber, outputsMerkleRoot, proof)
}

// IDaveConsensusConsensusCreationIterator is returned from FilterConsensusCreation and is used to iterate over the raw logs and unpacked data for ConsensusCreation events raised by the IDaveConsensus contract.
type IDaveConsensusConsensusCreationIterator struct {
	Event *IDaveConsensusConsensusCreation // Event containing the contract specifics and raw log

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
func (it *IDaveConsensusConsensusCreationIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IDaveConsensusConsensusCreation)
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
		it.Event = new(IDaveConsensusConsensusCreation)
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
func (it *IDaveConsensusConsensusCreationIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IDaveConsensusConsensusCreationIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IDaveConsensusConsensusCreation represents a ConsensusCreation event raised by the IDaveConsensus contract.
type IDaveConsensusConsensusCreation struct {
	InputBox          common.Address
	AppContract       common.Address
	TournamentFactory common.Address
	Raw               types.Log // Blockchain specific contextual infos
}

// FilterConsensusCreation is a free log retrieval operation binding the contract event 0xaf68463e16cb5595a44214bea8d366ecf7cd3410269c50f92c104b50a7829daa.
//
// Solidity: event ConsensusCreation(address inputBox, address appContract, address tournamentFactory)
func (_IDaveConsensus *IDaveConsensusFilterer) FilterConsensusCreation(opts *bind.FilterOpts) (*IDaveConsensusConsensusCreationIterator, error) {

	logs, sub, err := _IDaveConsensus.contract.FilterLogs(opts, "ConsensusCreation")
	if err != nil {
		return nil, err
	}
	return &IDaveConsensusConsensusCreationIterator{contract: _IDaveConsensus.contract, event: "ConsensusCreation", logs: logs, sub: sub}, nil
}

// WatchConsensusCreation is a free log subscription operation binding the contract event 0xaf68463e16cb5595a44214bea8d366ecf7cd3410269c50f92c104b50a7829daa.
//
// Solidity: event ConsensusCreation(address inputBox, address appContract, address tournamentFactory)
func (_IDaveConsensus *IDaveConsensusFilterer) WatchConsensusCreation(opts *bind.WatchOpts, sink chan<- *IDaveConsensusConsensusCreation) (event.Subscription, error) {

	logs, sub, err := _IDaveConsensus.contract.WatchLogs(opts, "ConsensusCreation")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IDaveConsensusConsensusCreation)
				if err := _IDaveConsensus.contract.UnpackLog(event, "ConsensusCreation", log); err != nil {
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

// ParseConsensusCreation is a log parse operation binding the contract event 0xaf68463e16cb5595a44214bea8d366ecf7cd3410269c50f92c104b50a7829daa.
//
// Solidity: event ConsensusCreation(address inputBox, address appContract, address tournamentFactory)
func (_IDaveConsensus *IDaveConsensusFilterer) ParseConsensusCreation(log types.Log) (*IDaveConsensusConsensusCreation, error) {
	event := new(IDaveConsensusConsensusCreation)
	if err := _IDaveConsensus.contract.UnpackLog(event, "ConsensusCreation", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IDaveConsensusEpochSealedIterator is returned from FilterEpochSealed and is used to iterate over the raw logs and unpacked data for EpochSealed events raised by the IDaveConsensus contract.
type IDaveConsensusEpochSealedIterator struct {
	Event *IDaveConsensusEpochSealed // Event containing the contract specifics and raw log

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
func (it *IDaveConsensusEpochSealedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IDaveConsensusEpochSealed)
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
		it.Event = new(IDaveConsensusEpochSealed)
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
func (it *IDaveConsensusEpochSealedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IDaveConsensusEpochSealedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IDaveConsensusEpochSealed represents a EpochSealed event raised by the IDaveConsensus contract.
type IDaveConsensusEpochSealed struct {
	EpochNumber             *big.Int
	InputIndexLowerBound    *big.Int
	InputIndexUpperBound    *big.Int
	InitialMachineStateHash [32]byte
	OutputsMerkleRoot       [32]byte
	Tournament              common.Address
	Raw                     types.Log // Blockchain specific contextual infos
}

// FilterEpochSealed is a free log retrieval operation binding the contract event 0xa91d0b68c00a132585cc08007b46ff5f0abc622f5286b5701149b33784764ced.
//
// Solidity: event EpochSealed(uint256 epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, bytes32 initialMachineStateHash, bytes32 outputsMerkleRoot, address tournament)
func (_IDaveConsensus *IDaveConsensusFilterer) FilterEpochSealed(opts *bind.FilterOpts) (*IDaveConsensusEpochSealedIterator, error) {

	logs, sub, err := _IDaveConsensus.contract.FilterLogs(opts, "EpochSealed")
	if err != nil {
		return nil, err
	}
	return &IDaveConsensusEpochSealedIterator{contract: _IDaveConsensus.contract, event: "EpochSealed", logs: logs, sub: sub}, nil
}

// WatchEpochSealed is a free log subscription operation binding the contract event 0xa91d0b68c00a132585cc08007b46ff5f0abc622f5286b5701149b33784764ced.
//
// Solidity: event EpochSealed(uint256 epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, bytes32 initialMachineStateHash, bytes32 outputsMerkleRoot, address tournament)
func (_IDaveConsensus *IDaveConsensusFilterer) WatchEpochSealed(opts *bind.WatchOpts, sink chan<- *IDaveConsensusEpochSealed) (event.Subscription, error) {

	logs, sub, err := _IDaveConsensus.contract.WatchLogs(opts, "EpochSealed")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IDaveConsensusEpochSealed)
				if err := _IDaveConsensus.contract.UnpackLog(event, "EpochSealed", log); err != nil {
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

// ParseEpochSealed is a log parse operation binding the contract event 0xa91d0b68c00a132585cc08007b46ff5f0abc622f5286b5701149b33784764ced.
//
// Solidity: event EpochSealed(uint256 epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, bytes32 initialMachineStateHash, bytes32 outputsMerkleRoot, address tournament)
func (_IDaveConsensus *IDaveConsensusFilterer) ParseEpochSealed(log types.Log) (*IDaveConsensusEpochSealed, error) {
	event := new(IDaveConsensusEpochSealed)
	if err := _IDaveConsensus.contract.UnpackLog(event, "EpochSealed", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
