// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package toptournament

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

// TopTournamentMetaData contains all meta data concerning the TopTournament contract.
var TopTournamentMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"advanceMatch\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_newLeftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_newRightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"arbitrationResult\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"canWinMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"eliminateInnerTournament\",\"inputs\":[{\"name\":\"_childTournament\",\"type\":\"address\",\"internalType\":\"contractNonRootTournament\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"eliminateMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getCommitment\",\"inputs\":[{\"name\":\"_commitmentRoot\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structClock.State\",\"components\":[{\"name\":\"allowance\",\"type\":\"uint64\",\"internalType\":\"Time.Duration\"},{\"name\":\"startInstant\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"}]},{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMatch\",\"inputs\":[{\"name\":\"_matchIdHash\",\"type\":\"bytes32\",\"internalType\":\"Match.IdHash\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structMatch.State\",\"components\":[{\"name\":\"otherParent\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"runningLeafPosition\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentHeight\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"log2step\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"height\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMatchCycle\",\"inputs\":[{\"name\":\"_matchIdHash\",\"type\":\"bytes32\",\"internalType\":\"Match.IdHash\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isClosed\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isFinished\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"joinTournament\",\"inputs\":[{\"name\":\"_finalState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"_proof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"sealInnerMatchAndCreateInnerTournament\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"_leftLeaf\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightLeaf\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_agreeHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"_agreeHashProof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"timeFinished\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"tournamentLevelConstants\",\"inputs\":[],\"outputs\":[{\"name\":\"_maxLevel\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_level\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_log2step\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_height\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"winInnerTournament\",\"inputs\":[{\"name\":\"_childTournament\",\"type\":\"address\",\"internalType\":\"contractNonRootTournament\"},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"winMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"commitmentJoined\",\"inputs\":[{\"name\":\"root\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchAdvanced\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Match.IdHash\"},{\"name\":\"parent\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"},{\"name\":\"left\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchCreated\",\"inputs\":[{\"name\":\"one\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"two\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"leftOfTwo\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchDeleted\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Match.IdHash\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"newInnerTournament\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Match.IdHash\"},{\"name\":\"\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractNonRootTournament\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"ChildTournamentCannotBeEliminated\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ChildTournamentMustBeEliminated\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ChildTournamentNotFinished\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"CommitmentMismatch\",\"inputs\":[{\"name\":\"received\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"expected\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"type\":\"error\",\"name\":\"EliminateByTimeout\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"IncorrectAgreeState\",\"inputs\":[{\"name\":\"initialState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"agreeState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}]},{\"type\":\"error\",\"name\":\"InvalidContestedFinalState\",\"inputs\":[{\"name\":\"contestedFinalStateOne\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"contestedFinalStateTwo\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"finalState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}]},{\"type\":\"error\",\"name\":\"LengthMismatch\",\"inputs\":[{\"name\":\"treeHeight\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"siblingsLength\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]},{\"type\":\"error\",\"name\":\"TournamentIsClosed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TournamentIsFinished\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WinByTimeout\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WrongChildren\",\"inputs\":[{\"name\":\"commitment\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"parent\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"left\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"right\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"type\":\"error\",\"name\":\"WrongTournamentWinner\",\"inputs\":[{\"name\":\"commitmentRoot\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"winner\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}]",
}

// TopTournamentABI is the input ABI used to generate the binding from.
// Deprecated: Use TopTournamentMetaData.ABI instead.
var TopTournamentABI = TopTournamentMetaData.ABI

// TopTournament is an auto generated Go binding around an Ethereum contract.
type TopTournament struct {
	TopTournamentCaller     // Read-only binding to the contract
	TopTournamentTransactor // Write-only binding to the contract
	TopTournamentFilterer   // Log filterer for contract events
}

// TopTournamentCaller is an auto generated read-only Go binding around an Ethereum contract.
type TopTournamentCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TopTournamentTransactor is an auto generated write-only Go binding around an Ethereum contract.
type TopTournamentTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TopTournamentFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type TopTournamentFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TopTournamentSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type TopTournamentSession struct {
	Contract     *TopTournament    // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// TopTournamentCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type TopTournamentCallerSession struct {
	Contract *TopTournamentCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts        // Call options to use throughout this session
}

// TopTournamentTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type TopTournamentTransactorSession struct {
	Contract     *TopTournamentTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts        // Transaction auth options to use throughout this session
}

