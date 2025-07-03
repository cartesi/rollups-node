// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package roottournament

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

// ClockState is an auto generated low-level Go binding around an user-defined struct.
type ClockState struct {
	Allowance    uint64
	StartInstant uint64
}

// MatchId is an auto generated low-level Go binding around an user-defined struct.
type MatchId struct {
	CommitmentOne [32]byte
	CommitmentTwo [32]byte
}

// MatchState is an auto generated low-level Go binding around an user-defined struct.
type MatchState struct {
	OtherParent         [32]byte
	LeftNode            [32]byte
	RightNode           [32]byte
	RunningLeafPosition *big.Int
	CurrentHeight       uint64
	Log2step            uint64
	Height              uint64
}

// RootTournamentMetaData contains all meta data concerning the RootTournament contract.
var RootTournamentMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"advanceMatch\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_newLeftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_newRightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"arbitrationResult\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"canWinMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"eliminateMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getCommitment\",\"inputs\":[{\"name\":\"_commitmentRoot\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structClock.State\",\"components\":[{\"name\":\"allowance\",\"type\":\"uint64\",\"internalType\":\"Time.Duration\"},{\"name\":\"startInstant\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"}]},{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMatch\",\"inputs\":[{\"name\":\"_matchIdHash\",\"type\":\"bytes32\",\"internalType\":\"Match.IdHash\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structMatch.State\",\"components\":[{\"name\":\"otherParent\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"runningLeafPosition\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentHeight\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"log2step\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"height\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMatchCycle\",\"inputs\":[{\"name\":\"_matchIdHash\",\"type\":\"bytes32\",\"internalType\":\"Match.IdHash\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isClosed\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isFinished\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"joinTournament\",\"inputs\":[{\"name\":\"_finalState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"_proof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"timeFinished\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"tournamentLevelConstants\",\"inputs\":[],\"outputs\":[{\"name\":\"_maxLevel\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_level\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_log2step\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_height\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"winMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"commitmentJoined\",\"inputs\":[{\"name\":\"root\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchAdvanced\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Match.IdHash\"},{\"name\":\"parent\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"},{\"name\":\"left\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchCreated\",\"inputs\":[{\"name\":\"one\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"two\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"leftOfTwo\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchDeleted\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Match.IdHash\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"EliminateByTimeout\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidContestedFinalState\",\"inputs\":[{\"name\":\"contestedFinalStateOne\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"contestedFinalStateTwo\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"finalState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}]},{\"type\":\"error\",\"name\":\"TournamentIsClosed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TournamentIsFinished\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WinByTimeout\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WrongChildren\",\"inputs\":[{\"name\":\"commitment\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"parent\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"left\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"right\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}]",
}

// RootTournamentABI is the input ABI used to generate the binding from.
// Deprecated: Use RootTournamentMetaData.ABI instead.
var RootTournamentABI = RootTournamentMetaData.ABI

// RootTournament is an auto generated Go binding around an Ethereum contract.
type RootTournament struct {
	RootTournamentCaller     // Read-only binding to the contract
	RootTournamentTransactor // Write-only binding to the contract
	RootTournamentFilterer   // Log filterer for contract events
}

// RootTournamentCaller is an auto generated read-only Go binding around an Ethereum contract.
type RootTournamentCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RootTournamentTransactor is an auto generated write-only Go binding around an Ethereum contract.
type RootTournamentTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RootTournamentFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type RootTournamentFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// RootTournamentSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type RootTournamentSession struct {
	Contract     *RootTournament   // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// RootTournamentCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type RootTournamentCallerSession struct {
	Contract *RootTournamentCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts         // Call options to use throughout this session
}

// RootTournamentTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type RootTournamentTransactorSession struct {
	Contract     *RootTournamentTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts         // Transaction auth options to use throughout this session
}

// RootTournamentRaw is an auto generated low-level Go binding around an Ethereum contract.
type RootTournamentRaw struct {
	Contract *RootTournament // Generic contract binding to access the raw methods on
}

// RootTournamentCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type RootTournamentCallerRaw struct {
	Contract *RootTournamentCaller // Generic read-only contract binding to access the raw methods on
}

// RootTournamentTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type RootTournamentTransactorRaw struct {
	Contract *RootTournamentTransactor // Generic write-only contract binding to access the raw methods on
}

