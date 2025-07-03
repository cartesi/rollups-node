// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package tournament

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

// TournamentMetaData contains all meta data concerning the Tournament contract.
var TournamentMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"advanceMatch\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_newLeftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_newRightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"canWinMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"eliminateMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getCommitment\",\"inputs\":[{\"name\":\"_commitmentRoot\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structClock.State\",\"components\":[{\"name\":\"allowance\",\"type\":\"uint64\",\"internalType\":\"Time.Duration\"},{\"name\":\"startInstant\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"}]},{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMatch\",\"inputs\":[{\"name\":\"_matchIdHash\",\"type\":\"bytes32\",\"internalType\":\"Match.IdHash\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structMatch.State\",\"components\":[{\"name\":\"otherParent\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"runningLeafPosition\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentHeight\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"log2step\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"height\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getMatchCycle\",\"inputs\":[{\"name\":\"_matchIdHash\",\"type\":\"bytes32\",\"internalType\":\"Match.IdHash\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isClosed\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isFinished\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"joinTournament\",\"inputs\":[{\"name\":\"_finalState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"_proof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"timeFinished\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"Time.Instant\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"tournamentLevelConstants\",\"inputs\":[],\"outputs\":[{\"name\":\"_maxLevel\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_level\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_log2step\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_height\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"winMatchByTimeout\",\"inputs\":[{\"name\":\"_matchId\",\"type\":\"tuple\",\"internalType\":\"structMatch.Id\",\"components\":[{\"name\":\"commitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"commitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]},{\"name\":\"_leftNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_rightNode\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"commitmentJoined\",\"inputs\":[{\"name\":\"root\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchAdvanced\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Match.IdHash\"},{\"name\":\"parent\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"},{\"name\":\"left\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchCreated\",\"inputs\":[{\"name\":\"one\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"two\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"Tree.Node\"},{\"name\":\"leftOfTwo\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Tree.Node\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"matchDeleted\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"Match.IdHash\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"EliminateByTimeout\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidContestedFinalState\",\"inputs\":[{\"name\":\"contestedFinalStateOne\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"contestedFinalStateTwo\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"finalState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}]},{\"type\":\"error\",\"name\":\"TournamentIsClosed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TournamentIsFinished\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WinByTimeout\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"WrongChildren\",\"inputs\":[{\"name\":\"commitment\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"parent\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"left\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"right\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"}]}]",
}

// TournamentABI is the input ABI used to generate the binding from.
// Deprecated: Use TournamentMetaData.ABI instead.
var TournamentABI = TournamentMetaData.ABI

// Tournament is an auto generated Go binding around an Ethereum contract.
type Tournament struct {
	TournamentCaller     // Read-only binding to the contract
	TournamentTransactor // Write-only binding to the contract
	TournamentFilterer   // Log filterer for contract events
}

// TournamentCaller is an auto generated read-only Go binding around an Ethereum contract.
type TournamentCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TournamentTransactor is an auto generated write-only Go binding around an Ethereum contract.
type TournamentTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TournamentFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type TournamentFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TournamentSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type TournamentSession struct {
	Contract     *Tournament       // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// TournamentCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type TournamentCallerSession struct {
	Contract *TournamentCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts     // Call options to use throughout this session
}

// TournamentTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type TournamentTransactorSession struct {
	Contract     *TournamentTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// TournamentRaw is an auto generated low-level Go binding around an Ethereum contract.
type TournamentRaw struct {
	Contract *Tournament // Generic contract binding to access the raw methods on
}

// TournamentCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type TournamentCallerRaw struct {
	Contract *TournamentCaller // Generic read-only contract binding to access the raw methods on
}

// TournamentTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type TournamentTransactorRaw struct {
	Contract *TournamentTransactor // Generic write-only contract binding to access the raw methods on
}

