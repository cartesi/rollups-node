// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package nonroottournament

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

// NonRootTournamentMetaData contains all meta data concerning the NonRootTournament contract.
var NonRootTournamentMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"advanceMatch\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_newLeftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_newRightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"canBeEliminated\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"canWinMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"eliminateMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getCommitment\",\"inputs\":[{\"name\":\"_commitmentRoot\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structClock.State\",\"components\":[{\"name\":\"allowance\",\"type\":\"uint64\",\"internalType\":\"Time.Duration\"},{\"name\":\"startInstant\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"}]},{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMatch\",\"inputs\":[{\"name\":\"_matchIdHash\",\"type\":\"bytes32\",\"internalType\":\"Match.IdHash\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structMatch.State\",\"components\":[{\"name\":\"otherParent\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"runningLeafPosition\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentHeight\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"log2step\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"height\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMatchCycle\",\"inputs\":[{\"name\":\"_matchIdHash\",\"type\":\"bytes32\",\"internalType\":\"Match.IdHash\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"innerTournamentWinner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structClock.State\",\"components\":[{\"name\":\"allowance\",\"type\":\"uint64\",\"internalType\":\"Time.Duration\"},{\"name\":\"startInstant\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isClosed\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isFinished\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"joinTournament\",\"inputs\":[{\"name\":\"_finalState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"_proof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"timeFinished\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"tournamentLevelConstants\",\"inputs\":[],\"outputs\":[{\"name\":\"_maxLevel\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_level\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_log2step\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_height\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"winMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"commitmentJoined\",\"inputs\":[{\"name\":\"root\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchAdvanced\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Match.IdHash\"},{\"name\":\"parent\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"},{\"name\":\"left\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchCreated\",\"inputs\":[{\"name\":\"one\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"two\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"leftOfTwo\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchDeleted\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Match.IdHash\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"EliminateByTimeout\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidContestedFinalState\",\"inputs\":[{\"name\":\"contestedFinalStateOne\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"contestedFinalStateTwo\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"finalState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}]},{\"type\":\"error\",\"name\":\"TournamentIsClosed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TournamentIsFinished\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WinByTimeout\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WrongChildren\",\"inputs\":[{\"name\":\"commitment\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"parent\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"left\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"right\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}]",
}

// NonRootTournamentABI is the input ABI used to generate the binding from.
// Deprecated: Use NonRootTournamentMetaData.ABI instead.
var NonRootTournamentABI = NonRootTournamentMetaData.ABI

// NonRootTournament is an auto generated Go binding around an Ethereum contract.
type NonRootTournament struct {
	NonRootTournamentCaller     // Read-only binding to the contract
	NonRootTournamentTransactor // Write-only binding to the contract
	NonRootTournamentFilterer   // Log filterer for contract events
}

// NonRootTournamentCaller is an auto generated read-only Go binding around an Ethereum contract.
type NonRootTournamentCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// NonRootTournamentTransactor is an auto generated write-only Go binding around an Ethereum contract.
type NonRootTournamentTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// NonRootTournamentFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type NonRootTournamentFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// NonRootTournamentSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type NonRootTournamentSession struct {
	Contract     *NonRootTournament // Generic contract binding to set the session for
	CallOpts     bind.CallOpts      // Call options to use throughout this session
	TransactOpts bind.TransactOpts  // Transaction auth options to use throughout this session
}

// NonRootTournamentCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type NonRootTournamentCallerSession struct {
	Contract *NonRootTournamentCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts            // Call options to use throughout this session
}

// NonRootTournamentTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type NonRootTournamentTransactorSession struct {
	Contract     *NonRootTournamentTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts            // Transaction auth options to use throughout this session
}

// NonRootTournamentRaw is an auto generated low-level Go binding around an Ethereum contract.
type NonRootTournamentRaw struct {
	Contract *NonRootTournament // Generic contract binding to access the raw methods on
}

// NonRootTournamentCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type NonRootTournamentCallerRaw struct {
	Contract *NonRootTournamentCaller // Generic read-only contract binding to access the raw methods on
}

// NonRootTournamentTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type NonRootTournamentTransactorRaw struct {
	Contract *NonRootTournamentTransactor // Generic write-only contract binding to access the raw methods on
}