// NewRootTournament creates a new instance of RootTournament, bound to a specific deployed contract.
func NewRootTournament(address common.Address, backend bind.ContractBackend) (*RootTournament, error) {
	contract, err := bindRootTournament(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &RootTournament{RootTournamentCaller: RootTournamentCaller{contract: contract}, RootTournamentTransactor: RootTournamentTransactor{contract: contract}, RootTournamentFilterer: RootTournamentFilterer{contract: contract}}, nil
}

// NewRootTournamentCaller creates a new read-only instance of RootTournament, bound to a specific deployed contract.
func NewRootTournamentCaller(address common.Address, caller bind.ContractCaller) (*RootTournamentCaller, error) {
	contract, err := bindRootTournament(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &RootTournamentCaller{contract: contract}, nil
}

// NewRootTournamentTransactor creates a new write-only instance of RootTournament, bound to a specific deployed contract.
func NewRootTournamentTransactor(address common.Address, transactor bind.ContractTransactor) (*RootTournamentTransactor, error) {
	contract, err := bindRootTournament(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &RootTournamentTransactor{contract: contract}, nil
}

// NewRootTournamentFilterer creates a new log filterer instance of RootTournament, bound to a specific deployed contract.
func NewRootTournamentFilterer(address common.Address, filterer bind.ContractFilterer) (*RootTournamentFilterer, error) {
	contract, err := bindRootTournament(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &RootTournamentFilterer{contract: contract}, nil
}

// bindRootTournament binds a generic wrapper to an already deployed contract.
func bindRootTournament(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := RootTournamentMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_RootTournament *RootTournamentRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _RootTournament.Contract.RootTournamentCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_RootTournament *RootTournamentRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _RootTournament.Contract.RootTournamentTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_RootTournament *RootTournamentRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _RootTournament.Contract.RootTournamentTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_RootTournament *RootTournamentCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _RootTournament.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_RootTournament *RootTournamentTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _RootTournament.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_RootTournament *RootTournamentTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _RootTournament.Contract.contract.Transact(opts, method, params...)
}

// ArbitrationResult is a free data retrieval call binding the contract method 0xcb2773db.
//
// Solidity: function arbitrationResult() view returns(bool, bytes32, bytes32)
func (_RootTournament *RootTournamentCaller) ArbitrationResult(opts *bind.CallOpts) (bool, [32]byte, [32]byte, error) {
	var out []interface{}
	err := _RootTournament.contract.Call(opts, &out, "arbitrationResult")

	if err != nil {
		return *new(bool), *new([32]byte), *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	out1 := *abi.ConvertType(out[1], new([32]byte)).(*[32]byte)
	out2 := *abi.ConvertType(out[2], new([32]byte)).(*[32]byte)

	return out0, out1, out2, err

}

// ArbitrationResult is a free data retrieval call binding the contract method 0xcb2773db.
//
// Solidity: function arbitrationResult() view returns(bool, bytes32, bytes32)
func (_RootTournament *RootTournamentSession) ArbitrationResult() (bool, [32]byte, [32]byte, error) {
	return _RootTournament.Contract.ArbitrationResult(&_RootTournament.CallOpts)
}

// ArbitrationResult is a free data retrieval call binding the contract method 0xcb2773db.
//
// Solidity: function arbitrationResult() view returns(bool, bytes32, bytes32)
func (_RootTournament *RootTournamentCallerSession) ArbitrationResult() (bool, [32]byte, [32]byte, error) {
	return _RootTournament.Contract.ArbitrationResult(&_RootTournament.CallOpts)
}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_RootTournament *RootTournamentCaller) CanWinMatchByTimeout(opts *bind.CallOpts, _matchId MatchId) (bool, error) {
	var out []interface{}
	err := _RootTournament.contract.Call(opts, &out, "canWinMatchByTimeout", _matchId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_RootTournament *RootTournamentSession) CanWinMatchByTimeout(_matchId MatchId) (bool, error) {
	return _RootTournament.Contract.CanWinMatchByTimeout(&_RootTournament.CallOpts, _matchId)
}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_RootTournament *RootTournamentCallerSession) CanWinMatchByTimeout(_matchId MatchId) (bool, error) {
	return _RootTournament.Contract.CanWinMatchByTimeout(&_RootTournament.CallOpts, _matchId)
}

// GetCommitment is a free data retrieval call binding the contract method 0x7795820c.
//
// Solidity: function getCommitment(bytes32 _commitmentRoot) view returns((uint64,uint64), bytes32)
func (_RootTournament *RootTournamentCaller) GetCommitment(opts *bind.CallOpts, _commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	var out []interface{}
	err := _RootTournament.contract.Call(opts, &out, "getCommitment", _commitmentRoot)

	if err != nil {
		return *new(ClockState), *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new(ClockState)).(*ClockState)
	out1 := *abi.ConvertType(out[1], new([32]byte)).(*[32]byte)

	return out0, out1, err

}

// GetCommitment is a free data retrieval call binding the contract method 0x7795820c.
//
// Solidity: function getCommitment(bytes32 _commitmentRoot) view returns((uint64,uint64), bytes32)
func (_RootTournament *RootTournamentSession) GetCommitment(_commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	return _RootTournament.Contract.GetCommitment(&_RootTournament.CallOpts, _commitmentRoot)
}

// GetCommitment is a free data retrieval call binding the contract method 0x7795820c.
//
// Solidity: function getCommitment(bytes32 _commitmentRoot) view returns((uint64,uint64), bytes32)
func (_RootTournament *RootTournamentCallerSession) GetCommitment(_commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	return _RootTournament.Contract.GetCommitment(&_RootTournament.CallOpts, _commitmentRoot)
}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_RootTournament *RootTournamentCaller) GetMatch(opts *bind.CallOpts, _matchIdHash [32]byte) (MatchState, error) {
	var out []interface{}
	err := _RootTournament.contract.Call(opts, &out, "getMatch", _matchIdHash)

	if err != nil {
		return *new(MatchState), err
	}

	out0 := *abi.ConvertType(out[0], new(MatchState)).(*MatchState)

	return out0, err

}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_RootTournament *RootTournamentSession) GetMatch(_matchIdHash [32]byte) (MatchState, error) {
	return _RootTournament.Contract.GetMatch(&_RootTournament.CallOpts, _matchIdHash)
}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_RootTournament *RootTournamentCallerSession) GetMatch(_matchIdHash [32]byte) (MatchState, error) {
	return _RootTournament.Contract.GetMatch(&_RootTournament.CallOpts, _matchIdHash)
}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_RootTournament *RootTournamentCaller) GetMatchCycle(opts *bind.CallOpts, _matchIdHash [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _RootTournament.contract.Call(opts, &out, "getMatchCycle", _matchIdHash)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_RootTournament *RootTournamentSession) GetMatchCycle(_matchIdHash [32]byte) (*big.Int, error) {
	return _RootTournament.Contract.GetMatchCycle(&_RootTournament.CallOpts, _matchIdHash)
}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_RootTournament *RootTournamentCallerSession) GetMatchCycle(_matchIdHash [32]byte) (*big.Int, error) {
	return _RootTournament.Contract.GetMatchCycle(&_RootTournament.CallOpts, _matchIdHash)
}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_RootTournament *RootTournamentCaller) IsClosed(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _RootTournament.contract.Call(opts, &out, "isClosed")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_RootTournament *RootTournamentSession) IsClosed() (bool, error) {
	return _RootTournament.Contract.IsClosed(&_RootTournament.CallOpts)
}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_RootTournament *RootTournamentCallerSession) IsClosed() (bool, error) {
	return _RootTournament.Contract.IsClosed(&_RootTournament.CallOpts)
}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_RootTournament *RootTournamentCaller) IsFinished(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _RootTournament.contract.Call(opts, &out, "isFinished")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_RootTournament *RootTournamentSession) IsFinished() (bool, error) {
	return _RootTournament.Contract.IsFinished(&_RootTournament.CallOpts)
}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_RootTournament *RootTournamentCallerSession) IsFinished() (bool, error) {
	return _RootTournament.Contract.IsFinished(&_RootTournament.CallOpts)
}

