// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package imultileveltournamentfactory

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

// IMultiLevelTournamentFactoryMetaData contains all meta data concerning the IMultiLevelTournamentFactory contract.
var IMultiLevelTournamentFactoryMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"instantiate\",\"inputs\":[{\"name\":\"initialState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"provider\",\"type\":\"address\",\"internalType\":\"contractIDataProvider\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractITournament\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"instantiateBottom\",\"inputs\":[{\"name\":\"_initialHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"_contestedCommitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_contestedFinalStateOne\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"_contestedCommitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_contestedFinalStateTwo\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"_allowance\",\"type\":\"uint64\",\"internalType\":\"Time.Duration\"},{\"name\":\"_startCycle\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"_level\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_provider\",\"type\":\"address\",\"internalType\":\"contractIDataProvider\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractTournament\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"instantiateMiddle\",\"inputs\":[{\"name\":\"_initialHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"_contestedCommitmentOne\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_contestedFinalStateOne\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"_contestedCommitmentTwo\",\"type\":\"bytes32\",\"internalType\":\"Tree.Node\"},{\"name\":\"_contestedFinalStateTwo\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"_allowance\",\"type\":\"uint64\",\"internalType\":\"Time.Duration\"},{\"name\":\"_startCycle\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"_level\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_provider\",\"type\":\"address\",\"internalType\":\"contractIDataProvider\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractTournament\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"instantiateTop\",\"inputs\":[{\"name\":\"_initialHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"_provider\",\"type\":\"address\",\"internalType\":\"contractIDataProvider\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractTournament\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"tournamentCreated\",\"inputs\":[{\"name\":\"tournament\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractITournament\"}],\"anonymous\":false}]",
}

// IMultiLevelTournamentFactoryABI is the input ABI used to generate the binding from.
// Deprecated: Use IMultiLevelTournamentFactoryMetaData.ABI instead.
var IMultiLevelTournamentFactoryABI = IMultiLevelTournamentFactoryMetaData.ABI

// IMultiLevelTournamentFactory is an auto generated Go binding around an Ethereum contract.
type IMultiLevelTournamentFactory struct {
	IMultiLevelTournamentFactoryCaller     // Read-only binding to the contract
	IMultiLevelTournamentFactoryTransactor // Write-only binding to the contract
	IMultiLevelTournamentFactoryFilterer   // Log filterer for contract events
}

// IMultiLevelTournamentFactoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type IMultiLevelTournamentFactoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IMultiLevelTournamentFactoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IMultiLevelTournamentFactoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IMultiLevelTournamentFactoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IMultiLevelTournamentFactoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IMultiLevelTournamentFactorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IMultiLevelTournamentFactorySession struct {
	Contract     *IMultiLevelTournamentFactory // Generic contract binding to set the session for
	CallOpts     bind.CallOpts                 // Call options to use throughout this session
	TransactOpts bind.TransactOpts             // Transaction auth options to use throughout this session
}

// IMultiLevelTournamentFactoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IMultiLevelTournamentFactoryCallerSession struct {
	Contract *IMultiLevelTournamentFactoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                       // Call options to use throughout this session
}

// IMultiLevelTournamentFactoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IMultiLevelTournamentFactoryTransactorSession struct {
	Contract     *IMultiLevelTournamentFactoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                       // Transaction auth options to use throughout this session
}

// IMultiLevelTournamentFactoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type IMultiLevelTournamentFactoryRaw struct {
	Contract *IMultiLevelTournamentFactory // Generic contract binding to access the raw methods on
}

// IMultiLevelTournamentFactoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IMultiLevelTournamentFactoryCallerRaw struct {
	Contract *IMultiLevelTournamentFactoryCaller // Generic read-only contract binding to access the raw methods on
}

// IMultiLevelTournamentFactoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IMultiLevelTournamentFactoryTransactorRaw struct {
	Contract *IMultiLevelTournamentFactoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIMultiLevelTournamentFactory creates a new instance of IMultiLevelTournamentFactory, bound to a specific deployed contract.
func NewIMultiLevelTournamentFactory(address common.Address, backend bind.ContractBackend) (*IMultiLevelTournamentFactory, error) {
	contract, err := bindIMultiLevelTournamentFactory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IMultiLevelTournamentFactory{IMultiLevelTournamentFactoryCaller: IMultiLevelTournamentFactoryCaller{contract: contract}, IMultiLevelTournamentFactoryTransactor: IMultiLevelTournamentFactoryTransactor{contract: contract}, IMultiLevelTournamentFactoryFilterer: IMultiLevelTournamentFactoryFilterer{contract: contract}}, nil
}