// NewNonRootTournament creates a new instance of NonRootTournament, bound to a specific deployed contract.
func NewNonRootTournament(address common.Address, backend bind.ContractBackend) (*NonRootTournament, error) {
	contract, err := bindNonRootTournament(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &NonRootTournament{NonRootTournamentCaller: NonRootTournamentCaller{contract: contract}, NonRootTournamentTransactor: NonRootTournamentTransactor{contract: contract}, NonRootTournamentFilterer: NonRootTournamentFilterer{contract: contract}}, nil
}

// NewNonRootTournamentCaller creates a new read-only instance of NonRootTournament, bound to a specific deployed contract.
func NewNonRootTournamentCaller(address common.Address, caller bind.ContractCaller) (*NonRootTournamentCaller, error) {
	contract, err := bindNonRootTournament(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &NonRootTournamentCaller{contract: contract}, nil
}

// NewNonRootTournamentTransactor creates a new write-only instance of NonRootTournament, bound to a specific deployed contract.
func NewNonRootTournamentTransactor(address common.Address, transactor bind.ContractTransactor) (*NonRootTournamentTransactor, error) {
	contract, err := bindNonRootTournament(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &NonRootTournamentTransactor{contract: contract}, nil
}

// NewNonRootTournamentFilterer creates a new log filterer instance of NonRootTournament, bound to a specific deployed contract.
func NewNonRootTournamentFilterer(address common.Address, filterer bind.ContractFilterer) (*NonRootTournamentFilterer, error) {
	contract, err := bindNonRootTournament(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &NonRootTournamentFilterer{contract: contract}, nil
}

// bindNonRootTournament binds a generic wrapper to an already deployed contract.
func bindNonRootTournament(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := NonRootTournamentMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_NonRootTournament *NonRootTournamentRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _NonRootTournament.Contract.NonRootTournamentCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_NonRootTournament *NonRootTournamentRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _NonRootTournament.Contract.NonRootTournamentTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_NonRootTournament *NonRootTournamentRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _NonRootTournament.Contract.NonRootTournamentTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_NonRootTournament *NonRootTournamentCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _NonRootTournament.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_NonRootTournament *NonRootTournamentTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _NonRootTournament.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_NonRootTournament *NonRootTournamentTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _NonRootTournament.Contract.contract.Transact(opts, method, params...)
}

// CanBeEliminated is a free data retrieval call binding the contract method 0x95dd0e94.
//
// Solidity: function canBeEliminated() view returns(bool)
func (_NonRootTournament *NonRootTournamentCaller) CanBeEliminated(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _NonRootTournament.contract.Call(opts, &out, "canBeEliminated")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// CanBeEliminated is a free data retrieval call binding the contract method 0x95dd0e94.
//
// Solidity: function canBeEliminated() view returns(bool)
func (_NonRootTournament *NonRootTournamentSession) CanBeEliminated() (bool, error) {
	return _NonRootTournament.Contract.CanBeEliminated(&_NonRootTournament.CallOpts)
}

// CanBeEliminated is a free data retrieval call binding the contract method 0x95dd0e94.
//
// Solidity: function canBeEliminated() view returns(bool)
func (_NonRootTournament *NonRootTournamentCallerSession) CanBeEliminated() (bool, error) {
	return _NonRootTournament.Contract.CanBeEliminated(&_NonRootTournament.CallOpts)
}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_NonRootTournament *NonRootTournamentCaller) CanWinMatchByTimeout(opts *bind.CallOpts, _matchId MatchId) (bool, error) {
	var out []interface{}
	err := _NonRootTournament.contract.Call(opts, &out, "canWinMatchByTimeout", _matchId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_NonRootTournament *NonRootTournamentSession) CanWinMatchByTimeout(_matchId MatchId) (bool, error) {
	return _NonRootTournament.Contract.CanWinMatchByTimeout(&_NonRootTournament.CallOpts, _matchId)
}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_NonRootTournament *NonRootTournamentCallerSession) CanWinMatchByTimeout(_matchId MatchId) (bool, error) {
	return _NonRootTournament.Contract.CanWinMatchByTimeout(&_NonRootTournament.CallOpts, _matchId)
}

// GetCommitment is a free data retrieval call binding the contract method 0x7795820c.
//
// Solidity: function getCommitment(bytes32 _commitmentRoot) view returns((uint64,uint64), bytes32)
func (_NonRootTournament *NonRootTournamentCaller) GetCommitment(opts *bind.CallOpts, _commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	var out []interface{}
	err := _NonRootTournament.contract.Call(opts, &out, "getCommitment", _commitmentRoot)

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
func (_NonRootTournament *NonRootTournamentSession) GetCommitment(_commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	return _NonRootTournament.Contract.GetCommitment(&_NonRootTournament.CallOpts, _commitmentRoot)
}

// GetCommitment is a free data retrieval call binding the contract method 0x7795820c.
//
// Solidity: function getCommitment(bytes32 _commitmentRoot) view returns((uint64,uint64), bytes32)
func (_NonRootTournament *NonRootTournamentCallerSession) GetCommitment(_commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	return _NonRootTournament.Contract.GetCommitment(&_NonRootTournament.CallOpts, _commitmentRoot)
}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_NonRootTournament *NonRootTournamentCaller) GetMatch(opts *bind.CallOpts, _matchIdHash [32]byte) (MatchState, error) {
	var out []interface{}
	err := _NonRootTournament.contract.Call(opts, &out, "getMatch", _matchIdHash)

	if err != nil {
		return *new(MatchState), err
	}

	out0 := *abi.ConvertType(out[0], new(MatchState)).(*MatchState)

	return out0, err

}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_NonRootTournament *NonRootTournamentSession) GetMatch(_matchIdHash [32]byte) (MatchState, error) {
	return _NonRootTournament.Contract.GetMatch(&_NonRootTournament.CallOpts, _matchIdHash)
}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_NonRootTournament *NonRootTournamentCallerSession) GetMatch(_matchIdHash [32]byte) (MatchState, error) {
	return _NonRootTournament.Contract.GetMatch(&_NonRootTournament.CallOpts, _matchIdHash)
}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_NonRootTournament *NonRootTournamentCaller) GetMatchCycle(opts *bind.CallOpts, _matchIdHash [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _NonRootTournament.contract.Call(opts, &out, "getMatchCycle", _matchIdHash)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_NonRootTournament *NonRootTournamentSession) GetMatchCycle(_matchIdHash [32]byte) (*big.Int, error) {
	return _NonRootTournament.Contract.GetMatchCycle(&_NonRootTournament.CallOpts, _matchIdHash)
}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_NonRootTournament *NonRootTournamentCallerSession) GetMatchCycle(_matchIdHash [32]byte) (*big.Int, error) {
	return _NonRootTournament.Contract.GetMatchCycle(&_NonRootTournament.CallOpts, _matchIdHash)
}

// InnerTournamentWinner is a free data retrieval call binding the contract method 0x5145236f.
//
// Solidity: function innerTournamentWinner() view returns(bool, bytes32, bytes32, (uint64,uint64))
func (_NonRootTournament *NonRootTournamentCaller) InnerTournamentWinner(opts *bind.CallOpts) (bool, [32]byte, [32]byte, ClockState, error) {
	var out []interface{}
	err := _NonRootTournament.contract.Call(opts, &out, "innerTournamentWinner")

	if err != nil {
		return *new(bool), *new([32]byte), *new([32]byte), *new(ClockState), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)
	out1 := *abi.ConvertType(out[1], new([32]byte)).(*[32]byte)
	out2 := *abi.ConvertType(out[2], new([32]byte)).(*[32]byte)
	out3 := *abi.ConvertType(out[3], new(ClockState)).(*ClockState)

	return out0, out1, out2, out3, err

}

// InnerTournamentWinner is a free data retrieval call binding the contract method 0x5145236f.
//
// Solidity: function innerTournamentWinner() view returns(bool, bytes32, bytes32, (uint64,uint64))
func (_NonRootTournament *NonRootTournamentSession) InnerTournamentWinner() (bool, [32]byte, [32]byte, ClockState, error) {
	return _NonRootTournament.Contract.InnerTournamentWinner(&_NonRootTournament.CallOpts)
}

// InnerTournamentWinner is a free data retrieval call binding the contract method 0x5145236f.
//
// Solidity: function innerTournamentWinner() view returns(bool, bytes32, bytes32, (uint64,uint64))
func (_NonRootTournament *NonRootTournamentCallerSession) InnerTournamentWinner() (bool, [32]byte, [32]byte, ClockState, error) {
	return _NonRootTournament.Contract.InnerTournamentWinner(&_NonRootTournament.CallOpts)
}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_NonRootTournament *NonRootTournamentCaller) IsClosed(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _NonRootTournament.contract.Call(opts, &out, "isClosed")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_NonRootTournament *NonRootTournamentSession) IsClosed() (bool, error) {
	return _NonRootTournament.Contract.IsClosed(&_NonRootTournament.CallOpts)
}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_NonRootTournament *NonRootTournamentCallerSession) IsClosed() (bool, error) {
	return _NonRootTournament.Contract.IsClosed(&_NonRootTournament.CallOpts)
}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_NonRootTournament *NonRootTournamentCaller) IsFinished(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _NonRootTournament.contract.Call(opts, &out, "isFinished")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_NonRootTournament *NonRootTournamentSession) IsFinished() (bool, error) {
	return _NonRootTournament.Contract.IsFinished(&_NonRootTournament.CallOpts)
}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_NonRootTournament *NonRootTournamentCallerSession) IsFinished() (bool, error) {
	return _NonRootTournament.Contract.IsFinished(&_NonRootTournament.CallOpts)
}