// TimeFinished is a free data retrieval call binding the contract method 0x39cdfaf2.
//
// Solidity: function timeFinished() view returns(bool, uint64)
func (_RootTournament *RootTournamentCaller) TimeFinished(opts *bind.CallOpts) (bool, uint64, error) {
	var out []interface{}
	err := _RootTournament.contract.Call(opts, &out, "timeFinished")

	if err != nil {
		return *new(bool), *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	out1 := *abi.ConvertType(out[1], new(uint64)).(*uint64)

	return out0, out1, err

}

// TimeFinished is a free data retrieval call binding the contract method 0x39cdfaf2.
//
// Solidity: function timeFinished() view returns(bool, uint64)
func (_RootTournament *RootTournamentSession) TimeFinished() (bool, uint64, error) {
	return _RootTournament.Contract.TimeFinished(&_RootTournament.CallOpts)
}

// TimeFinished is a free data retrieval call binding the contract method 0x39cdfaf2.
//
// Solidity: function timeFinished() view returns(bool, uint64)
func (_RootTournament *RootTournamentCallerSession) TimeFinished() (bool, uint64, error) {
	return _RootTournament.Contract.TimeFinished(&_RootTournament.CallOpts)
}

// TournamentLevelConstants is a free data retrieval call binding the contract method 0xa1af906b.
//
// Solidity: function tournamentLevelConstants() view returns(uint64 _maxLevel, uint64 _level, uint64 _log2step, uint64 _height)
func (_RootTournament *RootTournamentCaller) TournamentLevelConstants(opts *bind.CallOpts) (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	var out []interface{}
	err := _RootTournament.contract.Call(opts, &out, "tournamentLevelConstants")

	outstruct := new(struct {
		MaxLevel uint64
		Level    uint64
		Log2step uint64
		Height   uint64
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.MaxLevel = *abi.ConvertType(out[0], new(uint64)).(*uint64)
	outstruct.Level = *abi.ConvertType(out[1], new(uint64)).(*uint64)
	outstruct.Log2step = *abi.ConvertType(out[2], new(uint64)).(*uint64)
	outstruct.Height = *abi.ConvertType(out[3], new(uint64)).(*uint64)

	return *outstruct, err

}

// TournamentLevelConstants is a free data retrieval call binding the contract method 0xa1af906b.
//
// Solidity: function tournamentLevelConstants() view returns(uint64 _maxLevel, uint64 _level, uint64 _log2step, uint64 _height)
func (_RootTournament *RootTournamentSession) TournamentLevelConstants() (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	return _RootTournament.Contract.TournamentLevelConstants(&_RootTournament.CallOpts)
}

// TournamentLevelConstants is a free data retrieval call binding the contract method 0xa1af906b.
//
// Solidity: function tournamentLevelConstants() view returns(uint64 _maxLevel, uint64 _level, uint64 _log2step, uint64 _height)
func (_RootTournament *RootTournamentCallerSession) TournamentLevelConstants() (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	return _RootTournament.Contract.TournamentLevelConstants(&_RootTournament.CallOpts)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_RootTournament *RootTournamentTransactor) AdvanceMatch(opts *bind.TransactOpts, _matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _RootTournament.contract.Transact(opts, "advanceMatch", _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_RootTournament *RootTournamentSession) AdvanceMatch(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _RootTournament.Contract.AdvanceMatch(&_RootTournament.TransactOpts, _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_RootTournament *RootTournamentTransactorSession) AdvanceMatch(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _RootTournament.Contract.AdvanceMatch(&_RootTournament.TransactOpts, _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_RootTournament *RootTournamentTransactor) EliminateMatchByTimeout(opts *bind.TransactOpts, _matchId MatchId) (*types.Transaction, error) {
	return _RootTournament.contract.Transact(opts, "eliminateMatchByTimeout", _matchId)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_RootTournament *RootTournamentSession) EliminateMatchByTimeout(_matchId MatchId) (*types.Transaction, error) {
	return _RootTournament.Contract.EliminateMatchByTimeout(&_RootTournament.TransactOpts, _matchId)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_RootTournament *RootTournamentTransactorSession) EliminateMatchByTimeout(_matchId MatchId) (*types.Transaction, error) {
	return _RootTournament.Contract.EliminateMatchByTimeout(&_RootTournament.TransactOpts, _matchId)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_RootTournament *RootTournamentTransactor) JoinTournament(opts *bind.TransactOpts, _finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _RootTournament.contract.Transact(opts, "joinTournament", _finalState, _proof, _leftNode, _rightNode)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_RootTournament *RootTournamentSession) JoinTournament(_finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _RootTournament.Contract.JoinTournament(&_RootTournament.TransactOpts, _finalState, _proof, _leftNode, _rightNode)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_RootTournament *RootTournamentTransactorSession) JoinTournament(_finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _RootTournament.Contract.JoinTournament(&_RootTournament.TransactOpts, _finalState, _proof, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_RootTournament *RootTournamentTransactor) WinMatchByTimeout(opts *bind.TransactOpts, _matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _RootTournament.contract.Transact(opts, "winMatchByTimeout", _matchId, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_RootTournament *RootTournamentSession) WinMatchByTimeout(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _RootTournament.Contract.WinMatchByTimeout(&_RootTournament.TransactOpts, _matchId, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_RootTournament *RootTournamentTransactorSession) WinMatchByTimeout(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _RootTournament.Contract.WinMatchByTimeout(&_RootTournament.TransactOpts, _matchId, _leftNode, _rightNode)
}

// RootTournamentCommitmentJoinedIterator is returned from FilterCommitmentJoined and is used to iterate over the raw logs and unpacked data for CommitmentJoined events raised by the RootTournament contract.
type RootTournamentCommitmentJoinedIterator struct {
	Event *RootTournamentCommitmentJoined // Event containing the contract specifics and raw log

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
func (it *RootTournamentCommitmentJoinedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(RootTournamentCommitmentJoined)
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
		it.Event = new(RootTournamentCommitmentJoined)
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
func (it *RootTournamentCommitmentJoinedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *RootTournamentCommitmentJoinedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// RootTournamentCommitmentJoined represents a CommitmentJoined event raised by the RootTournament contract.
type RootTournamentCommitmentJoined struct {
	Root [32]byte
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterCommitmentJoined is a free log retrieval operation binding the contract event 0xe53537f202911d376d6e285835b2a2016e83e99fbe84a059d445cc2be4807262.
//
// Solidity: event commitmentJoined(bytes32 root)
func (_RootTournament *RootTournamentFilterer) FilterCommitmentJoined(opts *bind.FilterOpts) (*RootTournamentCommitmentJoinedIterator, error) {

	logs, sub, err := _RootTournament.contract.FilterLogs(opts, "commitmentJoined")
	if err != nil {
		return nil, err
	}
	return &RootTournamentCommitmentJoinedIterator{contract: _RootTournament.contract, event: "commitmentJoined", logs: logs, sub: sub}, nil
}

// WatchCommitmentJoined is a free log subscription operation binding the contract event 0xe53537f202911d376d6e285835b2a2016e83e99fbe84a059d445cc2be4807262.
//
// Solidity: event commitmentJoined(bytes32 root)
func (_RootTournament *RootTournamentFilterer) WatchCommitmentJoined(opts *bind.WatchOpts, sink chan<- *RootTournamentCommitmentJoined) (event.Subscription, error) {

	logs, sub, err := _RootTournament.contract.WatchLogs(opts, "commitmentJoined")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(RootTournamentCommitmentJoined)
				if err := _RootTournament.contract.UnpackLog(event, "commitmentJoined", log); err != nil {
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

// ParseCommitmentJoined is a log parse operation binding the contract event 0xe53537f202911d376d6e285835b2a2016e83e99fbe84a059d445cc2be4807262.
//
// Solidity: event commitmentJoined(bytes32 root)
func (_RootTournament *RootTournamentFilterer) ParseCommitmentJoined(log types.Log) (*RootTournamentCommitmentJoined, error) {
	event := new(RootTournamentCommitmentJoined)
	if err := _RootTournament.contract.UnpackLog(event, "commitmentJoined", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// RootTournamentMatchAdvancedIterator is returned from FilterMatchAdvanced and is used to iterate over the raw logs and unpacked data for MatchAdvanced events raised by the RootTournament contract.
type RootTournamentMatchAdvancedIterator struct {
	Event *RootTournamentMatchAdvanced // Event containing the contract specifics and raw log

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
func (it *RootTournamentMatchAdvancedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(RootTournamentMatchAdvanced)
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
		it.Event = new(RootTournamentMatchAdvanced)
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
func (it *RootTournamentMatchAdvancedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *RootTournamentMatchAdvancedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// RootTournamentMatchAdvanced represents a MatchAdvanced event raised by the RootTournament contract.
type RootTournamentMatchAdvanced struct {
	Arg0   [32]byte
	Parent [32]byte
	Left   [32]byte
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterMatchAdvanced is a free log retrieval operation binding the contract event 0x29ff393c59c37f91e930fad4d88447efc58cf5d7c048499e1f20edb369941378.
//
// Solidity: event matchAdvanced(bytes32 indexed arg0, bytes32 parent, bytes32 left)
func (_RootTournament *RootTournamentFilterer) FilterMatchAdvanced(opts *bind.FilterOpts, arg0 [][32]byte) (*RootTournamentMatchAdvancedIterator, error) {

	var arg0Rule []interface{}
	for _, arg0Item := range arg0 {
		arg0Rule = append(arg0Rule, arg0Item)
	}

	logs, sub, err := _RootTournament.contract.FilterLogs(opts, "matchAdvanced", arg0Rule)
	if err != nil {
		return nil, err
	}
	return &RootTournamentMatchAdvancedIterator{contract: _RootTournament.contract, event: "matchAdvanced", logs: logs, sub: sub}, nil
}

// WatchMatchAdvanced is a free log subscription operation binding the contract event 0x29ff393c59c37f91e930fad4d88447efc58cf5d7c048499e1f20edb369941378.
//
// Solidity: event matchAdvanced(bytes32 indexed arg0, bytes32 parent, bytes32 left)
func (_RootTournament *RootTournamentFilterer) WatchMatchAdvanced(opts *bind.WatchOpts, sink chan<- *RootTournamentMatchAdvanced, arg0 [][32]byte) (event.Subscription, error) {

	var arg0Rule []interface{}
	for _, arg0Item := range arg0 {
		arg0Rule = append(arg0Rule, arg0Item)
	}

	logs, sub, err := _RootTournament.contract.WatchLogs(opts, "matchAdvanced", arg0Rule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(RootTournamentMatchAdvanced)
				if err := _RootTournament.contract.UnpackLog(event, "matchAdvanced", log); err != nil {
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

// ParseMatchAdvanced is a log parse operation binding the contract event 0x29ff393c59c37f91e930fad4d88447efc58cf5d7c048499e1f20edb369941378.
//
// Solidity: event matchAdvanced(bytes32 indexed arg0, bytes32 parent, bytes32 left)
func (_RootTournament *RootTournamentFilterer) ParseMatchAdvanced(log types.Log) (*RootTournamentMatchAdvanced, error) {
	event := new(RootTournamentMatchAdvanced)
	if err := _RootTournament.contract.UnpackLog(event, "matchAdvanced", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// RootTournamentMatchCreatedIterator is returned from FilterMatchCreated and is used to iterate over the raw logs and unpacked data for MatchCreated events raised by the RootTournament contract.
type RootTournamentMatchCreatedIterator struct {
	Event *RootTournamentMatchCreated // Event containing the contract specifics and raw log

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
func (it *RootTournamentMatchCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(RootTournamentMatchCreated)
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
		it.Event = new(RootTournamentMatchCreated)
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
func (it *RootTournamentMatchCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *RootTournamentMatchCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// RootTournamentMatchCreated represents a MatchCreated event raised by the RootTournament contract.
type RootTournamentMatchCreated struct {
	One       [32]byte
	Two       [32]byte
	LeftOfTwo [32]byte
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterMatchCreated is a free log retrieval operation binding the contract event 0x32911001007d8c9879b608566be8acc2184592f0a43706f804f285455bb0f52e.
//
// Solidity: event matchCreated(bytes32 indexed one, bytes32 indexed two, bytes32 leftOfTwo)
func (_RootTournament *RootTournamentFilterer) FilterMatchCreated(opts *bind.FilterOpts, one [][32]byte, two [][32]byte) (*RootTournamentMatchCreatedIterator, error) {

	var oneRule []interface{}
	for _, oneItem := range one {
		oneRule = append(oneRule, oneItem)
	}
	var twoRule []interface{}
	for _, twoItem := range two {
		twoRule = append(twoRule, twoItem)
	}

	logs, sub, err := _RootTournament.contract.FilterLogs(opts, "matchCreated", oneRule, twoRule)
	if err != nil {
		return nil, err
	}
	return &RootTournamentMatchCreatedIterator{contract: _RootTournament.contract, event: "matchCreated", logs: logs, sub: sub}, nil
}

// WatchMatchCreated is a free log subscription operation binding the contract event 0x32911001007d8c9879b608566be8acc2184592f0a43706f804f285455bb0f52e.
//
// Solidity: event matchCreated(bytes32 indexed one, bytes32 indexed two, bytes32 leftOfTwo)
func (_RootTournament *RootTournamentFilterer) WatchMatchCreated(opts *bind.WatchOpts, sink chan<- *RootTournamentMatchCreated, one [][32]byte, two [][32]byte) (event.Subscription, error) {

	var oneRule []interface{}
	for _, oneItem := range one {
		oneRule = append(oneRule, oneItem)
	}
	var twoRule []interface{}
	for _, twoItem := range two {
		twoRule = append(twoRule, twoItem)
	}

	logs, sub, err := _RootTournament.contract.WatchLogs(opts, "matchCreated", oneRule, twoRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(RootTournamentMatchCreated)
				if err := _RootTournament.contract.UnpackLog(event, "matchCreated", log); err != nil {
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

// ParseMatchCreated is a log parse operation binding the contract event 0x32911001007d8c9879b608566be8acc2184592f0a43706f804f285455bb0f52e.
//
// Solidity: event matchCreated(bytes32 indexed one, bytes32 indexed two, bytes32 leftOfTwo)
func (_RootTournament *RootTournamentFilterer) ParseMatchCreated(log types.Log) (*RootTournamentMatchCreated, error) {
	event := new(RootTournamentMatchCreated)
	if err := _RootTournament.contract.UnpackLog(event, "matchCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// RootTournamentMatchDeletedIterator is returned from FilterMatchDeleted and is used to iterate over the raw logs and unpacked data for MatchDeleted events raised by the RootTournament contract.
type RootTournamentMatchDeletedIterator struct {
	Event *RootTournamentMatchDeleted // Event containing the contract specifics and raw log

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
func (it *RootTournamentMatchDeletedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(RootTournamentMatchDeleted)
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
		it.Event = new(RootTournamentMatchDeleted)
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
func (it *RootTournamentMatchDeletedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *RootTournamentMatchDeletedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// RootTournamentMatchDeleted represents a MatchDeleted event raised by the RootTournament contract.
type RootTournamentMatchDeleted struct {
	Arg0 [32]byte
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterMatchDeleted is a free log retrieval operation binding the contract event 0x0afce37c521a4613a2db0c4983987a3c4af722e33d3412963fccbc0eb0df0d28.
//
// Solidity: event matchDeleted(bytes32 arg0)
func (_RootTournament *RootTournamentFilterer) FilterMatchDeleted(opts *bind.FilterOpts) (*RootTournamentMatchDeletedIterator, error) {

	logs, sub, err := _RootTournament.contract.FilterLogs(opts, "matchDeleted")
	if err != nil {
		return nil, err
	}
	return &RootTournamentMatchDeletedIterator{contract: _RootTournament.contract, event: "matchDeleted", logs: logs, sub: sub}, nil
}

// WatchMatchDeleted is a free log subscription operation binding the contract event 0x0afce37c521a4613a2db0c4983987a3c4af722e33d3412963fccbc0eb0df0d28.
//
// Solidity: event matchDeleted(bytes32 arg0)
func (_RootTournament *RootTournamentFilterer) WatchMatchDeleted(opts *bind.WatchOpts, sink chan<- *RootTournamentMatchDeleted) (event.Subscription, error) {

	logs, sub, err := _RootTournament.contract.WatchLogs(opts, "matchDeleted")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(RootTournamentMatchDeleted)
				if err := _RootTournament.contract.UnpackLog(event, "matchDeleted", log); err != nil {
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

// ParseMatchDeleted is a log parse operation binding the contract event 0x0afce37c521a4613a2db0c4983987a3c4af722e33d3412963fccbc0eb0df0d28.
//
// Solidity: event matchDeleted(bytes32 arg0)
func (_RootTournament *RootTournamentFilterer) ParseMatchDeleted(log types.Log) (*RootTournamentMatchDeleted, error) {
	event := new(RootTournamentMatchDeleted)
	if err := _RootTournament.contract.UnpackLog(event, "matchDeleted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