// NewTournament creates a new instance of Tournament, bound to a specific deployed contract.
func NewTournament(address common.Address, backend bind.ContractBackend) (*Tournament, error) {
	contract, err := bindTournament(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Tournament{TournamentCaller: TournamentCaller{contract: contract}, TournamentTransactor: TournamentTransactor{contract: contract}, TournamentFilterer: TournamentFilterer{contract: contract}}, nil
}

// NewTournamentCaller creates a new read-only instance of Tournament, bound to a specific deployed contract.
func NewTournamentCaller(address common.Address, caller bind.ContractCaller) (*TournamentCaller, error) {
	contract, err := bindTournament(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &TournamentCaller{contract: contract}, nil
}

// NewTournamentTransactor creates a new write-only instance of Tournament, bound to a specific deployed contract.
func NewTournamentTransactor(address common.Address, transactor bind.ContractTransactor) (*TournamentTransactor, error) {
	contract, err := bindTournament(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &TournamentTransactor{contract: contract}, nil
}

// NewTournamentFilterer creates a new log filterer instance of Tournament, bound to a specific deployed contract.
func NewTournamentFilterer(address common.Address, filterer bind.ContractFilterer) (*TournamentFilterer, error) {
	contract, err := bindTournament(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &TournamentFilterer{contract: contract}, nil
}

// bindTournament binds a generic wrapper to an already deployed contract.
func bindTournament(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := TournamentMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Tournament *TournamentRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Tournament.Contract.TournamentCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Tournament *TournamentRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Tournament.Contract.TournamentTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Tournament *TournamentRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Tournament.Contract.TournamentTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Tournament *TournamentCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Tournament.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Tournament *TournamentTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Tournament.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Tournament *TournamentTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Tournament.Contract.contract.Transact(opts, method, params...)
}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_Tournament *TournamentCaller) CanWinMatchByTimeout(opts *bind.CallOpts, _matchId MatchId) (bool, error) {
	var out []interface{}
	err := _Tournament.contract.Call(opts, &out, "canWinMatchByTimeout", _matchId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_Tournament *TournamentSession) CanWinMatchByTimeout(_matchId MatchId) (bool, error) {
	return _Tournament.Contract.CanWinMatchByTimeout(&_Tournament.CallOpts, _matchId)
}

// CanWinMatchByTimeout is a free data retrieval call binding the contract method 0x6a1a140d.
//
// Solidity: function canWinMatchByTimeout((bytes32,bytes32) _matchId) view returns(bool)
func (_Tournament *TournamentCallerSession) CanWinMatchByTimeout(_matchId MatchId) (bool, error) {
	return _Tournament.Contract.CanWinMatchByTimeout(&_Tournament.CallOpts, _matchId)
}

// GetCommitment is a free data retrieval call binding the contract method 0x7795820c.
//
// Solidity: function getCommitment(bytes32 _commitmentRoot) view returns((uint64,uint64), bytes32)
func (_Tournament *TournamentCaller) GetCommitment(opts *bind.CallOpts, _commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	var out []interface{}
	err := _Tournament.contract.Call(opts, &out, "getCommitment", _commitmentRoot)

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
func (_Tournament *TournamentSession) GetCommitment(_commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	return _Tournament.Contract.GetCommitment(&_Tournament.CallOpts, _commitmentRoot)
}

// GetCommitment is a free data retrieval call binding the contract method 0x7795820c.
//
// Solidity: function getCommitment(bytes32 _commitmentRoot) view returns((uint64,uint64), bytes32)
func (_Tournament *TournamentCallerSession) GetCommitment(_commitmentRoot [32]byte) (ClockState, [32]byte, error) {
	return _Tournament.Contract.GetCommitment(&_Tournament.CallOpts, _commitmentRoot)
}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_Tournament *TournamentCaller) GetMatch(opts *bind.CallOpts, _matchIdHash [32]byte) (MatchState, error) {
	var out []interface{}
	err := _Tournament.contract.Call(opts, &out, "getMatch", _matchIdHash)

	if err != nil {
		return *new(MatchState), err
	}

	out0 := *abi.ConvertType(out[0], new(MatchState)).(*MatchState)

	return out0, err

}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_Tournament *TournamentSession) GetMatch(_matchIdHash [32]byte) (MatchState, error) {
	return _Tournament.Contract.GetMatch(&_Tournament.CallOpts, _matchIdHash)
}

// GetMatch is a free data retrieval call binding the contract method 0xfcc6077d.
//
// Solidity: function getMatch(bytes32 _matchIdHash) view returns((bytes32,bytes32,bytes32,uint256,uint64,uint64,uint64))
func (_Tournament *TournamentCallerSession) GetMatch(_matchIdHash [32]byte) (MatchState, error) {
	return _Tournament.Contract.GetMatch(&_Tournament.CallOpts, _matchIdHash)
}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_Tournament *TournamentCaller) GetMatchCycle(opts *bind.CallOpts, _matchIdHash [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _Tournament.contract.Call(opts, &out, "getMatchCycle", _matchIdHash)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_Tournament *TournamentSession) GetMatchCycle(_matchIdHash [32]byte) (*big.Int, error) {
	return _Tournament.Contract.GetMatchCycle(&_Tournament.CallOpts, _matchIdHash)
}

// GetMatchCycle is a free data retrieval call binding the contract method 0x8acc802d.
//
// Solidity: function getMatchCycle(bytes32 _matchIdHash) view returns(uint256)
func (_Tournament *TournamentCallerSession) GetMatchCycle(_matchIdHash [32]byte) (*big.Int, error) {
	return _Tournament.Contract.GetMatchCycle(&_Tournament.CallOpts, _matchIdHash)
}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_Tournament *TournamentCaller) IsClosed(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _Tournament.contract.Call(opts, &out, "isClosed")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_Tournament *TournamentSession) IsClosed() (bool, error) {
	return _Tournament.Contract.IsClosed(&_Tournament.CallOpts)
}

// IsClosed is a free data retrieval call binding the contract method 0xc2b6b58c.
//
// Solidity: function isClosed() view returns(bool)
func (_Tournament *TournamentCallerSession) IsClosed() (bool, error) {
	return _Tournament.Contract.IsClosed(&_Tournament.CallOpts)
}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_Tournament *TournamentCaller) IsFinished(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _Tournament.contract.Call(opts, &out, "isFinished")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_Tournament *TournamentSession) IsFinished() (bool, error) {
	return _Tournament.Contract.IsFinished(&_Tournament.CallOpts)
}

// IsFinished is a free data retrieval call binding the contract method 0x7b352962.
//
// Solidity: function isFinished() view returns(bool)
func (_Tournament *TournamentCallerSession) IsFinished() (bool, error) {
	return _Tournament.Contract.IsFinished(&_Tournament.CallOpts)
}

// TimeFinished is a free data retrieval call binding the contract method 0x39cdfaf2.
//
// Solidity: function timeFinished() view returns(bool, uint64)
func (_Tournament *TournamentCaller) TimeFinished(opts *bind.CallOpts) (bool, uint64, error) {
	var out []interface{}
	err := _Tournament.contract.Call(opts, &out, "timeFinished")

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
func (_Tournament *TournamentSession) TimeFinished() (bool, uint64, error) {
	return _Tournament.Contract.TimeFinished(&_Tournament.CallOpts)
}

// TimeFinished is a free data retrieval call binding the contract method 0x39cdfaf2.
//
// Solidity: function timeFinished() view returns(bool, uint64)
func (_Tournament *TournamentCallerSession) TimeFinished() (bool, uint64, error) {
	return _Tournament.Contract.TimeFinished(&_Tournament.CallOpts)
}

// TournamentLevelConstants is a free data retrieval call binding the contract method 0xa1af906b.
//
// Solidity: function tournamentLevelConstants() view returns(uint64 _maxLevel, uint64 _level, uint64 _log2step, uint64 _height)
func (_Tournament *TournamentCaller) TournamentLevelConstants(opts *bind.CallOpts) (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	var out []interface{}
	err := _Tournament.contract.Call(opts, &out, "tournamentLevelConstants")

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
func (_Tournament *TournamentSession) TournamentLevelConstants() (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	return _Tournament.Contract.TournamentLevelConstants(&_Tournament.CallOpts)
}

// TournamentLevelConstants is a free data retrieval call binding the contract method 0xa1af906b.
//
// Solidity: function tournamentLevelConstants() view returns(uint64 _maxLevel, uint64 _level, uint64 _log2step, uint64 _height)
func (_Tournament *TournamentCallerSession) TournamentLevelConstants() (struct {
	MaxLevel uint64
	Level    uint64
	Log2step uint64
	Height   uint64
}, error) {
	return _Tournament.Contract.TournamentLevelConstants(&_Tournament.CallOpts)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_Tournament *TournamentTransactor) AdvanceMatch(opts *bind.TransactOpts, _matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _Tournament.contract.Transact(opts, "advanceMatch", _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_Tournament *TournamentSession) AdvanceMatch(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _Tournament.Contract.AdvanceMatch(&_Tournament.TransactOpts, _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// AdvanceMatch is a paid mutator transaction binding the contract method 0xfcc85391.
//
// Solidity: function advanceMatch((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode, bytes32 _newLeftNode, bytes32 _newRightNode) returns()
func (_Tournament *TournamentTransactorSession) AdvanceMatch(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte, _newLeftNode [32]byte, _newRightNode [32]byte) (*types.Transaction, error) {
	return _Tournament.Contract.AdvanceMatch(&_Tournament.TransactOpts, _matchId, _leftNode, _rightNode, _newLeftNode, _newRightNode)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_Tournament *TournamentTransactor) EliminateMatchByTimeout(opts *bind.TransactOpts, _matchId MatchId) (*types.Transaction, error) {
	return _Tournament.contract.Transact(opts, "eliminateMatchByTimeout", _matchId)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_Tournament *TournamentSession) EliminateMatchByTimeout(_matchId MatchId) (*types.Transaction, error) {
	return _Tournament.Contract.EliminateMatchByTimeout(&_Tournament.TransactOpts, _matchId)
}

// EliminateMatchByTimeout is a paid mutator transaction binding the contract method 0x9a9b4b2b.
//
// Solidity: function eliminateMatchByTimeout((bytes32,bytes32) _matchId) returns()
func (_Tournament *TournamentTransactorSession) EliminateMatchByTimeout(_matchId MatchId) (*types.Transaction, error) {
	return _Tournament.Contract.EliminateMatchByTimeout(&_Tournament.TransactOpts, _matchId)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_Tournament *TournamentTransactor) JoinTournament(opts *bind.TransactOpts, _finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _Tournament.contract.Transact(opts, "joinTournament", _finalState, _proof, _leftNode, _rightNode)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_Tournament *TournamentSession) JoinTournament(_finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _Tournament.Contract.JoinTournament(&_Tournament.TransactOpts, _finalState, _proof, _leftNode, _rightNode)
}

// JoinTournament is a paid mutator transaction binding the contract method 0x1d5bf796.
//
// Solidity: function joinTournament(bytes32 _finalState, bytes32[] _proof, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_Tournament *TournamentTransactorSession) JoinTournament(_finalState [32]byte, _proof [][32]byte, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _Tournament.Contract.JoinTournament(&_Tournament.TransactOpts, _finalState, _proof, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_Tournament *TournamentTransactor) WinMatchByTimeout(opts *bind.TransactOpts, _matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _Tournament.contract.Transact(opts, "winMatchByTimeout", _matchId, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_Tournament *TournamentSession) WinMatchByTimeout(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _Tournament.Contract.WinMatchByTimeout(&_Tournament.TransactOpts, _matchId, _leftNode, _rightNode)
}

// WinMatchByTimeout is a paid mutator transaction binding the contract method 0xff78e0ee.
//
// Solidity: function winMatchByTimeout((bytes32,bytes32) _matchId, bytes32 _leftNode, bytes32 _rightNode) returns()
func (_Tournament *TournamentTransactorSession) WinMatchByTimeout(_matchId MatchId, _leftNode [32]byte, _rightNode [32]byte) (*types.Transaction, error) {
	return _Tournament.Contract.WinMatchByTimeout(&_Tournament.TransactOpts, _matchId, _leftNode, _rightNode)
}

// TournamentCommitmentJoinedIterator is returned from FilterCommitmentJoined and is used to iterate over the raw logs and unpacked data for CommitmentJoined events raised by the Tournament contract.
type TournamentCommitmentJoinedIterator struct {
	Event *TournamentCommitmentJoined // Event containing the contract specifics and raw log

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
func (it *TournamentCommitmentJoinedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TournamentCommitmentJoined)
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
		it.Event = new(TournamentCommitmentJoined)
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
func (it *TournamentCommitmentJoinedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TournamentCommitmentJoinedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TournamentCommitmentJoined represents a CommitmentJoined event raised by the Tournament contract.
type TournamentCommitmentJoined struct {
	Root [32]byte
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterCommitmentJoined is a free log retrieval operation binding the contract event 0xe53537f202911d376d6e285835b2a2016e83e99fbe84a059d445cc2be4807262.
//
// Solidity: event commitmentJoined(bytes32 root)
func (_Tournament *TournamentFilterer) FilterCommitmentJoined(opts *bind.FilterOpts) (*TournamentCommitmentJoinedIterator, error) {

	logs, sub, err := _Tournament.contract.FilterLogs(opts, "commitmentJoined")
	if err != nil {
		return nil, err
	}
	return &TournamentCommitmentJoinedIterator{contract: _Tournament.contract, event: "commitmentJoined", logs: logs, sub: sub}, nil
}

// WatchCommitmentJoined is a free log subscription operation binding the contract event 0xe53537f202911d376d6e285835b2a2016e83e99fbe84a059d445cc2be4807262.
//
// Solidity: event commitmentJoined(bytes32 root)
func (_Tournament *TournamentFilterer) WatchCommitmentJoined(opts *bind.WatchOpts, sink chan<- *TournamentCommitmentJoined) (event.Subscription, error) {

	logs, sub, err := _Tournament.contract.WatchLogs(opts, "commitmentJoined")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TournamentCommitmentJoined)
				if err := _Tournament.contract.UnpackLog(event, "commitmentJoined", log); err != nil {
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
func (_Tournament *TournamentFilterer) ParseCommitmentJoined(log types.Log) (*TournamentCommitmentJoined, error) {
	event := new(TournamentCommitmentJoined)
	if err := _Tournament.contract.UnpackLog(event, "commitmentJoined", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TournamentMatchAdvancedIterator is returned from FilterMatchAdvanced and is used to iterate over the raw logs and unpacked data for MatchAdvanced events raised by the Tournament contract.
type TournamentMatchAdvancedIterator struct {
	Event *TournamentMatchAdvanced // Event containing the contract specifics and raw log

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
func (it *TournamentMatchAdvancedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TournamentMatchAdvanced)
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
		it.Event = new(TournamentMatchAdvanced)
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
func (it *TournamentMatchAdvancedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TournamentMatchAdvancedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TournamentMatchAdvanced represents a MatchAdvanced event raised by the Tournament contract.
type TournamentMatchAdvanced struct {
	Arg0   [32]byte
	Parent [32]byte
	Left   [32]byte
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterMatchAdvanced is a free log retrieval operation binding the contract event 0x29ff393c59c37f91e930fad4d88447efc58cf5d7c048499e1f20edb369941378.
//
// Solidity: event matchAdvanced(bytes32 indexed arg0, bytes32 parent, bytes32 left)
func (_Tournament *TournamentFilterer) FilterMatchAdvanced(opts *bind.FilterOpts, arg0 [][32]byte) (*TournamentMatchAdvancedIterator, error) {

	var arg0Rule []interface{}
	for _, arg0Item := range arg0 {
		arg0Rule = append(arg0Rule, arg0Item)
	}

	logs, sub, err := _Tournament.contract.FilterLogs(opts, "matchAdvanced", arg0Rule)
	if err != nil {
		return nil, err
	}
	return &TournamentMatchAdvancedIterator{contract: _Tournament.contract, event: "matchAdvanced", logs: logs, sub: sub}, nil
}

// WatchMatchAdvanced is a free log subscription operation binding the contract event 0x29ff393c59c37f91e930fad4d88447efc58cf5d7c048499e1f20edb369941378.
//
// Solidity: event matchAdvanced(bytes32 indexed arg0, bytes32 parent, bytes32 left)
func (_Tournament *TournamentFilterer) WatchMatchAdvanced(opts *bind.WatchOpts, sink chan<- *TournamentMatchAdvanced, arg0 [][32]byte) (event.Subscription, error) {

	var arg0Rule []interface{}
	for _, arg0Item := range arg0 {
		arg0Rule = append(arg0Rule, arg0Item)
	}

	logs, sub, err := _Tournament.contract.WatchLogs(opts, "matchAdvanced", arg0Rule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TournamentMatchAdvanced)
				if err := _Tournament.contract.UnpackLog(event, "matchAdvanced", log); err != nil {
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
func (_Tournament *TournamentFilterer) ParseMatchAdvanced(log types.Log) (*TournamentMatchAdvanced, error) {
	event := new(TournamentMatchAdvanced)
	if err := _Tournament.contract.UnpackLog(event, "matchAdvanced", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TournamentMatchCreatedIterator is returned from FilterMatchCreated and is used to iterate over the raw logs and unpacked data for MatchCreated events raised by the Tournament contract.
type TournamentMatchCreatedIterator struct {
	Event *TournamentMatchCreated // Event containing the contract specifics and raw log

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
func (it *TournamentMatchCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TournamentMatchCreated)
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
		it.Event = new(TournamentMatchCreated)
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
func (it *TournamentMatchCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TournamentMatchCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TournamentMatchCreated represents a MatchCreated event raised by the Tournament contract.
type TournamentMatchCreated struct {
	One       [32]byte
	Two       [32]byte
	LeftOfTwo [32]byte
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterMatchCreated is a free log retrieval operation binding the contract event 0x32911001007d8c9879b608566be8acc2184592f0a43706f804f285455bb0f52e.
//
// Solidity: event matchCreated(bytes32 indexed one, bytes32 indexed two, bytes32 leftOfTwo)
func (_Tournament *TournamentFilterer) FilterMatchCreated(opts *bind.FilterOpts, one [][32]byte, two [][32]byte) (*TournamentMatchCreatedIterator, error) {

	var oneRule []interface{}
	for _, oneItem := range one {
		oneRule = append(oneRule, oneItem)
	}
	var twoRule []interface{}
	for _, twoItem := range two {
		twoRule = append(twoRule, twoItem)
	}

	logs, sub, err := _Tournament.contract.FilterLogs(opts, "matchCreated", oneRule, twoRule)
	if err != nil {
		return nil, err
	}
	return &TournamentMatchCreatedIterator{contract: _Tournament.contract, event: "matchCreated", logs: logs, sub: sub}, nil
}

// WatchMatchCreated is a free log subscription operation binding the contract event 0x32911001007d8c9879b608566be8acc2184592f0a43706f804f285455bb0f52e.
//
// Solidity: event matchCreated(bytes32 indexed one, bytes32 indexed two, bytes32 leftOfTwo)
func (_Tournament *TournamentFilterer) WatchMatchCreated(opts *bind.WatchOpts, sink chan<- *TournamentMatchCreated, one [][32]byte, two [][32]byte) (event.Subscription, error) {

	var oneRule []interface{}
	for _, oneItem := range one {
		oneRule = append(oneRule, oneItem)
	}
	var twoRule []interface{}
	for _, twoItem := range two {
		twoRule = append(twoRule, twoItem)
	}

	logs, sub, err := _Tournament.contract.WatchLogs(opts, "matchCreated", oneRule, twoRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TournamentMatchCreated)
				if err := _Tournament.contract.UnpackLog(event, "matchCreated", log); err != nil {
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
func (_Tournament *TournamentFilterer) ParseMatchCreated(log types.Log) (*TournamentMatchCreated, error) {
	event := new(TournamentMatchCreated)
	if err := _Tournament.contract.UnpackLog(event, "matchCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// TournamentMatchDeletedIterator is returned from FilterMatchDeleted and is used to iterate over the raw logs and unpacked data for MatchDeleted events raised by the Tournament contract.
type TournamentMatchDeletedIterator struct {
	Event *TournamentMatchDeleted // Event containing the contract specifics and raw log

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
func (it *TournamentMatchDeletedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TournamentMatchDeleted)
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
		it.Event = new(TournamentMatchDeleted)
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
func (it *TournamentMatchDeletedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TournamentMatchDeletedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TournamentMatchDeleted represents a MatchDeleted event raised by the Tournament contract.
type TournamentMatchDeleted struct {
	Arg0 [32]byte
	Raw  types.Log // Blockchain specific contextual infos
}

// FilterMatchDeleted is a free log retrieval operation binding the contract event 0x0afce37c521a4613a2db0c4983987a3c4af722e33d3412963fccbc0eb0df0d28.
//
// Solidity: event matchDeleted(bytes32 arg0)
func (_Tournament *TournamentFilterer) FilterMatchDeleted(opts *bind.FilterOpts) (*TournamentMatchDeletedIterator, error) {

	logs, sub, err := _Tournament.contract.FilterLogs(opts, "matchDeleted")
	if err != nil {
		return nil, err
	}
	return &TournamentMatchDeletedIterator{contract: _Tournament.contract, event: "matchDeleted", logs: logs, sub: sub}, nil
}

// WatchMatchDeleted is a free log subscription operation binding the contract event 0x0afce37c521a4613a2db0c4983987a3c4af722e33d3412963fccbc0eb0df0d28.
//
// Solidity: event matchDeleted(bytes32 arg0)
func (_Tournament *TournamentFilterer) WatchMatchDeleted(opts *bind.WatchOpts, sink chan<- *TournamentMatchDeleted) (event.Subscription, error) {

	logs, sub, err := _Tournament.contract.WatchLogs(opts, "matchDeleted")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TournamentMatchDeleted)
				if err := _Tournament.contract.UnpackLog(event, "matchDeleted", log); err != nil {
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
func (_Tournament *TournamentFilterer) ParseMatchDeleted(log types.Log) (*TournamentMatchDeleted, error) {
	event := new(TournamentMatchDeleted)
	if err := _Tournament.contract.UnpackLog(event, "matchDeleted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