// NewIMultiLevelTournamentFactoryCaller creates a new read-only instance of IMultiLevelTournamentFactory, bound to a specific deployed contract.
func NewIMultiLevelTournamentFactoryCaller(address common.Address, caller bind.ContractCaller) (*IMultiLevelTournamentFactoryCaller, error) {
	contract, err := bindIMultiLevelTournamentFactory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IMultiLevelTournamentFactoryCaller{contract: contract}, nil
}

// NewIMultiLevelTournamentFactoryTransactor creates a new write-only instance of IMultiLevelTournamentFactory, bound to a specific deployed contract.
func NewIMultiLevelTournamentFactoryTransactor(address common.Address, transactor bind.ContractTransactor) (*IMultiLevelTournamentFactoryTransactor, error) {
	contract, err := bindIMultiLevelTournamentFactory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IMultiLevelTournamentFactoryTransactor{contract: contract}, nil
}

// NewIMultiLevelTournamentFactoryFilterer creates a new log filterer instance of IMultiLevelTournamentFactory, bound to a specific deployed contract.
func NewIMultiLevelTournamentFactoryFilterer(address common.Address, filterer bind.ContractFilterer) (*IMultiLevelTournamentFactoryFilterer, error) {
	contract, err := bindIMultiLevelTournamentFactory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IMultiLevelTournamentFactoryFilterer{contract: contract}, nil
}

// bindIMultiLevelTournamentFactory binds a generic wrapper to an already deployed contract.
func bindIMultiLevelTournamentFactory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IMultiLevelTournamentFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IMultiLevelTournamentFactory.Contract.IMultiLevelTournamentFactoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.Contract.IMultiLevelTournamentFactoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.Contract.IMultiLevelTournamentFactoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IMultiLevelTournamentFactory.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.Contract.contract.Transact(opts, method, params...)
}

// Instantiate is a paid mutator transaction binding the contract method 0x0b64d79b.
//
// Solidity: function instantiate(bytes32 initialState, address provider) returns(address)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryTransactor) Instantiate(opts *bind.TransactOpts, initialState [32]byte, provider common.Address) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.contract.Transact(opts, "instantiate", initialState, provider)
}

// Instantiate is a paid mutator transaction binding the contract method 0x0b64d79b.
//
// Solidity: function instantiate(bytes32 initialState, address provider) returns(address)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactorySession) Instantiate(initialState [32]byte, provider common.Address) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.Contract.Instantiate(&_IMultiLevelTournamentFactory.TransactOpts, initialState, provider)
}

// Instantiate is a paid mutator transaction binding the contract method 0x0b64d79b.
//
// Solidity: function instantiate(bytes32 initialState, address provider) returns(address)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryTransactorSession) Instantiate(initialState [32]byte, provider common.Address) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.Contract.Instantiate(&_IMultiLevelTournamentFactory.TransactOpts, initialState, provider)
}

// InstantiateBottom is a paid mutator transaction binding the contract method 0x18cf0e9a.
//
// Solidity: function instantiateBottom(bytes32 _initialHash, bytes32 _contestedCommitmentOne, bytes32 _contestedFinalStateOne, bytes32 _contestedCommitmentTwo, bytes32 _contestedFinalStateTwo, uint64 _allowance, uint256 _startCycle, uint64 _level, address _provider) returns(address)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryTransactor) InstantiateBottom(opts *bind.TransactOpts, _initialHash [32]byte, _contestedCommitmentOne [32]byte, _contestedFinalStateOne [32]byte, _contestedCommitmentTwo [32]byte, _contestedFinalStateTwo [32]byte, _allowance uint64, _startCycle *big.Int, _level uint64, _provider common.Address) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.contract.Transact(opts, "instantiateBottom", _initialHash, _contestedCommitmentOne, _contestedFinalStateOne, _contestedCommitmentTwo, _contestedFinalStateTwo, _allowance, _startCycle, _level, _provider)
}

