// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package nonleaftournament

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

// NonLeafTournamentMetaData contains all meta data concerning the NonLeafTournament contract.
var NonLeafTournamentMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"advanceMatch\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_newLeftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_newRightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"canWinMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"eliminateInnerTournament\",\"inputs\":[{\"name\":\"_childTournament\",\"type\":\"address\",\"internalType\":\"contractNonRootTournament\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"eliminateMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getCommitment\",\"inputs\":[{\"name\":\"_commitmentRoot\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structClock.State\",\"components\":[{\"name\":\"allowance\",\"type\":\"uint64\",\"internalType\":\"Time.Duration\"},{\"name\":\"startInstant\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"}]},{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMatch\",\"inputs\":[{\"name\":\"_matchIdHash\",\"type\":\"bytes32\",\"internalType\":\"Match.IdHash\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structMatch.State\",\"components\":[{\"name\":\"otherParent\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"runningLeafPosition\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentHeight\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"log2step\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"height\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMatchCycle\",\"inputs\":[{\"name\":\"_matchIdHash\",\"type\":\"bytes32\",\"internalType\":\"Match.IdHash\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isClosed\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isFinished\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"joinTournament\",\"inputs\":[{\"name\":\"_finalState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"_proof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"sealInnerMatchAndCreateInnerTournament\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"_leftLeaf\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightLeaf\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_agreeHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"_agreeHashProof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"timeFinished\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"tournamentLevelConstants\",\"inputs\":[],\"outputs\":[{\"name\":\"_maxLevel\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_level\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_log2step\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_height\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"winInnerTournament\",\"inputs\":[{\"name\":\"_childTournament\",\"type\":\"address\",\"internalType\":\"contractNonRootTournament\"},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"winMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"commitmentJoined\",\"inputs\":[{\"name\":\"root\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchAdvanced\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Match.IdHash\"},{\"name\":\"parent\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"},{\"name\":\"left\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchCreated\",\"inputs\":[{\"name\":\"one\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"two\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"leftOfTwo\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchDeleted\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Match.IdHash\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"newInnerTournament\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Match.IdHash\"},{\"name\":\"\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractNonRootTournament\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"ChildTournamentCannotBeEliminated\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ChildTournamentMustBeEliminated\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ChildTournamentNotFinished\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"CommitmentMismatch\",\"inputs\":[{\"name\":\"received\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"expected\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"type\":\"error\",\"name\":\"EliminateByTimeout\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"IncorrectAgreeState\",\"inputs\":[{\"name\":\"initialState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"agreeState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}]},{\"type\":\"error\",\"name\":\"InvalidContestedFinalState\",\"inputs\":[{\"name\":\"contestedFinalStateOne\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"contestedFinalStateTwo\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"finalState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}]},{\"type\":\"error\",\"name\":\"LengthMismatch\",\"inputs\":[{\"name\":\"treeHeight\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"siblingsLength\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]},{\"type\":\"error\",\"name\":\"TournamentIsClosed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TournamentIsFinished\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WinByTimeout\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WrongChildren\",\"inputs\":[{\"name\":\"commitment\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"parent\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"left\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"right\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"type\":\"error\",\"name\":\"WrongTournamentWinner\",\"inputs\":[{\"name\":\"commitmentRoot\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"winner\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}]",
}

// NonLeafTournamentABI is the input ABI used to generate the binding from.
// Deprecated: Use NonLeafTournamentMetaData.ABI instead.
var NonLeafTournamentABI = NonLeafTournamentMetaData.ABI

// NonLeafTournament is an auto generated Go binding around an Ethereum contract.
type NonLeafTournament struct {
	NonLeafTournamentCaller     // Read-only binding to the contract
	NonLeafTournamentTransactor // Write-only binding to the contract
	NonLeafTournamentFilterer   // Log filterer for contract events
}

// NonLeafTournamentCaller is an auto generated read-only Go binding around an Ethereum contract.
type NonLeafTournamentCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// NonLeafTournamentTransactor is an auto generated write-only Go binding around an Ethereum contract.
type NonLeafTournamentTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// NonLeafTournamentFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type NonLeafTournamentFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// NonLeafTournamentSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type NonLeafTournamentSession struct {
	Contract     *NonLeafTournament // Generic contract binding to set the session for
	CallOpts     bind.CallOpts      // Call options to use throughout this session
	TransactOpts bind.TransactOpts  // Transaction auth options to use throughout this session
}

// NonLeafTournamentCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type NonLeafTournamentCallerSession struct {
	Contract *NonLeafTournamentCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts            // Call options to use throughout this session
}

// NonLeafTournamentTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type NonLeafTournamentTransactorSession struct {
	Contract     *NonLeafTournamentTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts            // Transaction auth options to use throughout this session
}

// NonLeafTournamentRaw is an auto generated low-level Go binding around an Ethereum contract.
type NonLeafTournamentRaw struct {
	Contract *NonLeafTournament // Generic contract binding to access the raw methods on
}

// NonLeafTournamentCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type NonLeafTournamentCallerRaw struct {
	Contract *NonLeafTournamentCaller // Generic read-only contract binding to access the raw methods on
}

// NonLeafTournamentTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type NonLeafTournamentTransactorRaw struct {
	Contract *NonLeafTournamentTransactor // Generic write-only contract binding to access the raw methods on
}

// NewNonLeafTournament creates a new instance of NonLeafTournament, bound to a specific deployed contract.
func NewNonLeafTournament(address common.Address, backend bind.ContractBackend) (*NonLeafTournament, error) {
	contract, err := bindNonLeafTournament(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &NonLeafTournament{NonLeafTournamentCaller: NonLeafTournamentCaller{contract: contract}, NonLeafTournamentTransactor: NonLeafTournamentTransactor{contract: contract}, NonLeafTournamentFilterer: NonLeafTournamentFilterer{contract: contract}}, nil
}

// NewNonLeafTournamentCaller creates a new read-only instance of NonLeafTournament, bound to a specific deployed contract.
func NewNonLeafTournamentCaller(address common.Address, caller bind.ContractCaller) (*NonLeafTournamentCaller, error) {
	contract, err := bindNonLeafTournament(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &NonLeafTournamentCaller{contract: contract}, nil
}

// NewNonLeafTournamentTransactor creates a new write-only instance of NonLeafTournament, bound to a specific deployed contract.
func NewNonLeafTournamentTransactor(address common.Address, transactor bind.ContractTransactor) (*NonLeafTournamentTransactor, error) {
	contract, err := bindNonLeafTournament(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &NonLeafTournamentTransactor{contract: contract}, nil
}

// NewNonLeafTournamentFilterer creates a new log filterer instance of NonLeafTournament, bound to a specific deployed contract.
func NewNonLeafTournamentFilterer(address common.Address, filterer bind.ContractFilterer) (*NonLeafTournamentFilterer, error) {
	contract, err := bindNonLeafTournament(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &NonLeafTournamentFilterer{contract: contract}, nil
}

// bindNonLeafTournament binds a generic wrapper to an already deployed contract.
func bindNonLeafTournament(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := NonLeafTournamentMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_NonLeafTournament *NonLeafTournamentRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _NonLeafTournament.Contract.NonLeafTournamentCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_NonLeafTournament *NonLeafTournamentRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.NonLeafTournamentTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_NonLeafTournament *NonLeafTournamentRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.NonLeafTournamentTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_NonLeafTournament *NonLeafTournamentCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _NonLeafTournament.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_NonLeafTournament *NonLeafTournamentTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_NonLeafTournament *NonLeafTournamentTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.contract.Transact(opts, method, params...)
}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_NonLeafTournament *NonLeafTournamentCaller) CanWinMatchByTimeout(opts *bind.CallOpts, _matchId MatchId) (bool, error) {
	var out []interface{}
	err := _NonLeafTournament.contract.Call(opts, &out, "canWinMatchByTimeout", _matchId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_NonLeafTournament *NonLeafTournamentSession) CanWinMatchByTimeout(_matchId MatchId) (bool, error) {
	return _NonLeafTournament.Contract.CanWinMatchByTimeout(&_NonLeafTournament.CallOpts, _matchId)
}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_NonLeafTournament *NonLeafTournamentCallerSession) CanWinMatchByTimeout(_matchId MatchId) (bool, error) {
	return _NonLeafTournament.Contract.CanWinMatchByTimeout(&_NonLeafTournament.CallOpts, _matchId)
}

// GetCommitment is a free data retrieval call binding the contract method 0x7795820c.
//
// Solidity: function getCommitment(bytes32 _commitmentRoot) view returns((uint64,uint64), bytes32)
func (_NonLeafTournament *NonLeafTournamentCaller) GetCommitment(opts *bind.CallOpts, _commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	var out []interface{}
	err := _NonLeafTournament.contract.Call(opts, &out, "getCommitment", _commitmentRoot)

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
func (_NonLeafTournament *NonLeafTournamentSession) GetCommitment(_commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	return _NonLeafTournament.Contract.GetCommitment(&_NonLeafTournament.CallOpts, _commitmentRoot)
}

// GetCommitment is a free data retrieval call binding the contract method 0x7795820c.
//
// Solidity: function getCommitment(bytes32 _commitmentRoot) view returns((uint64,uint64), bytes32)
func (_NonLeafTournament *NonLeafTournamentCallerSession) GetCommitment(_commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	return _NonLeafTournament.Contract.GetCommitment(&_NonLeafTournament.CallOpts, _commitmentRoot)
}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_NonLeafTournament *NonLeafTournamentCaller) GetMatch(opts *bind.CallOpts, _matchIdHash [32]byte) (MatchState, error) {
	var out []interface{}
	err := _NonLeafTournament.contract.Call(opts, &out, "getMatch", _matchIdHash)

	if err != nil {
		return *new(MatchState), err
	}

	out0 := *abi.ConvertType(out[0], new(MatchState)).(*MatchState)

	return out0, err

}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_NonLeafTournament *NonLeafTournamentSession) GetMatch(_matchIdHash [32]byte) (MatchState, error) {
	return _NonLeafTournament.Contract.GetMatch(&_NonLeafTournament.CallOpts, _matchIdHash)
}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_NonLeafTournament *NonLeafTournamentCallerSession) GetMatch(_matchIdHash [32]byte) (MatchState, error) {
	return _NonLeafTournament.Contract.GetMatch(&_NonLeafTournament.CallOpts, _matchIdHash)
}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_NonLeafTournament *NonLeafTournamentCaller) GetMatchCycle(opts *bind.CallOpts, _matchIdHash [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _NonLeafTournament.contract.Call(opts, &out, "getMatchCycle", _matchIdHash)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_NonLeafTournament *NonLeafTournamentSession) GetMatchCycle(_matchIdHash [32]byte) (*big.Int, error) {
	return _NonLeafTournament.Contract.GetMatchCycle(&_NonLeafTournament.CallOpts, _matchIdHash)
}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_NonLeafTournament *NonLeafTournamentCallerSession) GetMatchCycle(_matchIdHash [32]byte) (*big.Int, error) {
	return _NonLeafTournament.Contract.GetMatchCycle(&_NonLeafTournament.CallOpts, _matchIdHash)
}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_NonLeafTournament *NonLeafTournamentCaller) IsClosed(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _NonLeafTournament.contract.Call(opts, &out, "isClosed")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_NonLeafTournament *NonLeafTournamentSession) IsClosed() (bool, error) {
	return _NonLeafTournament.Contract.IsClosed(&_NonLeafTournament.CallOpts)
}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_NonLeafTournament *NonLeafTournamentCallerSession) IsClosed() (bool, error) {
	return _NonLeafTournament.Contract.IsClosed(&_NonLeafTournament.CallOpts)
}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_NonLeafTournament *NonLeafTournamentCaller) IsFinished(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _NonLeafTournament.contract.Call(opts, &out, "isFinished")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_NonLeafTournament *NonLeafTournamentSession) IsFinished() (bool, error) {
	return _NonLeafTournament.Contract.IsFinished(&_NonLeafTournament.CallOpts)
}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_NonLeafTournament *NonLeafTournamentCallerSession) IsFinished() (bool, error) {
	return _NonLeafTournament.Contract.IsFinished(&_NonLeafTournament.CallOpts)
}

