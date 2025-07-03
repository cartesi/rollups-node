// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package daveconsensus

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

// DaveConsensusMetaData contains all meta data concerning the DaveConsensus contract.
var DaveConsensusMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"inputBox\",\"type\":\"address\",\"internalType\":\"contractIInputBox\"},{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"tournamentFactory\",\"type\":\"address\",\"internalType\":\"contractITournamentFactory\"},{\"name\":\"initialMachineStateHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"canSettle\",\"inputs\":[],\"outputs\":[{\"name\":\"isFinished\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"winnerCommitment\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getApplicationContract\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getCurrentSealedEpoch\",\"inputs\":[],\"outputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"inputIndexLowerBound\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"inputIndexUpperBound\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"tournament\",\"type\":\"address\",\"internalType\":\"contractITournament\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getInputBox\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractIInputBox\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getTournamentFactory\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractITournamentFactory\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isOutputsMerkleRootValid\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"provideMerkleRootOfInput\",\"inputs\":[{\"name\":\"inputIndexWithinEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"input\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"settle\",\"inputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"proof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"supportsInterface\",\"inputs\":[{\"name\":\"interfaceId\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"ConsensusCreation\",\"inputs\":[{\"name\":\"inputBox\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractIInputBox\"},{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"tournamentFactory\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractITournamentFactory\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"EpochSealed\",\"inputs\":[{\"name\":\"epochNumber\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"inputIndexLowerBound\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"inputIndexUpperBound\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"initialMachineStateHash\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Machine.Hash\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"},{\"name\":\"tournament\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractITournament\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"ApplicationMismatch\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"received\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"IncorrectEpochNumber\",\"inputs\":[{\"name\":\"received\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InputHashMismatch\",\"inputs\":[{\"name\":\"fromReceivedInput\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"fromInputBox\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"InvalidOutputsMerkleRootProof\",\"inputs\":[{\"name\":\"settledState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}]},{\"type\":\"error\",\"name\":\"InvalidOutputsMerkleRootProofSize\",\"inputs\":[{\"name\":\"suppliedProofSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"TournamentNotFinishedYet\",\"inputs\":[]}]",
}

// DaveConsensusABI is the input ABI used to generate the binding from.
// Deprecated: Use DaveConsensusMetaData.ABI instead.
var DaveConsensusABI = DaveConsensusMetaData.ABI

// DaveConsensus is an auto generated Go binding around an Ethereum contract.
type DaveConsensus struct {
	DaveConsensusCaller     // Read-only binding to the contract
	DaveConsensusTransactor // Write-only binding to the contract
	DaveConsensusFilterer   // Log filterer for contract events
}

// DaveConsensusCaller is an auto generated read-only Go binding around an Ethereum contract.
type DaveConsensusCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DaveConsensusTransactor is an auto generated write-only Go binding around an Ethereum contract.
type DaveConsensusTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DaveConsensusFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type DaveConsensusFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DaveConsensusSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type DaveConsensusSession struct {
	Contract     *DaveConsensus    // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// DaveConsensusCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type DaveConsensusCallerSession struct {
	Contract *DaveConsensusCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts        // Call options to use throughout this session
}

// DaveConsensusTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type DaveConsensusTransactorSession struct {
	Contract     *DaveConsensusTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts        // Transaction auth options to use throughout this session
}

// DaveConsensusRaw is an auto generated low-level Go binding around an Ethereum contract.
type DaveConsensusRaw struct {
	Contract *DaveConsensus // Generic contract binding to access the raw methods on
}

// DaveConsensusCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type DaveConsensusCallerRaw struct {
	Contract *DaveConsensusCaller // Generic read-only contract binding to access the raw methods on
}

// DaveConsensusTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type DaveConsensusTransactorRaw struct {
	Contract *DaveConsensusTransactor // Generic write-only contract binding to access the raw methods on
}