// InstantiateBottom is a paid mutator transaction binding the contract method 0x18cf0e9a.
//
// Solidity: function instantiateBottom(bytes32 _initialHash, bytes32 _contestedCommitmentOne, bytes32 _contestedFinalStateOne, bytes32 _contestedCommitmentTwo, bytes32 _contestedFinalStateTwo, uint64 _allowance, uint256 _startCycle, uint64 _level, address _provider) returns(address)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactorySession) InstantiateBottom(_initialHash [32]byte, _contestedCommitmentOne [32]byte, _contestedFinalStateOne [32]byte, _contestedCommitmentTwo [32]byte, _contestedFinalStateTwo [32]byte, _allowance uint64, _startCycle *big.Int, _level uint64, _provider common.Address) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.Contract.InstantiateBottom(&_IMultiLevelTournamentFactory.TransactOpts, _initialHash, _contestedCommitmentOne, _contestedFinalStateOne, _contestedCommitmentTwo, _contestedFinalStateTwo, _allowance, _startCycle, _level, _provider)
}

// InstantiateBottom is a paid mutator transaction binding the contract method 0x18cf0e9a.
//
// Solidity: function instantiateBottom(bytes32 _initialHash, bytes32 _contestedCommitmentOne, bytes32 _contestedFinalStateOne, bytes32 _contestedCommitmentTwo, bytes32 _contestedFinalStateTwo, uint64 _allowance, uint256 _startCycle, uint64 _level, address _provider) returns(address)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryTransactorSession) InstantiateBottom(_initialHash [32]byte, _contestedCommitmentOne [32]byte, _contestedFinalStateOne [32]byte, _contestedCommitmentTwo [32]byte, _contestedFinalStateTwo [32]byte, _allowance uint64, _startCycle *big.Int, _level uint64, _provider common.Address) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.Contract.InstantiateBottom(&_IMultiLevelTournamentFactory.TransactOpts, _initialHash, _contestedCommitmentOne, _contestedFinalStateOne, _contestedCommitmentTwo, _contestedFinalStateTwo, _allowance, _startCycle, _level, _provider)
}

// InstantiateMiddle is a paid mutator transaction binding the contract method 0xcb0f2e58.
//
// Solidity: function instantiateMiddle(bytes32 _initialHash, bytes32 _contestedCommitmentOne, bytes32 _contestedFinalStateOne, bytes32 _contestedCommitmentTwo, bytes32 _contestedFinalStateTwo, uint64 _allowance, uint256 _startCycle, uint64 _level, address _provider) returns(address)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryTransactor) InstantiateMiddle(opts *bind.TransactOpts, _initialHash [32]byte, _contestedCommitmentOne [32]byte, _contestedFinalStateOne [32]byte, _contestedCommitmentTwo [32]byte, _contestedFinalStateTwo [32]byte, _allowance uint64, _startCycle *big.Int, _level uint64, _provider common.Address) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.contract.Transact(opts, "instantiateMiddle", _initialHash, _contestedCommitmentOne, _contestedFinalStateOne, _contestedCommitmentTwo, _contestedFinalStateTwo, _allowance, _startCycle, _level, _provider)
}

// InstantiateMiddle is a paid mutator transaction binding the contract method 0xcb0f2e58.
//
// Solidity: function instantiateMiddle(bytes32 _initialHash, bytes32 _contestedCommitmentOne, bytes32 _contestedFinalStateOne, bytes32 _contestedCommitmentTwo, bytes32 _contestedFinalStateTwo, uint64 _allowance, uint256 _startCycle, uint64 _level, address _provider) returns(address)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactorySession) InstantiateMiddle(_initialHash [32]byte, _contestedCommitmentOne [32]byte, _contestedFinalStateOne [32]byte, _contestedCommitmentTwo [32]byte, _contestedFinalStateTwo [32]byte, _allowance uint64, _startCycle *big.Int, _level uint64, _provider common.Address) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.Contract.InstantiateMiddle(&_IMultiLevelTournamentFactory.TransactOpts, _initialHash, _contestedCommitmentOne, _contestedFinalStateOne, _contestedCommitmentTwo, _contestedFinalStateTwo, _allowance, _startCycle, _level, _provider)
}

// InstantiateMiddle is a paid mutator transaction binding the contract method 0xcb0f2e58.
//
// Solidity: function instantiateMiddle(bytes32 _initialHash, bytes32 _contestedCommitmentOne, bytes32 _contestedFinalStateOne, bytes32 _contestedCommitmentTwo, bytes32 _contestedFinalStateTwo, uint64 _allowance, uint256 _startCycle, uint64 _level, address _provider) returns(address)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryTransactorSession) InstantiateMiddle(_initialHash [32]byte, _contestedCommitmentOne [32]byte, _contestedFinalStateOne [32]byte, _contestedCommitmentTwo [32]byte, _contestedFinalStateTwo [32]byte, _allowance uint64, _startCycle *big.Int, _level uint64, _provider common.Address) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.Contract.InstantiateMiddle(&_IMultiLevelTournamentFactory.TransactOpts, _initialHash, _contestedCommitmentOne, _contestedFinalStateOne, _contestedCommitmentTwo, _contestedFinalStateTwo, _allowance, _startCycle, _level, _provider)
}