// TimeFinished is a free data retrieval call binding the contract method 0x39cdfaf2.
//
// Solidity: function timeFinished() view returns(bool, uint64)
func (_NonLeafTournament *NonLeafTournamentCaller) TimeFinished(opts *bind.CallOpts) (bool, uint64, error) {
	var out []interface{}
	err := _NonLeafTournament.contract.Call(opts, &out, "timeFinished")

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
func (_NonLeafTournament *NonLeafTournamentSession) TimeFinished() (bool, uint64, error) {
	return _NonLeafTournament.Contract.TimeFinished(&_NonLeafTournament.CallOpts)
}

// TimeFinished is a free data retrieval call binding the contract method 0x39cdfaf2.
//
// Solidity: function timeFinished() view returns(bool, uint64)
func (_NonLeafTournament *NonLeafTournamentCallerSession) TimeFinished() (bool, uint64, error) {
	return _NonLeafTournament.Contract.TimeFinished(&_NonLeafTournament.CallOpts)
}

// TournamentLevelConstants is a free data retrieval call binding the contract method 0xa1af906b.
//
// Solidity: function tournamentLevelConstants() view returns(uint64 _maxLevel, uint64 _level, uint64 _log2step, uint64 _height)
func (_NonLeafTournament *NonLeafTournamentCaller) TournamentLevelConstants(opts *bind.CallOpts) (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	var out []interface{}
	err := _NonLeafTournament.contract.Call(opts, &out, "tournamentLevelConstants")

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
func (_NonLeafTournament *NonLeafTournamentSession) TournamentLevelConstants() (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	return _NonLeafTournament.Contract.TournamentLevelConstants(&_NonLeafTournament.CallOpts)
}

// TournamentLevelConstants is a free data retrieval call binding the contract method 0xa1af906b.
//
// Solidity: function tournamentLevelConstants() view returns(uint64 _maxLevel, uint64 _level, uint64 _log2step, uint64 _height)
func (_NonLeafTournament *NonLeafTournamentCallerSession) TournamentLevelConstants() (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	return _NonLeafTournament.Contract.TournamentLevelConstants(&_NonLeafTournament.CallOpts)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_NonLeafTournament *NonLeafTournamentTransactor) AdvanceMatch(opts *bind.TransactOpts, _matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.contract.Transact(opts, "advanceMatch", _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_NonLeafTournament *NonLeafTournamentSession) AdvanceMatch(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.AdvanceMatch(&_NonLeafTournament.TransactOpts, _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_NonLeafTournament *NonLeafTournamentTransactorSession) AdvanceMatch(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.AdvanceMatch(&_NonLeafTournament.TransactOpts, _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// EliminateInnerTournament is a paid mutator transaction binding the contract method 0x26860b49.
//
// Solidity: function eliminateInnerTournament(address _childTournament) returns()
func (_NonLeafTournament *NonLeafTournamentTransactor) EliminateInnerTournament(opts *bind.TransactOpts, _childTournament common.Address) (*types.Transaction, error) {
	return _NonLeafTournament.contract.Transact(opts, "eliminateInnerTournament", _childTournament)
}

// EliminateInnerTournament is a paid mutator transaction binding the contract method 0x26860b49.
//
// Solidity: function eliminateInnerTournament(address _childTournament) returns()
func (_NonLeafTournament *NonLeafTournamentSession) EliminateInnerTournament(_childTournament common.Address) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.EliminateInnerTournament(&_NonLeafTournament.TransactOpts, _childTournament)
}

// EliminateInnerTournament is a paid mutator transaction binding the contract method 0x26860b49.
//
// Solidity: function eliminateInnerTournament(address _childTournament) returns()
func (_NonLeafTournament *NonLeafTournamentTransactorSession) EliminateInnerTournament(_childTournament common.Address) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.EliminateInnerTournament(&_NonLeafTournament.TransactOpts, _childTournament)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_NonLeafTournament *NonLeafTournamentTransactor) EliminateMatchByTimeout(opts *bind.TransactOpts, _matchId MatchId) (*types.Transaction, error) {
	return _NonLeafTournament.contract.Transact(opts, "eliminateMatchByTimeout", _matchId)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_NonLeafTournament *NonLeafTournamentSession) EliminateMatchByTimeout(_matchId MatchId) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.EliminateMatchByTimeout(&_NonLeafTournament.TransactOpts, _matchId)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_NonLeafTournament *NonLeafTournamentTransactorSession) EliminateMatchByTimeout(_matchId MatchId) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.EliminateMatchByTimeout(&_NonLeafTournament.TransactOpts, _matchId)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonLeafTournament *NonLeafTournamentTransactor) JoinTournament(opts *bind.TransactOpts, _finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.contract.Transact(opts, "joinTournament", _finalState, _proof, _leftNode, _rightNode)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonLeafTournament *NonLeafTournamentSession) JoinTournament(_finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.JoinTournament(&_NonLeafTournament.TransactOpts, _finalState, _proof, _leftNode, _rightNode)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonLeafTournament *NonLeafTournamentTransactorSession) JoinTournament(_finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.JoinTournament(&_NonLeafTournament.TransactOpts, _finalState, _proof, _leftNode, _rightNode)
}

// SealInnerMatchAndCreateInnerTournament is a paid mutator transaction binding the contract method 0x3f36e2fe.
//
// Solidity: function sealInnerMatchAndCreateInnerTournament((bytes32,bytes32) _matchId, bytes32 _leftLeaf, bytes32 _rightLeaf, bytes32 _agreeHash, bytes32[] _agreeHashProof) returns()
func (_NonLeafTournament *NonLeafTournamentTransactor) SealInnerMatchAndCreateInnerTournament(opts *bind.TransactOpts, _matchId MatchId, _leftLeaf [32]byte, _rightLeaf [32]byte, _agreeHash [32]byte, _agreeHashProof [][32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.contract.Transact(opts, "sealInnerMatchAndCreateInnerTournament", _matchId, _leftLeaf, _rightLeaf, _agreeHash, _agreeHashProof)
}

// SealInnerMatchAndCreateInnerTournament is a paid mutator transaction binding the contract method 0x3f36e2fe.
//
// Solidity: function sealInnerMatchAndCreateInnerTournament((bytes32,bytes32) _matchId, bytes32 _leftLeaf, bytes32 _rightLeaf, bytes32 _agreeHash, bytes32[] _agreeHashProof) returns()
func (_NonLeafTournament *NonLeafTournamentSession) SealInnerMatchAndCreateInnerTournament(_matchId MatchId, _leftLeaf [32]byte, _rightLeaf [32]byte, _agreeHash [32]byte, _agreeHashProof [][32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.SealInnerMatchAndCreateInnerTournament(&_NonLeafTournament.TransactOpts, _matchId, _leftLeaf, _rightLeaf, _agreeHash, _agreeHashProof)
}

// SealInnerMatchAndCreateInnerTournament is a paid mutator transaction binding the contract method 0x3f36e2fe.
//
// Solidity: function sealInnerMatchAndCreateInnerTournament((bytes32,bytes32) _matchId, bytes32 _leftLeaf, bytes32 _rightLeaf, bytes32 _agreeHash, bytes32[] _agreeHashProof) returns()
func (_NonLeafTournament *NonLeafTournamentTransactorSession) SealInnerMatchAndCreateInnerTournament(_matchId MatchId, _leftLeaf [32]byte, _rightLeaf [32]byte, _agreeHash [32]byte, _agreeHashProof [][32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.SealInnerMatchAndCreateInnerTournament(&_NonLeafTournament.TransactOpts, _matchId, _leftLeaf, _rightLeaf, _agreeHash, _agreeHashProof)
}

// WinInnerTournament is a paid mutator transaction binding the contract method 0x4a95153e.
//
// Solidity: function winInnerTournament(address _childTournament, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonLeafTournament *NonLeafTournamentTransactor) WinInnerTournament(opts *bind.TransactOpts, _childTournament common.Address, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.contract.Transact(opts, "winInnerTournament", _childTournament, _leftNode, _rightNode)
}

// WinInnerTournament is a paid mutator transaction binding the contract method 0x4a95153e.
//
// Solidity: function winInnerTournament(address _childTournament, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonLeafTournament *NonLeafTournamentSession) WinInnerTournament(_childTournament common.Address, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.WinInnerTournament(&_NonLeafTournament.TransactOpts, _childTournament, _leftNode, _rightNode)
}

// WinInnerTournament is a paid mutator transaction binding the contract method 0x4a95153e.
//
// Solidity: function winInnerTournament(address _childTournament, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonLeafTournament *NonLeafTournamentTransactorSession) WinInnerTournament(_childTournament common.Address, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.WinInnerTournament(&_NonLeafTournament.TransactOpts, _childTournament, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonLeafTournament *NonLeafTournamentTransactor) WinMatchByTimeout(opts *bind.TransactOpts, _matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.contract.Transact(opts, "winMatchByTimeout", _matchId, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonLeafTournament *NonLeafTournamentSession) WinMatchByTimeout(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.WinMatchByTimeout(&_NonLeafTournament.TransactOpts, _matchId, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_NonLeafTournament *NonLeafTournamentTransactorSession) WinMatchByTimeout(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _NonLeafTournament.Contract.WinMatchByTimeout(&_NonLeafTournament.TransactOpts, _matchId, _leftNode, _rightNode)
}

// NonLeafTournamentCommitmentJoinedIterator is returned from FilterCommitmentJoined and is used to iterate over the raw logs and unpacked data for CommitmentJoined events raised by the NonLeafTournament contract.
type NonLeafTournamentCommitmentJoinedIterator struct {
	Event *NonLeafTournamentCommitmentJoined // Event containing the contract specifics and raw log

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
func (it *NonLeafTournamentCommitmentJoinedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(NonLeafTournamentCommitmentJoined)
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
		it.Event = new(NonLeafTournamentCommitmentJoined)
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
func (it *NonLeafTournamentCommitmentJoinedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *NonLeafTournamentCommitmentJoinedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// NonLeafTournamentCommitmentJoined represents a CommitmentJoined event raised by the NonLeafTournament contract.
type NonLeafTournamentCommitmentJoined struct {
	Root [32]byte
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterCommitmentJoined is a free log retrieval operation binding the contract event 0xe53537f202911d376d6e285835b2a2016e83e99fbe84a059d445cc2be4807262.
//
// Solidity: event commitmentJoined(bytes32 root)
func (_NonLeafTournament *NonLeafTournamentFilterer) FilterCommitmentJoined(opts *bind.FilterOpts) (*NonLeafTournamentCommitmentJoinedIterator, error) {

	logs, sub, err := _NonLeafTournament.contract.FilterLogs(opts, "commitmentJoined")
	if err != nil {
		return nil, err
	}
	return &NonLeafTournamentCommitmentJoinedIterator{contract: _NonLeafTournament.contract, event: "commitmentJoined", logs: logs, sub: sub}, nil
}

// WatchCommitmentJoined is a free log subscription operation binding the contract event 0xe53537f202911d376d6e285835b2a2016e83e99fbe84a059d445cc2be4807262.
//
// Solidity: event commitmentJoined(bytes32 root)
func (_NonLeafTournament *NonLeafTournamentFilterer) WatchCommitmentJoined(opts *bind.WatchOpts, sink chan<- *NonLeafTournamentCommitmentJoined) (event.Subscription, error) {

	logs, sub, err := _NonLeafTournament.contract.WatchLogs(opts, "commitmentJoined")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(NonLeafTournamentCommitmentJoined)
				if err := _NonLeafTournament.contract.UnpackLog(event, "commitmentJoined", log); err != nil {
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
func (_NonLeafTournament *NonLeafTournamentFilterer) ParseCommitmentJoined(log types.Log) (*NonLeafTournamentCommitmentJoined, error) {
	event := new(NonLeafTournamentCommitmentJoined)
	if err := _NonLeafTournament.contract.UnpackLog(event, "commitmentJoined", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// NonLeafTournamentMatchAdvancedIterator is returned from FilterMatchAdvanced and is used to iterate over the raw logs and unpacked data for MatchAdvanced events raised by the NonLeafTournament contract.
type NonLeafTournamentMatchAdvancedIterator struct {
	Event *NonLeafTournamentMatchAdvanced // Event containing the contract specifics and raw log

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
func (it *NonLeafTournamentMatchAdvancedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(NonLeafTournamentMatchAdvanced)
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
		it.Event = new(NonLeafTournamentMatchAdvanced)
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
func (it *NonLeafTournamentMatchAdvancedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *NonLeafTournamentMatchAdvancedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// NonLeafTournamentMatchAdvanced represents a MatchAdvanced event raised by the NonLeafTournament contract.
type NonLeafTournamentMatchAdvanced struct {
	Arg0   [32]byte
	Parent [32]byte
	Left   [32]byte
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterMatchAdvanced is a free log retrieval operation binding the contract event 0x29ff393c59c37f91e930fad4d88447efc58cf5d7c048499e1f20edb369941378.
//
// Solidity: event matchAdvanced(bytes32 indexed arg0, bytes32 parent, bytes32 left)
func (_NonLeafTournament *NonLeafTournamentFilterer) FilterMatchAdvanced(opts *bind.FilterOpts, arg0 [][32]byte) (*NonLeafTournamentMatchAdvancedIterator, error) {

	var arg0Rule []interface{}
	for _, arg0Item := range arg0 {
		arg0Rule = append(arg0Rule, arg0Item)
	}

	logs, sub, err := _NonLeafTournament.contract.FilterLogs(opts, "matchAdvanced", arg0Rule)
	if err != nil {
		return nil, err
	}
	return &NonLeafTournamentMatchAdvancedIterator{contract: _NonLeafTournament.contract, event: "matchAdvanced", logs: logs, sub: sub}, nil
}

// WatchMatchAdvanced is a free log subscription operation binding the contract event 0x29ff393c59c37f91e930fad4d88447efc58cf5d7c048499e1f20edb369941378.
//
// Solidity: event matchAdvanced(bytes32 indexed arg0, bytes32 parent, bytes32 left)
func (_NonLeafTournament *NonLeafTournamentFilterer) WatchMatchAdvanced(opts *bind.WatchOpts, sink chan<- *NonLeafTournamentMatchAdvanced, arg0 [][32]byte) (event.Subscription, error) {

	var arg0Rule []interface{}
	for _, arg0Item := range arg0 {
		arg0Rule = append(arg0Rule, arg0Item)
	}

	logs, sub, err := _NonLeafTournament.contract.WatchLogs(opts, "matchAdvanced", arg0Rule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(NonLeafTournamentMatchAdvanced)
				if err := _NonLeafTournament.contract.UnpackLog(event, "matchAdvanced", log); err != nil {
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
func (_NonLeafTournament *NonLeafTournamentFilterer) ParseMatchAdvanced(log types.Log) (*NonLeafTournamentMatchAdvanced, error) {
	event := new(NonLeafTournamentMatchAdvanced)
	if err := _NonLeafTournament.contract.UnpackLog(event, "matchAdvanced", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// NonLeafTournamentMatchCreatedIterator is returned from FilterMatchCreated and is used to iterate over the raw logs and unpacked data for MatchCreated events raised by the NonLeafTournament contract.
type NonLeafTournamentMatchCreatedIterator struct {
	Event *NonLeafTournamentMatchCreated // Event containing the contract specifics and raw log

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
func (it *NonLeafTournamentMatchCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(NonLeafTournamentMatchCreated)
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
		it.Event = new(NonLeafTournamentMatchCreated)
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
func (it *NonLeafTournamentMatchCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *NonLeafTournamentMatchCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// NonLeafTournamentMatchCreated represents a MatchCreated event raised by the NonLeafTournament contract.
type NonLeafTournamentMatchCreated struct {
	One       [32]byte
	Two       [32]byte
	LeftOfTwo [32]byte
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterMatchCreated is a free log retrieval operation binding the contract event 0x32911001007d8c9879b608566be8acc2184592f0a43706f804f285455bb0f52e.
//
// Solidity: event matchCreated(bytes32 indexed one, bytes32 indexed two, bytes32 leftOfTwo)
func (_NonLeafTournament *NonLeafTournamentFilterer) FilterMatchCreated(opts *bind.FilterOpts, one [][32]byte, two [][32]byte) (*NonLeafTournamentMatchCreatedIterator, error) {

	var oneRule []interface{}
	for _, oneItem := range one {
		oneRule = append(oneRule, oneItem)
	}
	var twoRule []interface{}
	for _, twoItem := range two {
		twoRule = append(twoRule, twoItem)
	}

	logs, sub, err := _NonLeafTournament.contract.FilterLogs(opts, "matchCreated", oneRule, twoRule)
	if err != nil {
		return nil, err
	}
	return &NonLeafTournamentMatchCreatedIterator{contract: _NonLeafTournament.contract, event: "matchCreated", logs: logs, sub: sub}, nil
}

// WatchMatchCreated is a free log subscription operation binding the contract event 0x32911001007d8c9879b608566be8acc2184592f0a43706f804f285455bb0f52e.
//
// Solidity: event matchCreated(bytes32 indexed one, bytes32 indexed two, bytes32 leftOfTwo)
func (_NonLeafTournament *NonLeafTournamentFilterer) WatchMatchCreated(opts *bind.WatchOpts, sink chan<- *NonLeafTournamentMatchCreated, one [][32]byte, two [][32]byte) (event.Subscription, error) {

	var oneRule []interface{}
	for _, oneItem := range one {
		oneRule = append(oneRule, oneItem)
	}
	var twoRule []interface{}
	for _, twoItem := range two {
		twoRule = append(twoRule, twoItem)
	}

	logs, sub, err := _NonLeafTournament.contract.WatchLogs(opts, "matchCreated", oneRule, twoRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(NonLeafTournamentMatchCreated)
				if err := _NonLeafTournament.contract.UnpackLog(event, "matchCreated", log); err != nil {
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
func (_NonLeafTournament *NonLeafTournamentFilterer) ParseMatchCreated(log types.Log) (*NonLeafTournamentMatchCreated, error) {
	event := new(NonLeafTournamentMatchCreated)
	if err := _NonLeafTournament.contract.UnpackLog(event, "matchCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// NonLeafTournamentMatchDeletedIterator is returned from FilterMatchDeleted and is used to iterate over the raw logs and unpacked data for MatchDeleted events raised by the NonLeafTournament contract.
type NonLeafTournamentMatchDeletedIterator struct {
	Event *NonLeafTournamentMatchDeleted // Event containing the contract specifics and raw log

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
func (it *NonLeafTournamentMatchDeletedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(NonLeafTournamentMatchDeleted)
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
		it.Event = new(NonLeafTournamentMatchDeleted)
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
func (it *NonLeafTournamentMatchDeletedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *NonLeafTournamentMatchDeletedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// NonLeafTournamentMatchDeleted represents a MatchDeleted event raised by the NonLeafTournament contract.
type NonLeafTournamentMatchDeleted struct {
	Arg0 [32]byte
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterMatchDeleted is a free log retrieval operation binding the contract event 0x0afce37c521a4613a2db0c4983987a3c4af722e33d3412963fccbc0eb0df0d28.
//
// Solidity: event matchDeleted(bytes32 arg0)
func (_NonLeafTournament *NonLeafTournamentFilterer) FilterMatchDeleted(opts *bind.FilterOpts) (*NonLeafTournamentMatchDeletedIterator, error) {

	logs, sub, err := _NonLeafTournament.contract.FilterLogs(opts, "matchDeleted")
	if err != nil {
		return nil, err
	}
	return &NonLeafTournamentMatchDeletedIterator{contract: _NonLeafTournament.contract, event: "matchDeleted", logs: logs, sub: sub}, nil
}

// WatchMatchDeleted is a free log subscription operation binding the contract event 0x0afce37c521a4613a2db0c4983987a3c4af722e33d3412963fccbc0eb0df0d28.
//
// Solidity: event matchDeleted(bytes32 arg0)
func (_NonLeafTournament *NonLeafTournamentFilterer) WatchMatchDeleted(opts *bind.WatchOpts, sink chan<- *NonLeafTournamentMatchDeleted) (event.Subscription, error) {

	logs, sub, err := _NonLeafTournament.contract.WatchLogs(opts, "matchDeleted")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(NonLeafTournamentMatchDeleted)
				if err := _NonLeafTournament.contract.UnpackLog(event, "matchDeleted", log); err != nil {
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
func (_NonLeafTournament *NonLeafTournamentFilterer) ParseMatchDeleted(log types.Log) (*NonLeafTournamentMatchDeleted, error) {
	event := new(NonLeafTournamentMatchDeleted)
	if err := _NonLeafTournament.contract.UnpackLog(event, "matchDeleted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// NonLeafTournamentNewInnerTournamentIterator is returned from FilterNewInnerTournament and is used to iterate over the raw logs and unpacked data for NewInnerTournament events raised by the NonLeafTournament contract.
type NonLeafTournamentNewInnerTournamentIterator struct {
	Event *NonLeafTournamentNewInnerTournament // Event containing the contract specifics and raw log

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
func (it *NonLeafTournamentNewInnerTournamentIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(NonLeafTournamentNewInnerTournament)
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
		it.Event = new(NonLeafTournamentNewInnerTournament)
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
func (it *NonLeafTournamentNewInnerTournamentIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *NonLeafTournamentNewInnerTournamentIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// NonLeafTournamentNewInnerTournament represents a NewInnerTournament event raised by the NonLeafTournament contract.
type NonLeafTournamentNewInnerTournament struct {
	Arg0 [32]byte
	Arg1 common.Address
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterNewInnerTournament is a free log retrieval operation binding the contract event 0x166b786700a644dcec96aa84a92c26ecfee11b50956409c8cc83616042290d79.
//
// Solidity: event newInnerTournament(bytes32 indexed arg0, address arg1)
func (_NonLeafTournament *NonLeafTournamentFilterer) FilterNewInnerTournament(opts *bind.FilterOpts, arg0 [][32]byte) (*NonLeafTournamentNewInnerTournamentIterator, error) {

	var arg0Rule []interface{}
	for _, arg0Item := range arg0 {
		arg0Rule = append(arg0Rule, arg0Item)
	}

	logs, sub, err := _NonLeafTournament.contract.FilterLogs(opts, "newInnerTournament", arg0Rule)
	if err != nil {
		return nil, err
	}
	return &NonLeafTournamentNewInnerTournamentIterator{contract: _NonLeafTournament.contract, event: "newInnerTournament", logs: logs, sub: sub}, nil
}

// WatchNewInnerTournament is a free log subscription operation binding the contract event 0x166b786700a644dcec96aa84a92c26ecfee11b50956409c8cc83616042290d79.
//
// Solidity: event newInnerTournament(bytes32 indexed arg0, address arg1)
func (_NonLeafTournament *NonLeafTournamentFilterer) WatchNewInnerTournament(opts *bind.WatchOpts, sink chan<- *NonLeafTournamentNewInnerTournament, arg0 [][32]byte) (event.Subscription, error) {

	var arg0Rule []interface{}
	for _, arg0Item := range arg0 {
		arg0Rule = append(arg0Rule, arg0Item)
	}

	logs, sub, err := _NonLeafTournament.contract.WatchLogs(opts, "newInnerTournament", arg0Rule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(NonLeafTournamentNewInnerTournament)
				if err := _NonLeafTournament.contract.UnpackLog(event, "newInnerTournament", log); err != nil {
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
func (_NonLeafTournament *NonLeafTournamentFilterer) ParseNewInnerTournament(log types.Log) (*NonLeafTournamentNewInnerTournament, error) {
	event := new(NonLeafTournamentNewInnerTournament)
	if err := _NonLeafTournament.contract.UnpackLog(event, "newInnerTournament", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