// TopTournamentRaw is an auto generated low-level Go binding around an Ethereum contract.
type TopTournamentRaw struct {
	Contract *TopTournament // Generic contract binding to access the raw methods on
}

// TopTournamentCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type TopTournamentCallerRaw struct {
	Contract *TopTournamentCaller // Generic read-only contract binding to access the raw methods on
}

// TopTournamentTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type TopTournamentTransactorRaw struct {
	Contract *TopTournamentTransactor // Generic write-only contract binding to access the raw methods on
}

// NewTopTournament creates a new instance of TopTournament, bound to a specific deployed contract.
func NewTopTournament(address common.Address, backend bind.ContractBackend) (*TopTournament, error) {
	contract, err := bindTopTournament(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &TopTournament{TopTournamentCaller: TopTournamentCaller{contract: contract}, TopTournamentTransactor: TopTournamentTransactor{contract: contract}, TopTournamentFilterer: TopTournamentFilterer{contract: contract}}, nil
}

// NewTopTournamentCaller creates a new read-only instance of TopTournament, bound to a specific deployed contract.
func NewTopTournamentCaller(address common.Address, caller bind.ContractCaller) (*TopTournamentCaller, error) {
	contract, err := bindTopTournament(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &TopTournamentCaller{contract: contract}, nil
}

// NewTopTournamentTransactor creates a new write-only instance of TopTournament, bound to a specific deployed contract.
func NewTopTournamentTransactor(address common.Address, transactor bind.ContractTransactor) (*TopTournamentTransactor, error) {
	contract, err := bindTopTournament(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &TopTournamentTransactor{contract: contract}, nil
}

// NewTopTournamentFilterer creates a new log filterer instance of TopTournament, bound to a specific deployed contract.
func NewTopTournamentFilterer(address common.Address, filterer bind.ContractFilterer) (*TopTournamentFilterer, error) {
	contract, err := bindTopTournament(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &TopTournamentFilterer{contract: contract}, nil
}

// bindTopTournament binds a generic wrapper to an already deployed contract.
func bindTopTournament(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := TopTournamentMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_TopTournament *TopTournamentRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _TopTournament.Contract.TopTournamentCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_TopTournament *TopTournamentRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _TopTournament.Contract.TopTournamentTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_TopTournament *TopTournamentRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _TopTournament.Contract.TopTournamentTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_TopTournament *TopTournamentCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _TopTournament.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_TopTournament *TopTournamentTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _TopTournament.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_TopTournament *TopTournamentTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _TopTournament.Contract.contract.Transact(opts, method, params...)
}

// ArbitrationResult is a free data retrieval call binding the contract method 0xcb2773db.
//
// Solidity: function arbitrationResult() view returns(bool, bytes32, bytes32)
func (_TopTournament *TopTournamentCaller) ArbitrationResult(opts *bind.CallOpts) (bool, [32]byte, [32]byte, error) {
	var out []interface{}
	err := _TopTournament.contract.Call(opts, &out, "arbitrationResult")

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
func (_TopTournament *TopTournamentSession) ArbitrationResult() (bool, [32]byte, [32]byte, error) {
	return _TopTournament.Contract.ArbitrationResult(&_TopTournament.CallOpts)
}

// ArbitrationResult is a free data retrieval call binding the contract method 0xcb2773db.
//
// Solidity: function arbitrationResult() view returns(bool, bytes32, bytes32)
func (_TopTournament *TopTournamentCallerSession) ArbitrationResult() (bool, [32]byte, [32]byte, error) {
	return _TopTournament.Contract.ArbitrationResult(&_TopTournament.CallOpts)
}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_TopTournament *TopTournamentCaller) CanWinMatchByTimeout(opts *bind.CallOpts, _matchId MatchId) (bool, error) {
	var out []interface{}
	err := _TopTournament.contract.Call(opts, &out, "canWinMatchByTimeout", _matchId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_TopTournament *TopTournamentSession) CanWinMatchByTimeout(_matchId MatchId) (bool, error) {
	return _TopTournament.Contract.CanWinMatchByTimeout(&_TopTournament.CallOpts, _matchId)
}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_TopTournament *TopTournamentCallerSession) CanWinMatchByTimeout(_matchId MatchId) (bool, error) {
	return _TopTournament.Contract.CanWinMatchByTimeout(&_TopTournament.CallOpts, _matchId)
}

// GetCommitment is a free data retrieval call binding the contract method 0x7795820c.
//
// Solidity: function getCommitment(bytes32 _commitmentRoot) view returns((uint64,uint64), bytes32)
func (_TopTournament *TopTournamentCaller) GetCommitment(opts *bind.CallOpts, _commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	var out []interface{}
	err := _TopTournament.contract.Call(opts, &out, "getCommitment", _commitmentRoot)

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
func (_TopTournament *TopTournamentSession) GetCommitment(_commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	return _TopTournament.Contract.GetCommitment(&_TopTournament.CallOpts, _commitmentRoot)
}

// GetCommitment is a free data retrieval call binding the contract method 0x7795820c.
//
// Solidity: function getCommitment(bytes32 _commitmentRoot) view returns((uint64,uint64), bytes32)
func (_TopTournament *TopTournamentCallerSession) GetCommitment(_commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	return _TopTournament.Contract.GetCommitment(&_TopTournament.CallOpts, _commitmentRoot)
}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_TopTournament *TopTournamentCaller) GetMatch(opts *bind.CallOpts, _matchIdHash [32]byte) (MatchState, error) {
	var out []interface{}
	err := _TopTournament.contract.Call(opts, &out, "getMatch", _matchIdHash)

	if err != nil {
		return *new(MatchState), err
	}

	out0 := *abi.ConvertType(out[0], new(MatchState)).(*MatchState)

	return out0, err

}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_TopTournament *TopTournamentSession) GetMatch(_matchIdHash [32]byte) (MatchState, error) {
	return _TopTournament.Contract.GetMatch(&_TopTournament.CallOpts, _matchIdHash)
}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_TopTournament *TopTournamentCallerSession) GetMatch(_matchIdHash [32]byte) (MatchState, error) {
	return _TopTournament.Contract.GetMatch(&_TopTournament.CallOpts, _matchIdHash)
}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_TopTournament *TopTournamentCaller) GetMatchCycle(opts *bind.CallOpts, _matchIdHash [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _TopTournament.contract.Call(opts, &out, "getMatchCycle", _matchIdHash)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_TopTournament *TopTournamentSession) GetMatchCycle(_matchIdHash [32]byte) (*big.Int, error) {
	return _TopTournament.Contract.GetMatchCycle(&_TopTournament.CallOpts, _matchIdHash)
}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_TopTournament *TopTournamentCallerSession) GetMatchCycle(_matchIdHash [32]byte) (*big.Int, error) {
	return _TopTournament.Contract.GetMatchCycle(&_TopTournament.CallOpts, _matchIdHash)
}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_TopTournament *TopTournamentCaller) IsClosed(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _TopTournament.contract.Call(opts, &out, "isClosed")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_TopTournament *TopTournamentSession) IsClosed() (bool, error) {
	return _TopTournament.Contract.IsClosed(&_TopTournament.CallOpts)
}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_TopTournament *TopTournamentCallerSession) IsClosed() (bool, error) {
	return _TopTournament.Contract.IsClosed(&_TopTournament.CallOpts)
}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_TopTournament *TopTournamentCaller) IsFinished(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _TopTournament.contract.Call(opts, &out, "isFinished")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_TopTournament *TopTournamentSession) IsFinished() (bool, error) {
	return _TopTournament.Contract.IsFinished(&_TopTournament.CallOpts)
}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_TopTournament *TopTournamentCallerSession) IsFinished() (bool, error) {
	return _TopTournament.Contract.IsFinished(&_TopTournament.CallOpts)
}