// InstantiateTop is a paid mutator transaction binding the contract method 0x70cabc6e.
//
// Solidity: function instantiateTop(bytes32 _initialHash, address _provider) returns(address)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryTransactor) InstantiateTop(opts *bind.TransactOpts, _initialHash [32]byte, _provider common.Address) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.contract.Transact(opts, "instantiateTop", _initialHash, _provider)
}

// InstantiateTop is a paid mutator transaction binding the contract method 0x70cabc6e.
//
// Solidity: function instantiateTop(bytes32 _initialHash, address _provider) returns(address)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactorySession) InstantiateTop(_initialHash [32]byte, _provider common.Address) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.Contract.InstantiateTop(&_IMultiLevelTournamentFactory.TransactOpts, _initialHash, _provider)
}

// InstantiateTop is a paid mutator transaction binding the contract method 0x70cabc6e.
//
// Solidity: function instantiateTop(bytes32 _initialHash, address _provider) returns(address)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryTransactorSession) InstantiateTop(_initialHash [32]byte, _provider common.Address) (*types.Transaction, error) {
	return _IMultiLevelTournamentFactory.Contract.InstantiateTop(&_IMultiLevelTournamentFactory.TransactOpts, _initialHash, _provider)
}

// IMultiLevelTournamentFactoryTournamentCreatedIterator is returned from FilterTournamentCreated and is used to iterate over the raw logs and unpacked data for TournamentCreated events raised by the IMultiLevelTournamentFactory contract.
type IMultiLevelTournamentFactoryTournamentCreatedIterator struct {
	Event *IMultiLevelTournamentFactoryTournamentCreated // Event containing the contract specifics and raw log

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
func (it *IMultiLevelTournamentFactoryTournamentCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IMultiLevelTournamentFactoryTournamentCreated)
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
		it.Event = new(IMultiLevelTournamentFactoryTournamentCreated)
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
func (it *IMultiLevelTournamentFactoryTournamentCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IMultiLevelTournamentFactoryTournamentCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IMultiLevelTournamentFactoryTournamentCreated represents a TournamentCreated event raised by the IMultiLevelTournamentFactory contract.
type IMultiLevelTournamentFactoryTournamentCreated struct {
	Tournament common.Address
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterTournamentCreated is a free log retrieval operation binding the contract event 0x68952387ba736c9928265c63b28112a625425d9dfbe48705686ea5bed1f92efb.
//
// Solidity: event tournamentCreated(address tournament)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryFilterer) FilterTournamentCreated(opts *bind.FilterOpts) (*IMultiLevelTournamentFactoryTournamentCreatedIterator, error) {

	logs, sub, err := _IMultiLevelTournamentFactory.contract.FilterLogs(opts, "tournamentCreated")
	if err != nil {
		return nil, err
	}
	return &IMultiLevelTournamentFactoryTournamentCreatedIterator{contract: _IMultiLevelTournamentFactory.contract, event: "tournamentCreated", logs: logs, sub: sub}, nil
}

// WatchTournamentCreated is a free log subscription operation binding the contract event 0x68952387ba736c9928265c63b28112a625425d9dfbe48705686ea5bed1f92efb.
//
// Solidity: event tournamentCreated(address tournament)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryFilterer) WatchTournamentCreated(opts *bind.WatchOpts, sink chan<- *IMultiLevelTournamentFactoryTournamentCreated) (event.Subscription, error) {

	logs, sub, err := _IMultiLevelTournamentFactory.contract.WatchLogs(opts, "tournamentCreated")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IMultiLevelTournamentFactoryTournamentCreated)
				if err := _IMultiLevelTournamentFactory.contract.UnpackLog(event, "tournamentCreated", log); err != nil {
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

// ParseTournamentCreated is a log parse operation binding the contract event 0x68952387ba736c9928265c63b28112a625425d9dfbe48705686ea5bed1f92efb.
//
// Solidity: event tournamentCreated(address tournament)
func (_IMultiLevelTournamentFactory *IMultiLevelTournamentFactoryFilterer) ParseTournamentCreated(log types.Log) (*IMultiLevelTournamentFactoryTournamentCreated, error) {
	event := new(IMultiLevelTournamentFactoryTournamentCreated)
	if err := _IMultiLevelTournamentFactory.contract.UnpackLog(event, "tournamentCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