// TimeFinished is a free data retrieval call binding the contract method 0x39cdfaf2.
//
// Solidity: function timeFinished() view returns(bool, uint64)
func (_NonRootTournament *NonRootTournamentCaller) TimeFinished(opts *bind.CallOpts) (bool, uint64, error) {
	var out []interface{}
	err := _NonRootTournament.contract.Call(opts, &out, "timeFinished")

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
func (_NonRootTournament *NonRootTournamentSession) TimeFinished() (bool, uint64, error) {
	return _NonRootTournament.Contract.TimeFinished(&_NonRootTournament.CallOpts)
}

// TimeFinished is a free data retrieval call binding the contract method 0x39cdfaf2.
//
// Solidity: function timeFinished() view returns(bool, uint64)
func (_NonRootTournament *NonRootTournamentCallerSession) TimeFinished() (bool, uint64, error) {
	return _NonRootTournament.Contract.TimeFinished(&_NonRootTournament.CallOpts)
}

// TournamentLevelConstants is a free data retrieval call binding the contract method 0xa1af906b.
//
// Solidity: function tournamentLevelConstants() view returns(uint64 _maxLevel, uint64 _level, uint64 _log2step, uint64 _height)
func (_NonRootTournament *NonRootTournamentCaller) TournamentLevelConstants(opts *bind.CallOpts) (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	var out []interface{}
	err := _NonRootTournament.contract.Call(opts, &out, "tournamentLevelConstants")

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
func (_NonRootTournament *NonRootTournamentSession) TournamentLevelConstants() (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	return _NonRootTournament.Contract.TournamentLevelConstants(&_NonRootTournament.CallOpts)
}

// TournamentLevelConstants is a free data retrieval call binding the contract method 0xa1af906b.
//
// Solidity: function tournamentLevelConstants() view returns(uint64 _maxLevel, uint64 _level, uint64 _log2step, uint64 _height)
func (_NonRootTournament *NonRootTournamentCallerSession) TournamentLevelConstants() (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	return _NonRootTournament.Contract.TournamentLevelConstants(&_NonRootTournament.CallOpts)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_NonRootTournament *NonRootTournamentTransactor) AdvanceMatch(opts *bind.TransactOpts, _matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _NonRootTournament.contract.Transact(opts, "advanceMatch", _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_NonRootTournament *NonRootTournamentSession) AdvanceMatch(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _NonRootTournament.Contract.AdvanceMatch(&_NonRootTournament.TransactOpts, _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_NonRootTournament *NonRootTournamentTransactorSession) AdvanceMatch(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _NonRootTournament.Contract.AdvanceMatch(&_NonRootTournament.TransactOpts, _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_NonRootTournament *NonRootTournamentTransactor) EliminateMatchByTimeout(opts *bind.TransactOpts, _matchId MatchId) (*types.Transaction, error) {
	return _NonRootTournament.contract.Transact(opts, "eliminateMatchByTimeout", _matchId)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_NonRootTournament *NonRootTournamentSession) EliminateMatchByTimeout(_matchId MatchId) (*types.Transaction, error) {
	return _NonRootTournament.Contract.EliminateMatchByTimeout(&_NonRootTournament.TransactOpts, _matchId)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_NonRootTournament *NonRootTournamentTransactorSession) EliminateMatchByTimeout(_matchId MatchId) (*types.Transaction, error) {
	return _NonRootTournament.Contract.EliminateMatchByTimeout(&_NonRootTournament.TransactOpts, _matchId)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonRootTournament *NonRootTournamentTransactor) JoinTournament(opts *bind.TransactOpts, _finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonRootTournament.contract.Transact(opts, "joinTournament", _finalState, _proof, _leftNode, _rightNode)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonRootTournament *NonRootTournamentSession) JoinTournament(_finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonRootTournament.Contract.JoinTournament(&_NonRootTournament.TransactOpts, _finalState, _proof, _leftNode, _rightNode)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonRootTournament *NonRootTournamentTransactorSession) JoinTournament(_finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonRootTournament.Contract.JoinTournament(&_NonRootTournament.TransactOpts, _finalState, _proof, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonRootTournament *NonRootTournamentTransactor) WinMatchByTimeout(opts *bind.TransactOpts, _matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonRootTournament.contract.Transact(opts, "winMatchByTimeout", _matchId, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonRootTournament *NonRootTournamentSession) WinMatchByTimeout(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonRootTournament.Contract.WinMatchByTimeout(&_NonRootTournament.TransactOpts, _matchId, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonRootTournament *NonRootTournamentTransactorSession) WinMatchByTimeout(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonRootTournament.Contract.WinMatchByTimeout(&_NonRootTournament.TransactOpts, _matchId, _leftNode, _rightNode)
}

// NonRootTournamentCommitmentJoinedIterator is returned from FilterCommitmentJoined and is used to iterate over the raw logs and unpacked data for CommitmentJoined events raised by the NonRootTournament contract.
type NonRootTournamentCommitmentJoinedIterator struct {
	Event *NonRootTournamentCommitmentJoined // Event containing the contract specifics and raw log

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
func (it *NonRootTournamentCommitmentJoinedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(NonRootTournamentCommitmentJoined)
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
		it.Event = new(NonRootTournamentCommitmentJoined)
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
func (it *NonRootTournamentCommitmentJoinedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *NonRootTournamentCommitmentJoinedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// NonRootTournamentCommitmentJoined represents a CommitmentJoined event raised by the NonRootTournament contract.
type NonRootTournamentCommitmentJoined struct {
	Root [32]byte
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterCommitmentJoined is a free log retrieval operation binding the contract event 0xe53537f202911d376d6e285835b2a2016e83e99fbe84a059d445cc2be4807262.
//
// Solidity: event commitmentJoined(bytes32 root)
func (_NonRootTournament *NonRootTournamentFilterer) FilterCommitmentJoined(opts *bind.FilterOpts) (*NonRootTournamentCommitmentJoinedIterator, error) {

	logs, sub, err := _NonRootTournament.contract.FilterLogs(opts, "commitmentJoined")
	if err != nil {
		return nil, err
	}
	return &NonRootTournamentCommitmentJoinedIterator{contract: _NonRootTournament.contract, event: "commitmentJoined", logs: logs, sub: sub}, nil
}

// WatchCommitmentJoined is a free log subscription operation binding the contract event 0xe53537f202911d376d6e285835b2a2016e83e99fbe84a059d445cc2be4807262.
//
// Solidity: event commitmentJoined(bytes32 root)
func (_NonRootTournament *NonRootTournamentFilterer) WatchCommitmentJoined(opts *bind.WatchOpts, sink chan<- *NonRootTournamentCommitmentJoined) (event.Subscription, error) {

	logs, sub, err := _NonRootTournament.contract.WatchLogs(opts, "commitmentJoined")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(NonRootTournamentCommitmentJoined)
				if err := _NonRootTournament.contract.UnpackLog(event, "commitmentJoined", log); err != nil {
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
func (_NonRootTournament *NonRootTournamentFilterer) ParseCommitmentJoined(log types.Log) (*NonRootTournamentCommitmentJoined, error) {
	event := new(NonRootTournamentCommitmentJoined)
	if err := _NonRootTournament.contract.UnpackLog(event, "commitmentJoined", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// NonRootTournamentMatchAdvancedIterator is returned from FilterMatchAdvanced and is used to iterate over the raw logs and unpacked data for MatchAdvanced events raised by the NonRootTournament contract.
type NonRootTournamentMatchAdvancedIterator struct {
	Event *NonRootTournamentMatchAdvanced // Event containing the contract specifics and raw log

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
func (it *NonRootTournamentMatchAdvancedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(NonRootTournamentMatchAdvanced)
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
		it.Event = new(NonRootTournamentMatchAdvanced)
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
func (it *NonRootTournamentMatchAdvancedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *NonRootTournamentMatchAdvancedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// NonRootTournamentMatchAdvanced represents a MatchAdvanced event raised by the NonRootTournament contract.
type NonRootTournamentMatchAdvanced struct {
	Arg0   [32]byte
	Parent [32]byte
	Left   [32]byte
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterMatchAdvanced is a free log retrieval operation binding the contract event 0x29ff393c59c37f91e930fad4d88447efc58cf5d7c048499e1f20edb369941378.
//
// Solidity: event matchAdvanced(bytes32 indexed arg0, bytes32 parent, bytes32 left)
func (_NonRootTournament *NonRootTournamentFilterer) FilterMatchAdvanced(opts *bind.FilterOpts, arg0 [][32]byte) (*NonRootTournamentMatchAdvancedIterator, error) {

	var arg0Rule []interface{}
	for _, arg0Item := range arg0 {
		arg0Rule = append(arg0Rule, arg0Item)
	}

	logs, sub, err := _NonRootTournament.contract.FilterLogs(opts, "matchAdvanced", arg0Rule)
	if err != nil {
		return nil, err
	}
	return &NonRootTournamentMatchAdvancedIterator{contract: _NonRootTournament.contract, event: "matchAdvanced", logs: logs, sub: sub}, nil
}

// WatchMatchAdvanced is a free log subscription operation binding the contract event 0x29ff393c59c37f91e930fad4d88447efc58cf5d7c048499e1f20edb369941378.
//
// Solidity: event matchAdvanced(bytes32 indexed arg0, bytes32 parent, bytes32 left)
func (_NonRootTournament *NonRootTournamentFilterer) WatchMatchAdvanced(opts *bind.WatchOpts, sink chan<- *NonRootTournamentMatchAdvanced, arg0 [][32]byte) (event.Subscription, error) {

	var arg0Rule []interface{}
	for _, arg0Item := range arg0 {
		arg0Rule = append(arg0Rule, arg0Item)
	}

	logs, sub, err := _NonRootTournament.contract.WatchLogs(opts, "matchAdvanced", arg0Rule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(NonRootTournamentMatchAdvanced)
				if err := _NonRootTournament.contract.UnpackLog(event, "matchAdvanced", log); err != nil {
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
func (_NonRootTournament *NonRootTournamentFilterer) ParseMatchAdvanced(log types.Log) (*NonRootTournamentMatchAdvanced, error) {
	event := new(NonRootTournamentMatchAdvanced)
	if err := _NonRootTournament.contract.UnpackLog(event, "matchAdvanced", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// NonRootTournamentMatchCreatedIterator is returned from FilterMatchCreated and is used to iterate over the raw logs and unpacked data for MatchCreated events raised by the NonRootTournament contract.
type NonRootTournamentMatchCreatedIterator struct {
	Event *NonRootTournamentMatchCreated // Event containing the contract specifics and raw log

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
func (it *NonRootTournamentMatchCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(NonRootTournamentMatchCreated)
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
		it.Event = new(NonRootTournamentMatchCreated)
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
func (it *NonRootTournamentMatchCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *NonRootTournamentMatchCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// NonRootTournamentMatchCreated represents a MatchCreated event raised by the NonRootTournament contract.
type NonRootTournamentMatchCreated struct {
	One       [32]byte
	Two       [32]byte
	LeftOfTwo [32]byte
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterMatchCreated is a free log retrieval operation binding the contract event 0x32911001007d8c9879b608566be8acc2184592f0a43706f804f285455bb0f52e.
//
// Solidity: event matchCreated(bytes32 indexed one, bytes32 indexed two, bytes32 leftOfTwo)
func (_NonRootTournament *NonRootTournamentFilterer) FilterMatchCreated(opts *bind.FilterOpts, one [][32]byte, two [][32]byte) (*NonRootTournamentMatchCreatedIterator, error) {

	var oneRule []interface{}
	for _, oneItem := range one {
		oneRule = append(oneRule, oneItem)
	}
	var twoRule []interface{}
	for _, twoItem := range two {
		twoRule = append(twoRule, twoItem)
	}

	logs, sub, err := _NonRootTournament.contract.FilterLogs(opts, "matchCreated", oneRule, twoRule)
	if err != nil {
		return nil, err
	}
	return &NonRootTournamentMatchCreatedIterator{contract: _NonRootTournament.contract, event: "matchCreated", logs: logs, sub: sub}, nil
}

// WatchMatchCreated is a free log subscription operation binding the contract event 0x32911001007d8c9879b608566be8acc2184592f0a43706f804f285455bb0f52e.
//
// Solidity: event matchCreated(bytes32 indexed one, bytes32 indexed two, bytes32 leftOfTwo)
func (_NonRootTournament *NonRootTournamentFilterer) WatchMatchCreated(opts *bind.WatchOpts, sink chan<- *NonRootTournamentMatchCreated, one [][32]byte, two [][32]byte) (event.Subscription, error) {

	var oneRule []interface{}
	for _, oneItem := range one {
		oneRule = append(oneRule, oneItem)
	}
	var twoRule []interface{}
	for _, twoItem := range two {
		twoRule = append(twoRule, twoItem)
	}

	logs, sub, err := _NonRootTournament.contract.WatchLogs(opts, "matchCreated", oneRule, twoRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(NonRootTournamentMatchCreated)
				if err := _NonRootTournament.contract.UnpackLog(event, "matchCreated", log); err != nil {
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
func (_NonRootTournament *NonRootTournamentFilterer) ParseMatchCreated(log types.Log) (*NonRootTournamentMatchCreated, error) {
	event := new(NonRootTournamentMatchCreated)
	if err := _NonRootTournament.contract.UnpackLog(event, "matchCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// NonRootTournamentMatchDeletedIterator is returned from FilterMatchDeleted and is used to iterate over the raw logs and unpacked data for MatchDeleted events raised by the NonRootTournament contract.
type NonRootTournamentMatchDeletedIterator struct {
	Event *NonRootTournamentMatchDeleted // Event containing the contract specifics and raw log

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
func (it *NonRootTournamentMatchDeletedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(NonRootTournamentMatchDeleted)
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
		it.Event = new(NonRootTournamentMatchDeleted)
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
func (it *NonRootTournamentMatchDeletedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *NonRootTournamentMatchDeletedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// NonRootTournamentMatchDeleted represents a MatchDeleted event raised by the NonRootTournament contract.
type NonRootTournamentMatchDeleted struct {
	Arg0 [32]byte
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterMatchDeleted is a free log retrieval operation binding the contract event 0x0afce37c521a4613a2db0c4983987a3c4af722e33d3412963fccbc0eb0df0d28.
//
// Solidity: event matchDeleted(bytes32 arg0)
func (_NonRootTournament *NonRootTournamentFilterer) FilterMatchDeleted(opts *bind.FilterOpts) (*NonRootTournamentMatchDeletedIterator, error) {

	logs, sub, err := _NonRootTournament.contract.FilterLogs(opts, "matchDeleted")
	if err != nil {
		return nil, err
	}
	return &NonRootTournamentMatchDeletedIterator{contract: _NonRootTournament.contract, event: "matchDeleted", logs: logs, sub: sub}, nil
}

// WatchMatchDeleted is a free log subscription operation binding the contract event 0x0afce37c521a4613a2db0c4983987a3c4af722e33d3412963fccbc0eb0df0d28.
//
// Solidity: event matchDeleted(bytes32 arg0)
func (_NonRootTournament *NonRootTournamentFilterer) WatchMatchDeleted(opts *bind.WatchOpts, sink chan<- *NonRootTournamentMatchDeleted) (event.Subscription, error) {

	logs, sub, err := _NonRootTournament.contract.WatchLogs(opts, "matchDeleted")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(NonRootTournamentMatchDeleted)
				if err := _NonRootTournament.contract.UnpackLog(event, "matchDeleted", log); err != nil {
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
func (_NonRootTournament *NonRootTournamentFilterer) ParseMatchDeleted(log types.Log) (*NonRootTournamentMatchDeleted, error) {
	event := new(NonRootTournamentMatchDeleted)
	if err := _NonRootTournament.contract.UnpackLog(event, "matchDeleted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