// TimeFinished is a free data retrieval call binding the contract method 0x39cdfaf2.
//
// Solidity: function timeFinished() view returns(bool, uint64)
func (_TopTournament *TopTournamentCaller) TimeFinished(opts *bind.CallOpts) (bool, uint64, error) {
	var out []interface{}
	err := _TopTournament.contract.Call(opts, &out, "timeFinished")

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
func (_TopTournament *TopTournamentSession) TimeFinished() (bool, uint64, error) {
	return _TopTournament.Contract.TimeFinished(&_TopTournament.CallOpts)
}

// TimeFinished is a free data retrieval call binding the contract method 0x39cdfaf2.
//
// Solidity: function timeFinished() view returns(bool, uint64)
func (_TopTournament *TopTournamentCallerSession) TimeFinished() (bool, uint64, error) {
	return _TopTournament.Contract.TimeFinished(&_TopTournament.CallOpts)
}

// TournamentLevelConstants is a free data retrieval call binding the contract method 0xa1af906b.
//
// Solidity: function tournamentLevelConstants() view returns(uint64 _maxLevel, uint64 _level, uint64 _log2step, uint64 _height)
func (_TopTournament *TopTournamentCaller) TournamentLevelConstants(opts *bind.CallOpts) (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	var out []interface{}
	err := _TopTournament.contract.Call(opts, &out, "tournamentLevelConstants")

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
func (_TopTournament *TopTournamentSession) TournamentLevelConstants() (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	return _TopTournament.Contract.TournamentLevelConstants(&_TopTournament.CallOpts)
}

// TournamentLevelConstants is a free data retrieval call binding the contract method 0xa1af906b.
//
// Solidity: function tournamentLevelConstants() view returns(uint64 _maxLevel, uint64 _level, uint64 _log2step, uint64 _height)
func (_TopTournament *TopTournamentCallerSession) TournamentLevelConstants() (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	return _TopTournament.Contract.TournamentLevelConstants(&_TopTournament.CallOpts)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_TopTournament *TopTournamentTransactor) AdvanceMatch(opts *bind.TransactOpts, _matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _TopTournament.contract.Transact(opts, "advanceMatch", _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_TopTournament *TopTournamentSession) AdvanceMatch(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _TopTournament.Contract.AdvanceMatch(&_TopTournament.TransactOpts, _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_TopTournament *TopTournamentTransactorSession) AdvanceMatch(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _TopTournament.Contract.AdvanceMatch(&_TopTournament.TransactOpts, _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// EliminateInnerTournament is a paid mutator transaction binding the contract method 0x26860b49.
//
// Solidity: function eliminateInnerTournament(address _childTournament) returns()
func (_TopTournament *TopTournamentTransactor) EliminateInnerTournament(opts *bind.TransactOpts, _childTournament common.Address) (*types.Transaction, error) {
	return _TopTournament.contract.Transact(opts, "eliminateInnerTournament", _childTournament)
}

// EliminateInnerTournament is a paid mutator transaction binding the contract method 0x26860b49.
//
// Solidity: function eliminateInnerTournament(address _childTournament) returns()
func (_TopTournament *TopTournamentSession) EliminateInnerTournament(_childTournament common.Address) (*types.Transaction, error) {
	return _TopTournament.Contract.EliminateInnerTournament(&_TopTournament.TransactOpts, _childTournament)
}

// EliminateInnerTournament is a paid mutator transaction binding the contract method 0x26860b49.
//
// Solidity: function eliminateInnerTournament(address _childTournament) returns()
func (_TopTournament *TopTournamentTransactorSession) EliminateInnerTournament(_childTournament common.Address) (*types.Transaction, error) {
	return _TopTournament.Contract.EliminateInnerTournament(&_TopTournament.TransactOpts, _childTournament)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_TopTournament *TopTournamentTransactor) EliminateMatchByTimeout(opts *bind.TransactOpts, _matchId MatchId) (*types.Transaction, error) {
	return _TopTournament.contract.Transact(opts, "eliminateMatchByTimeout", _matchId)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_TopTournament *TopTournamentSession) EliminateMatchByTimeout(_matchId MatchId) (*types.Transaction, error) {
	return _TopTournament.Contract.EliminateMatchByTimeout(&_TopTournament.TransactOpts, _matchId)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_TopTournament *TopTournamentTransactorSession) EliminateMatchByTimeout(_matchId MatchId) (*types.Transaction, error) {
	return _TopTournament.Contract.EliminateMatchByTimeout(&_TopTournament.TransactOpts, _matchId)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_TopTournament *TopTournamentTransactor) JoinTournament(opts *bind.TransactOpts, _finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _TopTournament.contract.Transact(opts, "joinTournament", _finalState, _proof, _leftNode, _rightNode)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_TopTournament *TopTournamentSession) JoinTournament(_finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _TopTournament.Contract.JoinTournament(&_TopTournament.TransactOpts, _finalState, _proof, _leftNode, _rightNode)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_TopTournament *TopTournamentTransactorSession) JoinTournament(_finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _TopTournament.Contract.JoinTournament(&_TopTournament.TransactOpts, _finalState, _proof, _leftNode, _rightNode)
}

// SealInnerMatchAndCreateInnerTournament is a paid mutator transaction binding the contract method 0x3f36e2fe.
//
// Solidity: function sealInnerMatchAndCreateInnerTournament((bytes32,bytes32) _matchId, bytes32 _leftLeaf, bytes32 _rightLeaf, bytes32 _agreeHash, bytes32[] _agreeHashProof) returns()
func (_TopTournament *TopTournamentTransactor) SealInnerMatchAndCreateInnerTournament(opts *bind.TransactOpts, _matchId MatchId, _leftLeaf [32]byte, _rightLeaf [32]byte, _agreeHash [32]byte, _agreeHashProof [][32]byte) (*types.Transaction, error) {
	return _TopTournament.contract.Transact(opts, "sealInnerMatchAndCreateInnerTournament", _matchId, _leftLeaf, _rightLeaf, _agreeHash, _agreeHashProof)
}

// SealInnerMatchAndCreateInnerTournament is a paid mutator transaction binding the contract method 0x3f36e2fe.
//
// Solidity: function sealInnerMatchAndCreateInnerTournament((bytes32,bytes32) _matchId, bytes32 _leftLeaf, bytes32 _rightLeaf, bytes32 _agreeHash, bytes32[] _agreeHashProof) returns()
func (_TopTournament *TopTournamentSession) SealInnerMatchAndCreateInnerTournament(_matchId MatchId, _leftLeaf [32]byte, _rightLeaf [32]byte, _agreeHash [32]byte, _agreeHashProof [][32]byte) (*types.Transaction, error) {
	return _TopTournament.Contract.SealInnerMatchAndCreateInnerTournament(&_TopTournament.TransactOpts, _matchId, _leftLeaf, _rightLeaf, _agreeHash, _agreeHashProof)
}

// SealInnerMatchAndCreateInnerTournament is a paid mutator transaction binding the contract method 0x3f36e2fe.
//
// Solidity: function sealInnerMatchAndCreateInnerTournament((bytes32,bytes32) _matchId, bytes32 _leftLeaf, bytes32 _rightLeaf, bytes32 _agreeHash, bytes32[] _agreeHashProof) returns()
func (_TopTournament *TopTournamentTransactorSession) SealInnerMatchAndCreateInnerTournament(_matchId MatchId, _leftLeaf [32]byte, _rightLeaf [32]byte, _agreeHash [32]byte, _agreeHashProof [][32]byte) (*types.Transaction, error) {
	return _TopTournament.Contract.SealInnerMatchAndCreateInnerTournament(&_TopTournament.TransactOpts, _matchId, _leftLeaf, _rightLeaf, _agreeHash, _agreeHashProof)
}

// WinInnerTournament is a paid mutator transaction binding the contract method 0x4a95153e.
//
// Solidity: function winInnerTournament(address _childTournament, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_TopTournament *TopTournamentTransactor) WinInnerTournament(opts *bind.TransactOpts, _childTournament common.Address, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _TopTournament.contract.Transact(opts, "winInnerTournament", _childTournament, _leftNode, _rightNode)
}

// WinInnerTournament is a paid mutator transaction binding the contract method 0x4a95153e.
//
// Solidity: function winInnerTournament(address _childTournament, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_TopTournament *TopTournamentSession) WinInnerTournament(_childTournament common.Address, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _TopTournament.Contract.WinInnerTournament(&_TopTournament.TransactOpts, _childTournament, _leftNode, _rightNode)
}

// WinInnerTournament is a paid mutator transaction binding the contract method 0x4a95153e.
//
// Solidity: function winInnerTournament(address _childTournament, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_TopTournament *TopTournamentTransactorSession) WinInnerTournament(_childTournament common.Address, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _TopTournament.Contract.WinInnerTournament(&_TopTournament.TransactOpts, _childTournament, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_TopTournament *TopTournamentTransactor) WinMatchByTimeout(opts *bind.TransactOpts, _matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _TopTournament.contract.Transact(opts, "winMatchByTimeout", _matchId, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_TopTournament *TopTournamentSession) WinMatchByTimeout(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _TopTournament.Contract.WinMatchByTimeout(&_TopTournament.TransactOpts, _matchId, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_TopTournament *TopTournamentTransactorSession) WinMatchByTimeout(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _TopTournament.Contract.WinMatchByTimeout(&_TopTournament.TransactOpts, _matchId, _leftNode, _rightNode)
}

// TopTournamentCommitmentJoinedIterator is returned from FilterCommitmentJoined and is used to iterate over the raw logs and unpacked data for CommitmentJoined events raised by the TopTournament contract.
type TopTournamentCommitmentJoinedIterator struct {
	Event *TopTournamentCommitmentJoined // Event containing the contract specifics and raw log

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
func (it *TopTournamentCommitmentJoinedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TopTournamentCommitmentJoined)
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
		it.Event = new(TopTournamentCommitmentJoined)
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
func (it *TopTournamentCommitmentJoinedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TopTournamentCommitmentJoinedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TopTournamentCommitmentJoined represents a CommitmentJoined event raised by the TopTournament contract.
type TopTournamentCommitmentJoined struct {
	Root [32]byte
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterCommitmentJoined is a free log retrieval operation binding the contract event 0xe53537f202911d376d6e285835b2a2016e83e99fbe84a059d445cc2be4807262.
//
// Solidity: event commitmentJoined(bytes32 root)
func (_TopTournament *TopTournamentFilterer) FilterCommitmentJoined(opts *bind.FilterOpts) (*TopTournamentCommitmentJoinedIterator, error) {

	logs, sub, err := _TopTournament.contract.FilterLogs(opts, "commitmentJoined")
	if err != nil {
		return nil, err
	}
	return &TopTournamentCommitmentJoinedIterator{contract: _TopTournament.contract, event: "commitmentJoined", logs: logs, sub: sub}, nil
}

// WatchCommitmentJoined is a free log subscription operation binding the contract event 0xe53537f202911d376d6e285835b2a2016e83e99fbe84a059d445cc2be4807262.
//
// Solidity: event commitmentJoined(bytes32 root)
func (_TopTournament *TopTournamentFilterer) WatchCommitmentJoined(opts *bind.WatchOpts, sink chan<- *TopTournamentCommitmentJoined) (event.Subscription, error) {

	logs, sub, err := _TopTournament.contract.WatchLogs(opts, "commitmentJoined")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TopTournamentCommitmentJoined)
				if err := _TopTournament.contract.UnpackLog(event, "commitmentJoined", log); err != nil {
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
func (_TopTournament *TopTournamentFilterer) ParseCommitmentJoined(log types.Log) (*TopTournamentCommitmentJoined, error) {
	event := new(TopTournamentCommitmentJoined)
	if err := _TopTournament.contract.UnpackLog(event, "commitmentJoined", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TopTournamentMatchAdvancedIterator is returned from FilterMatchAdvanced and is used to iterate over the raw logs and unpacked data for MatchAdvanced events raised by the TopTournament contract.
type TopTournamentMatchAdvancedIterator struct {
	Event *TopTournamentMatchAdvanced // Event containing the contract specifics and raw log

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
func (it *TopTournamentMatchAdvancedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TopTournamentMatchAdvanced)
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
		it.Event = new(TopTournamentMatchAdvanced)
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
func (it *TopTournamentMatchAdvancedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TopTournamentMatchAdvancedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TopTournamentMatchAdvanced represents a MatchAdvanced event raised by the TopTournament contract.
type TopTournamentMatchAdvanced struct {
	Arg0   [32]byte
	Parent [32]byte
	Left   [32]byte
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterMatchAdvanced is a free log retrieval operation binding the contract event 0x29ff393c59c37f91e930fad4d88447efc58cf5d7c048499e1f20edb369941378.
//
// Solidity: event matchAdvanced(bytes32 indexed arg0, bytes32 parent, bytes32 left)
func (_TopTournament *TopTournamentFilterer) FilterMatchAdvanced(opts *bind.FilterOpts, arg0 [][32]byte) (*TopTournamentMatchAdvancedIterator, error) {

	var arg0Rule []interface{}
	for _, arg0Item := range arg0 {
		arg0Rule = append(arg0Rule, arg0Item)
	}

	logs, sub, err := _TopTournament.contract.FilterLogs(opts, "matchAdvanced", arg0Rule)
	if err != nil {
		return nil, err
	}
	return &TopTournamentMatchAdvancedIterator{contract: _TopTournament.contract, event: "matchAdvanced", logs: logs, sub: sub}, nil
}

// WatchMatchAdvanced is a free log subscription operation binding the contract event 0x29ff393c59c37f91e930fad4d88447efc58cf5d7c048499e1f20edb369941378.
//
// Solidity: event matchAdvanced(bytes32 indexed arg0, bytes32 parent, bytes32 left)
func (_TopTournament *TopTournamentFilterer) WatchMatchAdvanced(opts *bind.WatchOpts, sink chan<- *TopTournamentMatchAdvanced, arg0 [][32]byte) (event.Subscription, error) {

	var arg0Rule []interface{}
	for _, arg0Item := range arg0 {
		arg0Rule = append(arg0Rule, arg0Item)
	}

	logs, sub, err := _TopTournament.contract.WatchLogs(opts, "matchAdvanced", arg0Rule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TopTournamentMatchAdvanced)
				if err := _TopTournament.contract.UnpackLog(event, "matchAdvanced", log); err != nil {
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
func (_TopTournament *TopTournamentFilterer) ParseMatchAdvanced(log types.Log) (*TopTournamentMatchAdvanced, error) {
	event := new(TopTournamentMatchAdvanced)
	if err := _TopTournament.contract.UnpackLog(event, "matchAdvanced", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TopTournamentMatchCreatedIterator is returned from FilterMatchCreated and is used to iterate over the raw logs and unpacked data for MatchCreated events raised by the TopTournament contract.
type TopTournamentMatchCreatedIterator struct {
	Event *TopTournamentMatchCreated // Event containing the contract specifics and raw log

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
func (it *TopTournamentMatchCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TopTournamentMatchCreated)
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
		it.Event = new(TopTournamentMatchCreated)
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
func (it *TopTournamentMatchCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TopTournamentMatchCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TopTournamentMatchCreated represents a MatchCreated event raised by the TopTournament contract.
type TopTournamentMatchCreated struct {
	One       [32]byte
	Two       [32]byte
	LeftOfTwo [32]byte
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterMatchCreated is a free log retrieval operation binding the contract event 0x32911001007d8c9879b608566be8acc2184592f0a43706f804f285455bb0f52e.
//
// Solidity: event matchCreated(bytes32 indexed one, bytes32 indexed two, bytes32 leftOfTwo)
func (_TopTournament *TopTournamentFilterer) FilterMatchCreated(opts *bind.FilterOpts, one [][32]byte, two [][32]byte) (*TopTournamentMatchCreatedIterator, error) {

	var oneRule []interface{}
	for _, oneItem := range one {
		oneRule = append(oneRule, oneItem)
	}
	var twoRule []interface{}
	for _, twoItem := range two {
		twoRule = append(twoRule, twoItem)
	}

	logs, sub, err := _TopTournament.contract.FilterLogs(opts, "matchCreated", oneRule, twoRule)
	if err != nil {
		return nil, err
	}
	return &TopTournamentMatchCreatedIterator{contract: _TopTournament.contract, event: "matchCreated", logs: logs, sub: sub}, nil
}

// WatchMatchCreated is a free log subscription operation binding the contract event 0x32911001007d8c9879b608566be8acc2184592f0a43706f804f285455bb0f52e.
//
// Solidity: event matchCreated(bytes32 indexed one, bytes32 indexed two, bytes32 leftOfTwo)
func (_TopTournament *TopTournamentFilterer) WatchMatchCreated(opts *bind.WatchOpts, sink chan<- *TopTournamentMatchCreated, one [][32]byte, two [][32]byte) (event.Subscription, error) {

	var oneRule []interface{}
	for _, oneItem := range one {
		oneRule = append(oneRule, oneItem)
	}
	var twoRule []interface{}
	for _, twoItem := range two {
		twoRule = append(twoRule, twoItem)
	}

	logs, sub, err := _TopTournament.contract.WatchLogs(opts, "matchCreated", oneRule, twoRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TopTournamentMatchCreated)
				if err := _TopTournament.contract.UnpackLog(event, "matchCreated", log); err != nil {
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
func (_TopTournament *TopTournamentFilterer) ParseMatchCreated(log types.Log) (*TopTournamentMatchCreated, error) {
	event := new(TopTournamentMatchCreated)
	if err := _TopTournament.contract.UnpackLog(event, "matchCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TopTournamentMatchDeletedIterator is returned from FilterMatchDeleted and is used to iterate over the raw logs and unpacked data for MatchDeleted events raised by the TopTournament contract.
type TopTournamentMatchDeletedIterator struct {
	Event *TopTournamentMatchDeleted // Event containing the contract specifics and raw log

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
func (it *TopTournamentMatchDeletedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TopTournamentMatchDeleted)
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
		it.Event = new(TopTournamentMatchDeleted)
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
func (it *TopTournamentMatchDeletedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TopTournamentMatchDeletedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TopTournamentMatchDeleted represents a MatchDeleted event raised by the TopTournament contract.
type TopTournamentMatchDeleted struct {
	Arg0 [32]byte
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterMatchDeleted is a free log retrieval operation binding the contract event 0x0afce37c521a4613a2db0c4983987a3c4af722e33d3412963fccbc0eb0df0d28.
//
// Solidity: event matchDeleted(bytes32 arg0)
func (_TopTournament *TopTournamentFilterer) FilterMatchDeleted(opts *bind.FilterOpts) (*TopTournamentMatchDeletedIterator, error) {

	logs, sub, err := _TopTournament.contract.FilterLogs(opts, "matchDeleted")
	if err != nil {
		return nil, err
	}
	return &TopTournamentMatchDeletedIterator{contract: _TopTournament.contract, event: "matchDeleted", logs: logs, sub: sub}, nil
}

// WatchMatchDeleted is a free log subscription operation binding the contract event 0x0afce37c521a4613a2db0c4983987a3c4af722e33d3412963fccbc0eb0df0d28.
//
// Solidity: event matchDeleted(bytes32 arg0)
func (_TopTournament *TopTournamentFilterer) WatchMatchDeleted(opts *bind.WatchOpts, sink chan<- *TopTournamentMatchDeleted) (event.Subscription, error) {

	logs, sub, err := _TopTournament.contract.WatchLogs(opts, "matchDeleted")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TopTournamentMatchDeleted)
				if err := _TopTournament.contract.UnpackLog(event, "matchDeleted", log); err != nil {
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
func (_TopTournament *TopTournamentFilterer) ParseMatchDeleted(log types.Log) (*TopTournamentMatchDeleted, error) {
	event := new(TopTournamentMatchDeleted)
	if err := _TopTournament.contract.UnpackLog(event, "matchDeleted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TopTournamentNewInnerTournamentIterator is returned from FilterNewInnerTournament and is used to iterate over the raw logs and unpacked data for NewInnerTournament events raised by the TopTournament contract.
type TopTournamentNewInnerTournamentIterator struct {
	Event *TopTournamentNewInnerTournament // Event containing the contract specifics and raw log

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
func (it *TopTournamentNewInnerTournamentIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TopTournamentNewInnerTournament)
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
		it.Event = new(TopTournamentNewInnerTournament)
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
func (it *TopTournamentNewInnerTournamentIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TopTournamentNewInnerTournamentIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TopTournamentNewInnerTournament represents a NewInnerTournament event raised by the TopTournament contract.
type TopTournamentNewInnerTournament struct {
	Arg0 [32]byte
	Arg1 common.Address
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterNewInnerTournament is a free log retrieval operation binding the contract event 0x166b786700a644dcec96aa84a92c26ecfee11b50956409c8cc83616042290d79.
//
// Solidity: event newInnerTournament(bytes32 indexed arg0, address arg1)
func (_TopTournament *TopTournamentFilterer) FilterNewInnerTournament(opts *bind.FilterOpts, arg0 [][32]byte) (*TopTournamentNewInnerTournamentIterator, error) {

	var arg0Rule []interface{}
	for _, arg0Item := range arg0 {
		arg0Rule = append(arg0Rule, arg0Item)
	}

	logs, sub, err := _TopTournament.contract.FilterLogs(opts, "newInnerTournament", arg0Rule)
	if err != nil {
		return nil, err
	}
	return &TopTournamentNewInnerTournamentIterator{contract: _TopTournament.contract, event: "newInnerTournament", logs: logs, sub: sub}, nil
}

// WatchNewInnerTournament is a free log subscription operation binding the contract event 0x166b786700a644dcec96aa84a92c26ecfee11b50956409c8cc83616042290d79.
//
// Solidity: event newInnerTournament(bytes32 indexed arg0, address arg1)
func (_TopTournament *TopTournamentFilterer) WatchNewInnerTournament(opts *bind.WatchOpts, sink chan<- *TopTournamentNewInnerTournament, arg0 [][32]byte) (event.Subscription, error) {

	var arg0Rule []interface{}
	for _, arg0Item := range arg0 {
		arg0Rule = append(arg0Rule, arg0Item)
	}

	logs, sub, err := _TopTournament.contract.WatchLogs(opts, "newInnerTournament", arg0Rule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TopTournamentNewInnerTournament)
				if err := _TopTournament.contract.UnpackLog(event, "newInnerTournament", log); err != nil {
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

// ParseNewInnerTournament is a log parse operation binding the contract event 0x166b786700a644dcec96aa84a92c26ecfee11b50956409c8cc83616042290d79.
//
// Solidity: event newInnerTournament(bytes32 indexed arg0, address arg1)
func (_TopTournament *TopTournamentFilterer) ParseNewInnerTournament(log types.Log) (*TopTournamentNewInnerTournament, error) {
	event := new(TopTournamentNewInnerTournament)
	if err := _TopTournament.contract.UnpackLog(event, "newInnerTournament", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
