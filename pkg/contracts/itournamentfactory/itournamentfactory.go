// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package itournamentfactory

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

// ITournamentFactoryMetaData contains all meta data concerning the ITournamentFactory contract.
var ITournamentFactoryMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"instantiate\",\"inputs\":[{\"name\":\"initialState\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"provider\",\"type\":\"address\",\"internalType\":\"contractIDataProvider\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractITournament\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"tournamentCreated\",\"inputs\":[{\"name\":\"tournament\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractITournament\"}],\"anonymous\":false}]",
}

// ITournamentFactoryABI is the input ABI used to generate the binding from.
// Deprecated: Use ITournamentFactoryMetaData.ABI instead.
var ITournamentFactoryABI = ITournamentFactoryMetaData.ABI

// ITournamentFactory is an auto generated Go binding around an Ethereum contract.
type ITournamentFactory struct {
	ITournamentFactoryCaller     // Read-only binding to the contract
	ITournamentFactoryTransactor // Write-only binding to the contract
	ITournamentFactoryFilterer   // Log filterer for contract events
}

// ITournamentFactoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type ITournamentFactoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ITournamentFactoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ITournamentFactoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ITournamentFactoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ITournamentFactoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ITournamentFactorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ITournamentFactorySession struct {
	Contract     *ITournamentFactory // Generic contract binding to set the session for
	CallOpts     bind.CallOpts       // Call options to use throughout this session
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// ITournamentFactoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ITournamentFactoryCallerSession struct {
	Contract *ITournamentFactoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts             // Call options to use throughout this session
}

// ITournamentFactoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ITournamentFactoryTransactorSession struct {
	Contract     *ITournamentFactoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts             // Transaction auth options to use throughout this session
}

// ITournamentFactoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type ITournamentFactoryRaw struct {
	Contract *ITournamentFactory // Generic contract binding to access the raw methods on
}

// ITournamentFactoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ITournamentFactoryCallerRaw struct {
	Contract *ITournamentFactoryCaller // Generic read-only contract binding to access the raw methods on
}

// ITournamentFactoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ITournamentFactoryTransactorRaw struct {
	Contract *ITournamentFactoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewITournamentFactory creates a new instance of ITournamentFactory, bound to a specific deployed contract.
func NewITournamentFactory(address common.Address, backend bind.ContractBackend) (*ITournamentFactory, error) {
	contract, err := bindITournamentFactory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &ITournamentFactory{ITournamentFactoryCaller: ITournamentFactoryCaller{contract: contract}, ITournamentFactoryTransactor: ITournamentFactoryTransactor{contract: contract}, ITournamentFactoryFilterer: ITournamentFactoryFilterer{contract: contract}}, nil
}

// NewITournamentFactoryCaller creates a new read-only instance of ITournamentFactory, bound to a specific deployed contract.
func NewITournamentFactoryCaller(address common.Address, caller bind.ContractCaller) (*ITournamentFactoryCaller, error) {
	contract, err := bindITournamentFactory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ITournamentFactoryCaller{contract: contract}, nil
}

// NewITournamentFactoryTransactor creates a new write-only instance of ITournamentFactory, bound to a specific deployed contract.
func NewITournamentFactoryTransactor(address common.Address, transactor bind.ContractTransactor) (*ITournamentFactoryTransactor, error) {
	contract, err := bindITournamentFactory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ITournamentFactoryTransactor{contract: contract}, nil
}

// NewITournamentFactoryFilterer creates a new log filterer instance of ITournamentFactory, bound to a specific deployed contract.
func NewITournamentFactoryFilterer(address common.Address, filterer bind.ContractFilterer) (*ITournamentFactoryFilterer, error) {
	contract, err := bindITournamentFactory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ITournamentFactoryFilterer{contract: contract}, nil
}

// bindITournamentFactory binds a generic wrapper to an already deployed contract.
func bindITournamentFactory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := ITournamentFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ITournamentFactory *ITournamentFactoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ITournamentFactory.Contract.ITournamentFactoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ITournamentFactory *ITournamentFactoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ITournamentFactory.Contract.ITournamentFactoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ITournamentFactory *ITournamentFactoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ITournamentFactory.Contract.ITournamentFactoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ITournamentFactory *ITournamentFactoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ITournamentFactory.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ITournamentFactory *ITournamentFactoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ITournamentFactory.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ITournamentFactory *ITournamentFactoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ITournamentFactory.Contract.contract.Transact(opts, method, params...)
}