// NewDaveConsensus creates a new instance of DaveConsensus, bound to a specific deployed contract.
func NewDaveConsensus(address common.Address, backend bind.ContractBackend) (*DaveConsensus, error) {
	contract, err := bindDaveConsensus(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &DaveConsensus{DaveConsensusCaller: DaveConsensusCaller{contract: contract}, DaveConsensusTransactor: DaveConsensusTransactor{contract: contract}, DaveConsensusFilterer: DaveConsensusFilterer{contract: contract}}, nil
}

// NewDaveConsensusCaller creates a new read-only instance of DaveConsensus, bound to a specific deployed contract.
func NewDaveConsensusCaller(address common.Address, caller bind.ContractCaller) (*DaveConsensusCaller, error) {
	contract, err := bindDaveConsensus(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &DaveConsensusCaller{contract: contract}, nil
}

// NewDaveConsensusTransactor creates a new write-only instance of DaveConsensus, bound to a specific deployed contract.
func NewDaveConsensusTransactor(address common.Address, transactor bind.ContractTransactor) (*DaveConsensusTransactor, error) {
	contract, err := bindDaveConsensus(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &DaveConsensusTransactor{contract: contract}, nil
}

// NewDaveConsensusFilterer creates a new log filterer instance of DaveConsensus, bound to a specific deployed contract.
func NewDaveConsensusFilterer(address common.Address, filterer bind.ContractFilterer) (*DaveConsensusFilterer, error) {
	contract, err := bindDaveConsensus(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &DaveConsensusFilterer{contract: contract}, nil
}

// bindDaveConsensus binds a generic wrapper to an already deployed contract.
func bindDaveConsensus(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := DaveConsensusMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_DaveConsensus *DaveConsensusRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _DaveConsensus.Contract.DaveConsensusCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_DaveConsensus *DaveConsensusRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DaveConsensus.Contract.DaveConsensusTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_DaveConsensus *DaveConsensusRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _DaveConsensus.Contract.DaveConsensusTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_DaveConsensus *DaveConsensusCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _DaveConsensus.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_DaveConsensus *DaveConsensusTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DaveConsensus.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_DaveConsensus *DaveConsensusTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _DaveConsensus.Contract.contract.Transact(opts, method, params...)
}

// CanSettle is a free data retrieval call binding the contract method 0xfaf7ba6a.
//
// Solidity: function canSettle() view returns(bool isFinished, uint256 epochNumber, bytes32 winnerCommitment)
func (_DaveConsensus *DaveConsensusCaller) CanSettle(opts *bind.CallOpts) (struct {
	IsFinished       bool
	EpochNumber      *big.Int
	WinnerCommitment [32]byte
}, error) {
	var out []interface{}
	err := _DaveConsensus.contract.Call(opts, &out, "canSettle")

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
func (_DaveConsensus *DaveConsensusSession) CanSettle() (struct {
	IsFinished       bool
	EpochNumber      *big.Int
	WinnerCommitment [32]byte
}, error) {
	return _DaveConsensus.Contract.CanSettle(&_DaveConsensus.CallOpts)
}

// CanSettle is a free data retrieval call binding the contract method 0xfaf7ba6a.
//
// Solidity: function canSettle() view returns(bool isFinished, uint256 epochNumber, bytes32 winnerCommitment)
func (_DaveConsensus *DaveConsensusCallerSession) CanSettle() (struct {
	IsFinished       bool
	EpochNumber      *big.Int
	WinnerCommitment [32]byte
}, error) {
	return _DaveConsensus.Contract.CanSettle(&_DaveConsensus.CallOpts)
}

// GetApplicationContract is a free data retrieval call binding the contract method 0xc050be00.
//
// Solidity: function getApplicationContract() view returns(address)
func (_DaveConsensus *DaveConsensusCaller) GetApplicationContract(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _DaveConsensus.contract.Call(opts, &out, "getApplicationContract")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetApplicationContract is a free data retrieval call binding the contract method 0xc050be00.
//
// Solidity: function getApplicationContract() view returns(address)
func (_DaveConsensus *DaveConsensusSession) GetApplicationContract() (common.Address, error) {
	return _DaveConsensus.Contract.GetApplicationContract(&_DaveConsensus.CallOpts)
}

// GetApplicationContract is a free data retrieval call binding the contract method 0xc050be00.
//
// Solidity: function getApplicationContract() view returns(address)
func (_DaveConsensus *DaveConsensusCallerSession) GetApplicationContract() (common.Address, error) {
	return _DaveConsensus.Contract.GetApplicationContract(&_DaveConsensus.CallOpts)
}

// GetCurrentSealedEpoch is a free data retrieval call binding the contract method 0x1239acd9.
//
// Solidity: function getCurrentSealedEpoch() view returns(uint256 epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, address tournament)
func (_DaveConsensus *DaveConsensusCaller) GetCurrentSealedEpoch(opts *bind.CallOpts) (struct {
	EpochNumber          *big.Int
	InputIndexLowerBound *big.Int
	InputIndexUpperBound *big.Int
	Tournament           common.Address
}, error) {
	var out []interface{}
	err := _DaveConsensus.contract.Call(opts, &out, "getCurrentSealedEpoch")

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
func (_DaveConsensus *DaveConsensusSession) GetCurrentSealedEpoch() (struct {
	EpochNumber          *big.Int
	InputIndexLowerBound *big.Int
	InputIndexUpperBound *big.Int
	Tournament           common.Address
}, error) {
	return _DaveConsensus.Contract.GetCurrentSealedEpoch(&_DaveConsensus.CallOpts)
}

// GetCurrentSealedEpoch is a free data retrieval call binding the contract method 0x1239acd9.
//
// Solidity: function getCurrentSealedEpoch() view returns(uint256 epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, address tournament)
func (_DaveConsensus *DaveConsensusCallerSession) GetCurrentSealedEpoch() (struct {
	EpochNumber          *big.Int
	InputIndexLowerBound *big.Int
	InputIndexUpperBound *big.Int
	Tournament           common.Address
}, error) {
	return _DaveConsensus.Contract.GetCurrentSealedEpoch(&_DaveConsensus.CallOpts)
}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_DaveConsensus *DaveConsensusCaller) GetInputBox(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _DaveConsensus.contract.Call(opts, &out, "getInputBox")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_DaveConsensus *DaveConsensusSession) GetInputBox() (common.Address, error) {
	return _DaveConsensus.Contract.GetInputBox(&_DaveConsensus.CallOpts)
}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_DaveConsensus *DaveConsensusCallerSession) GetInputBox() (common.Address, error) {
	return _DaveConsensus.Contract.GetInputBox(&_DaveConsensus.CallOpts)
}

// GetTournamentFactory is a free data retrieval call binding the contract method 0x813a1aaf.
//
// Solidity: function getTournamentFactory() view returns(address)
func (_DaveConsensus *DaveConsensusCaller) GetTournamentFactory(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _DaveConsensus.contract.Call(opts, &out, "getTournamentFactory")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetTournamentFactory is a free data retrieval call binding the contract method 0x813a1aaf.
//
// Solidity: function getTournamentFactory() view returns(address)
func (_DaveConsensus *DaveConsensusSession) GetTournamentFactory() (common.Address, error) {
	return _DaveConsensus.Contract.GetTournamentFactory(&_DaveConsensus.CallOpts)
}

// GetTournamentFactory is a free data retrieval call binding the contract method 0x813a1aaf.
//
// Solidity: function getTournamentFactory() view returns(address)
func (_DaveConsensus *DaveConsensusCallerSession) GetTournamentFactory() (common.Address, error) {
	return _DaveConsensus.Contract.GetTournamentFactory(&_DaveConsensus.CallOpts)
}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_DaveConsensus *DaveConsensusCaller) IsOutputsMerkleRootValid(opts *bind.CallOpts, appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	var out []interface{}
	err := _DaveConsensus.contract.Call(opts, &out, "isOutputsMerkleRootValid", appContract, outputsMerkleRoot)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_DaveConsensus *DaveConsensusSession) IsOutputsMerkleRootValid(appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	return _DaveConsensus.Contract.IsOutputsMerkleRootValid(&_DaveConsensus.CallOpts, appContract, outputsMerkleRoot)
}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_DaveConsensus *DaveConsensusCallerSession) IsOutputsMerkleRootValid(appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	return _DaveConsensus.Contract.IsOutputsMerkleRootValid(&_DaveConsensus.CallOpts, appContract, outputsMerkleRoot)
}

// ProvideMerkleRootOfInput is a free data retrieval call binding the contract method 0x7a96f480.
//
// Solidity: function provideMerkleRootOfInput(uint256 inputIndexWithinEpoch, bytes input) view returns(bytes32)
func (_DaveConsensus *DaveConsensusCaller) ProvideMerkleRootOfInput(opts *bind.CallOpts, inputIndexWithinEpoch *big.Int, input []byte) ([32]byte, error) {
	var out []interface{}
	err := _DaveConsensus.contract.Call(opts, &out, "provideMerkleRootOfInput", inputIndexWithinEpoch, input)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// ProvideMerkleRootOfInput is a free data retrieval call binding the contract method 0x7a96f480.
//
// Solidity: function provideMerkleRootOfInput(uint256 inputIndexWithinEpoch, bytes input) view returns(bytes32)
func (_DaveConsensus *DaveConsensusSession) ProvideMerkleRootOfInput(inputIndexWithinEpoch *big.Int, input []byte) ([32]byte, error) {
	return _DaveConsensus.Contract.ProvideMerkleRootOfInput(&_DaveConsensus.CallOpts, inputIndexWithinEpoch, input)
}

// ProvideMerkleRootOfInput is a free data retrieval call binding the contract method 0x7a96f480.
//
// Solidity: function provideMerkleRootOfInput(uint256 inputIndexWithinEpoch, bytes input) view returns(bytes32)
func (_DaveConsensus *DaveConsensusCallerSession) ProvideMerkleRootOfInput(inputIndexWithinEpoch *big.Int, input []byte) ([32]byte, error) {
	return _DaveConsensus.Contract.ProvideMerkleRootOfInput(&_DaveConsensus.CallOpts, inputIndexWithinEpoch, input)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_DaveConsensus *DaveConsensusCaller) SupportsInterface(opts *bind.CallOpts, interfaceId [4]byte) (bool, error) {
	var out []interface{}
	err := _DaveConsensus.contract.Call(opts, &out, "supportsInterface", interfaceId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_DaveConsensus *DaveConsensusSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _DaveConsensus.Contract.SupportsInterface(&_DaveConsensus.CallOpts, interfaceId)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_DaveConsensus *DaveConsensusCallerSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _DaveConsensus.Contract.SupportsInterface(&_DaveConsensus.CallOpts, interfaceId)
}

// Settle is a paid mutator transaction binding the contract method 0x8bca2e0c.
//
// Solidity: function settle(uint256 epochNumber, bytes32 outputsMerkleRoot, bytes32[] proof) returns()
func (_DaveConsensus *DaveConsensusTransactor) Settle(opts *bind.TransactOpts, epochNumber *big.Int, outputsMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _DaveConsensus.contract.Transact(opts, "settle", epochNumber, outputsMerkleRoot, proof)
}

// Settle is a paid mutator transaction binding the contract method 0x8bca2e0c.
//
// Solidity: function settle(uint256 epochNumber, bytes32 outputsMerkleRoot, bytes32[] proof) returns()
func (_DaveConsensus *DaveConsensusSession) Settle(epochNumber *big.Int, outputsMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _DaveConsensus.Contract.Settle(&_DaveConsensus.TransactOpts, epochNumber, outputsMerkleRoot, proof)
}

// Settle is a paid mutator transaction binding the contract method 0x8bca2e0c.
//
// Solidity: function settle(uint256 epochNumber, bytes32 outputsMerkleRoot, bytes32[] proof) returns()
func (_DaveConsensus *DaveConsensusTransactorSession) Settle(epochNumber *big.Int, outputsMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _DaveConsensus.Contract.Settle(&_DaveConsensus.TransactOpts, epochNumber, outputsMerkleRoot, proof)
}

// DaveConsensusConsensusCreationIterator is returned from FilterConsensusCreation and is used to iterate over the raw logs and unpacked data for ConsensusCreation events raised by the DaveConsensus contract.
type DaveConsensusConsensusCreationIterator struct {
	Event *DaveConsensusConsensusCreation // Event containing the contract specifics and raw log

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
func (it *DaveConsensusConsensusCreationIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(DaveConsensusConsensusCreation)
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
		it.Event = new(DaveConsensusConsensusCreation)
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
func (it *DaveConsensusConsensusCreationIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *DaveConsensusConsensusCreationIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// DaveConsensusConsensusCreation represents a ConsensusCreation event raised by the DaveConsensus contract.
type DaveConsensusConsensusCreation struct {
	InputBox          common.Address
	AppContract       common.Address
	TournamentFactory common.Address
	Raw               types.Log // Blockchain specific contextual infos
}

// FilterConsensusCreation is a free log retrieval operation binding the contract event 0xaf68463e16cb5595a44214bea8d366ecf7cd3410269c50f92c104b50a7829daa.
//
// Solidity: event ConsensusCreation(address inputBox, address appContract, address tournamentFactory)
func (_DaveConsensus *DaveConsensusFilterer) FilterConsensusCreation(opts *bind.FilterOpts) (*DaveConsensusConsensusCreationIterator, error) {

	logs, sub, err := _DaveConsensus.contract.FilterLogs(opts, "ConsensusCreation")
	if err != nil {
		return nil, err
	}
	return &DaveConsensusConsensusCreationIterator{contract: _DaveConsensus.contract, event: "ConsensusCreation", logs: logs, sub: sub}, nil
}

// WatchConsensusCreation is a free log subscription operation binding the contract event 0xaf68463e16cb5595a44214bea8d366ecf7cd3410269c50f92c104b50a7829daa.
//
// Solidity: event ConsensusCreation(address inputBox, address appContract, address tournamentFactory)
func (_DaveConsensus *DaveConsensusFilterer) WatchConsensusCreation(opts *bind.WatchOpts, sink chan<- *DaveConsensusConsensusCreation) (event.Subscription, error) {

	logs, sub, err := _DaveConsensus.contract.WatchLogs(opts, "ConsensusCreation")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(DaveConsensusConsensusCreation)
				if err := _DaveConsensus.contract.UnpackLog(event, "ConsensusCreation", log); err != nil {
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
func (_DaveConsensus *DaveConsensusFilterer) ParseConsensusCreation(log types.Log) (*DaveConsensusConsensusCreation, error) {
	event := new(DaveConsensusConsensusCreation)
	if err := _DaveConsensus.contract.UnpackLog(event, "ConsensusCreation", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// DaveConsensusEpochSealedIterator is returned from FilterEpochSealed and is used to iterate over the raw logs and unpacked data for EpochSealed events raised by the DaveConsensus contract.
type DaveConsensusEpochSealedIterator struct {
	Event *DaveConsensusEpochSealed // Event containing the contract specifics and raw log

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
func (it *DaveConsensusEpochSealedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(DaveConsensusEpochSealed)
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
		it.Event = new(DaveConsensusEpochSealed)
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
func (it *DaveConsensusEpochSealedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *DaveConsensusEpochSealedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// DaveConsensusEpochSealed represents a EpochSealed event raised by the DaveConsensus contract.
type DaveConsensusEpochSealed struct {
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
func (_DaveConsensus *DaveConsensusFilterer) FilterEpochSealed(opts *bind.FilterOpts) (*DaveConsensusEpochSealedIterator, error) {

	logs, sub, err := _DaveConsensus.contract.FilterLogs(opts, "EpochSealed")
	if err != nil {
		return nil, err
	}
	return &DaveConsensusEpochSealedIterator{contract: _DaveConsensus.contract, event: "EpochSealed", logs: logs, sub: sub}, nil
}

// WatchEpochSealed is a free log subscription operation binding the contract event 0xa91d0b68c00a132585cc08007b46ff5f0abc622f5286b5701149b33784764ced.
//
// Solidity: event EpochSealed(uint256 epochNumber, uint256 inputIndexLowerBound, uint256 inputIndexUpperBound, bytes32 initialMachineStateHash, bytes32 outputsMerkleRoot, address tournament)
func (_DaveConsensus *DaveConsensusFilterer) WatchEpochSealed(opts *bind.WatchOpts, sink chan<- *DaveConsensusEpochSealed) (event.Subscription, error) {

	logs, sub, err := _DaveConsensus.contract.WatchLogs(opts, "EpochSealed")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(DaveConsensusEpochSealed)
				if err := _DaveConsensus.contract.UnpackLog(event, "EpochSealed", log); err != nil {
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
func (_DaveConsensus *DaveConsensusFilterer) ParseEpochSealed(log types.Log) (*DaveConsensusEpochSealed, error) {
	event := new(DaveConsensusEpochSealed)
	if err := _DaveConsensus.contract.UnpackLog(event, "EpochSealed", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