// Instantiate is a paid mutator transaction binding the contract method 0x0b64d79b.
//
// Solidity: function instantiate(bytes32 initialState, address provider) returns(address)
func (_ITournamentFactory *ITournamentFactoryTransactor) Instantiate(opts *bind.TransactOpts, initialState [32]byte, provider common.Address) (*types.Transaction, error) {
	return _ITournamentFactory.contract.Transact(opts, "instantiate", initialState, provider)
}

// Instantiate is a paid mutator transaction binding the contract method 0x0b64d79b.
//
// Solidity: function instantiate(bytes32 initialState, address provider) returns(address)
func (_ITournamentFactory *ITournamentFactorySession) Instantiate(initialState [32]byte, provider common.Address) (*types.Transaction, error) {
	return _ITournamentFactory.Contract.Instantiate(&_ITournamentFactory.TransactOpts, initialState, provider)
}

// Instantiate is a paid mutator transaction binding the contract method 0x0b64d79b.
//
// Solidity: function instantiate(bytes32 initialState, address provider) returns(address)
func (_ITournamentFactory *ITournamentFactoryTransactorSession) Instantiate(initialState [32]byte, provider common.Address) (*types.Transaction, error) {
	return _ITournamentFactory.Contract.Instantiate(&_ITournamentFactory.TransactOpts, initialState, provider)
}

// ITournamentFactoryTournamentCreatedIterator is returned from FilterTournamentCreated and is used to iterate over the raw logs and unpacked data for TournamentCreated events raised by the ITournamentFactory contract.
type ITournamentFactoryTournamentCreatedIterator struct {
	Event *ITournamentFactoryTournamentCreated // Event containing the contract specifics and raw log

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
func (it *ITournamentFactoryTournamentCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ITournamentFactoryTournamentCreated)
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
		it.Event = new(ITournamentFactoryTournamentCreated)
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
func (it *ITournamentFactoryTournamentCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ITournamentFactoryTournamentCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ITournamentFactoryTournamentCreated represents a TournamentCreated event raised by the ITournamentFactory contract.
type ITournamentFactoryTournamentCreated struct {
	Tournament common.Address
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterTournamentCreated is a free log retrieval operation binding the contract event 0x68952387ba736c9928265c63b28112a625425d9dfbe48705686ea5bed1f92efb.
//
// Solidity: event tournamentCreated(address tournament)
func (_ITournamentFactory *ITournamentFactoryFilterer) FilterTournamentCreated(opts *bind.FilterOpts) (*ITournamentFactoryTournamentCreatedIterator, error) {

	logs, sub, err := _ITournamentFactory.contract.FilterLogs(opts, "tournamentCreated")
	if err != nil {
		return nil, err
	}
	return &ITournamentFactoryTournamentCreatedIterator{contract: _ITournamentFactory.contract, event: "tournamentCreated", logs: logs, sub: sub}, nil
}

// WatchTournamentCreated is a free log subscription operation binding the contract event 0x68952387ba736c9928265c63b28112a625425d9dfbe48705686ea5bed1f92efb.
//
// Solidity: event tournamentCreated(address tournament)
func (_ITournamentFactory *ITournamentFactoryFilterer) WatchTournamentCreated(opts *bind.WatchOpts, sink chan<- *ITournamentFactoryTournamentCreated) (event.Subscription, error) {

	logs, sub, err := _ITournamentFactory.contract.WatchLogs(opts, "tournamentCreated")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ITournamentFactoryTournamentCreated)
				if err := _ITournamentFactory.contract.UnpackLog(event, "tournamentCreated", log); err != nil {
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
func (_ITournamentFactory *ITournamentFactoryFilterer) ParseTournamentCreated(log types.Log) (*ITournamentFactoryTournamentCreated, error) {
	event := new(ITournamentFactoryTournamentCreated)
	if err := _ITournamentFactory.contract.UnpackLog(event, "tournamentCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
