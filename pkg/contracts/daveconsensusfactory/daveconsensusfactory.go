// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package daveconsensusfactory

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

// DaveConsensusFactoryMetaData contains all meta data concerning the DaveConsensusFactory contract.
var DaveConsensusFactoryMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"_inputBox\",\"type\":\"address\",\"internalType\":\"contractIInputBox\"},{\"name\":\"_tournament\",\"type\":\"address\",\"internalType\":\"contractITournamentFactory\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"calculateDaveConsensusAddress\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"initialMachineStateHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"newDaveConsensus\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"initialMachineStateHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractDaveConsensus\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"newDaveConsensus\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"initialMachineStateHash\",\"type\":\"bytes32\",\"internalType\":\"Machine.Hash\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractDaveConsensus\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"DaveConsensusCreated\",\"inputs\":[{\"name\":\"daveConsensus\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractDaveConsensus\"}],\"anonymous\":false}]",
}

// DaveConsensusFactoryABI is the input ABI used to generate the binding from.
// Deprecated: Use DaveConsensusFactoryMetaData.ABI instead.
var DaveConsensusFactoryABI = DaveConsensusFactoryMetaData.ABI

// DaveConsensusFactory is an auto generated Go binding around an Ethereum contract.
type DaveConsensusFactory struct {
	DaveConsensusFactoryCaller     // Read-only binding to the contract
	DaveConsensusFactoryTransactor // Write-only binding to the contract
	DaveConsensusFactoryFilterer   // Log filterer for contract events
}

// DaveConsensusFactoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type DaveConsensusFactoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DaveConsensusFactoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type DaveConsensusFactoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DaveConsensusFactoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type DaveConsensusFactoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// DaveConsensusFactorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type DaveConsensusFactorySession struct {
	Contract     *DaveConsensusFactory // Generic contract binding to set the session for
	CallOpts     bind.CallOpts         // Call options to use throughout this session
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// DaveConsensusFactoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type DaveConsensusFactoryCallerSession struct {
	Contract *DaveConsensusFactoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts               // Call options to use throughout this session
}

// DaveConsensusFactoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type DaveConsensusFactoryTransactorSession struct {
	Contract     *DaveConsensusFactoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts               // Transaction auth options to use throughout this session
}

// DaveConsensusFactoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type DaveConsensusFactoryRaw struct {
	Contract *DaveConsensusFactory // Generic contract binding to access the raw methods on
}

// DaveConsensusFactoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type DaveConsensusFactoryCallerRaw struct {
	Contract *DaveConsensusFactoryCaller // Generic read-only contract binding to access the raw methods on
}

// DaveConsensusFactoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type DaveConsensusFactoryTransactorRaw struct {
	Contract *DaveConsensusFactoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewDaveConsensusFactory creates a new instance of DaveConsensusFactory, bound to a specific deployed contract.
func NewDaveConsensusFactory(address common.Address, backend bind.ContractBackend) (*DaveConsensusFactory, error) {
	contract, err := bindDaveConsensusFactory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &DaveConsensusFactory{DaveConsensusFactoryCaller: DaveConsensusFactoryCaller{contract: contract}, DaveConsensusFactoryTransactor: DaveConsensusFactoryTransactor{contract: contract}, DaveConsensusFactoryFilterer: DaveConsensusFactoryFilterer{contract: contract}}, nil
}

// NewDaveConsensusFactoryCaller creates a new read-only instance of DaveConsensusFactory, bound to a specific deployed contract.
func NewDaveConsensusFactoryCaller(address common.Address, caller bind.ContractCaller) (*DaveConsensusFactoryCaller, error) {
	contract, err := bindDaveConsensusFactory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &DaveConsensusFactoryCaller{contract: contract}, nil
}

// NewDaveConsensusFactoryTransactor creates a new write-only instance of DaveConsensusFactory, bound to a specific deployed contract.
func NewDaveConsensusFactoryTransactor(address common.Address, transactor bind.ContractTransactor) (*DaveConsensusFactoryTransactor, error) {
	contract, err := bindDaveConsensusFactory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &DaveConsensusFactoryTransactor{contract: contract}, nil
}

// NewDaveConsensusFactoryFilterer creates a new log filterer instance of DaveConsensusFactory, bound to a specific deployed contract.
func NewDaveConsensusFactoryFilterer(address common.Address, filterer bind.ContractFilterer) (*DaveConsensusFactoryFilterer, error) {
	contract, err := bindDaveConsensusFactory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &DaveConsensusFactoryFilterer{contract: contract}, nil
}

// bindDaveConsensusFactory binds a generic wrapper to an already deployed contract.
func bindDaveConsensusFactory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := DaveConsensusFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_DaveConsensusFactory *DaveConsensusFactoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _DaveConsensusFactory.Contract.DaveConsensusFactoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_DaveConsensusFactory *DaveConsensusFactoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DaveConsensusFactory.Contract.DaveConsensusFactoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_DaveConsensusFactory *DaveConsensusFactoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _DaveConsensusFactory.Contract.DaveConsensusFactoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_DaveConsensusFactory *DaveConsensusFactoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _DaveConsensusFactory.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_DaveConsensusFactory *DaveConsensusFactoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _DaveConsensusFactory.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_DaveConsensusFactory *DaveConsensusFactoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _DaveConsensusFactory.Contract.contract.Transact(opts, method, params...)
}

// CalculateDaveConsensusAddress is a free data retrieval call binding the contract method 0x29bda06c.
//
// Solidity: function calculateDaveConsensusAddress(address appContract, bytes32 initialMachineStateHash, bytes32 salt) view returns(address)
func (_DaveConsensusFactory *DaveConsensusFactoryCaller) CalculateDaveConsensusAddress(opts *bind.CallOpts, appContract common.Address, initialMachineStateHash [32]byte, salt [32]byte) (common.Address, error) {
	var out []interface{}
	err := _DaveConsensusFactory.contract.Call(opts, &out, "calculateDaveConsensusAddress", appContract, initialMachineStateHash, salt)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// CalculateDaveConsensusAddress is a free data retrieval call binding the contract method 0x29bda06c.
//
// Solidity: function calculateDaveConsensusAddress(address appContract, bytes32 initialMachineStateHash, bytes32 salt) view returns(address)
func (_DaveConsensusFactory *DaveConsensusFactorySession) CalculateDaveConsensusAddress(appContract common.Address, initialMachineStateHash [32]byte, salt [32]byte) (common.Address, error) {
	return _DaveConsensusFactory.Contract.CalculateDaveConsensusAddress(&_DaveConsensusFactory.CallOpts, appContract, initialMachineStateHash, salt)
}

// CalculateDaveConsensusAddress is a free data retrieval call binding the contract method 0x29bda06c.
//
// Solidity: function calculateDaveConsensusAddress(address appContract, bytes32 initialMachineStateHash, bytes32 salt) view returns(address)
func (_DaveConsensusFactory *DaveConsensusFactoryCallerSession) CalculateDaveConsensusAddress(appContract common.Address, initialMachineStateHash [32]byte, salt [32]byte) (common.Address, error) {
	return _DaveConsensusFactory.Contract.CalculateDaveConsensusAddress(&_DaveConsensusFactory.CallOpts, appContract, initialMachineStateHash, salt)
}

// NewDaveConsensus is a paid mutator transaction binding the contract method 0x13f9004c.
//
// Solidity: function newDaveConsensus(address appContract, bytes32 initialMachineStateHash, bytes32 salt) returns(address)
func (_DaveConsensusFactory *DaveConsensusFactoryTransactor) NewDaveConsensus(opts *bind.TransactOpts, appContract common.Address, initialMachineStateHash [32]byte, salt [32]byte) (*types.Transaction, error) {
	return _DaveConsensusFactory.contract.Transact(opts, "newDaveConsensus", appContract, initialMachineStateHash, salt)
}

// NewDaveConsensus is a paid mutator transaction binding the contract method 0x13f9004c.
//
// Solidity: function newDaveConsensus(address appContract, bytes32 initialMachineStateHash, bytes32 salt) returns(address)
func (_DaveConsensusFactory *DaveConsensusFactorySession) NewDaveConsensus(appContract common.Address, initialMachineStateHash [32]byte, salt [32]byte) (*types.Transaction, error) {
	return _DaveConsensusFactory.Contract.NewDaveConsensus(&_DaveConsensusFactory.TransactOpts, appContract, initialMachineStateHash, salt)
}

// NewDaveConsensus is a paid mutator transaction binding the contract method 0x13f9004c.
//
// Solidity: function newDaveConsensus(address appContract, bytes32 initialMachineStateHash, bytes32 salt) returns(address)
func (_DaveConsensusFactory *DaveConsensusFactoryTransactorSession) NewDaveConsensus(appContract common.Address, initialMachineStateHash [32]byte, salt [32]byte) (*types.Transaction, error) {
	return _DaveConsensusFactory.Contract.NewDaveConsensus(&_DaveConsensusFactory.TransactOpts, appContract, initialMachineStateHash, salt)
}

// NewDaveConsensus0 is a paid mutator transaction binding the contract method 0xbe985100.
//
// Solidity: function newDaveConsensus(address appContract, bytes32 initialMachineStateHash) returns(address)
func (_DaveConsensusFactory *DaveConsensusFactoryTransactor) NewDaveConsensus0(opts *bind.TransactOpts, appContract common.Address, initialMachineStateHash [32]byte) (*types.Transaction, error) {
	return _DaveConsensusFactory.contract.Transact(opts, "newDaveConsensus0", appContract, initialMachineStateHash)
}

// NewDaveConsensus0 is a paid mutator transaction binding the contract method 0xbe985100.
//
// Solidity: function newDaveConsensus(address appContract, bytes32 initialMachineStateHash) returns(address)
func (_DaveConsensusFactory *DaveConsensusFactorySession) NewDaveConsensus0(appContract common.Address, initialMachineStateHash [32]byte) (*types.Transaction, error) {
	return _DaveConsensusFactory.Contract.NewDaveConsensus0(&_DaveConsensusFactory.TransactOpts, appContract, initialMachineStateHash)
}

// NewDaveConsensus0 is a paid mutator transaction binding the contract method 0xbe985100.
//
// Solidity: function newDaveConsensus(address appContract, bytes32 initialMachineStateHash) returns(address)
func (_DaveConsensusFactory *DaveConsensusFactoryTransactorSession) NewDaveConsensus0(appContract common.Address, initialMachineStateHash [32]byte) (*types.Transaction, error) {
	return _DaveConsensusFactory.Contract.NewDaveConsensus0(&_DaveConsensusFactory.TransactOpts, appContract, initialMachineStateHash)
}

// DaveConsensusFactoryDaveConsensusCreatedIterator is returned from FilterDaveConsensusCreated and is used to iterate over the raw logs and unpacked data for DaveConsensusCreated events raised by the DaveConsensusFactory contract.
type DaveConsensusFactoryDaveConsensusCreatedIterator struct {
	Event *DaveConsensusFactoryDaveConsensusCreated // Event containing the contract specifics and raw log

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
func (it *DaveConsensusFactoryDaveConsensusCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(DaveConsensusFactoryDaveConsensusCreated)
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
		it.Event = new(DaveConsensusFactoryDaveConsensusCreated)
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
func (it *DaveConsensusFactoryDaveConsensusCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *DaveConsensusFactoryDaveConsensusCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// DaveConsensusFactoryDaveConsensusCreated represents a DaveConsensusCreated event raised by the DaveConsensusFactory contract.
type DaveConsensusFactoryDaveConsensusCreated struct {
	DaveConsensus common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterDaveConsensusCreated is a free log retrieval operation binding the contract event 0x5b4f8016027b688a75b661b4f1846030bda698d4a8a05575434c4d713ca7f3be.
//
// Solidity: event DaveConsensusCreated(address daveConsensus)
func (_DaveConsensusFactory *DaveConsensusFactoryFilterer) FilterDaveConsensusCreated(opts *bind.FilterOpts) (*DaveConsensusFactoryDaveConsensusCreatedIterator, error) {

	logs, sub, err := _DaveConsensusFactory.contract.FilterLogs(opts, "DaveConsensusCreated")
	if err != nil {
		return nil, err
	}
	return &DaveConsensusFactoryDaveConsensusCreatedIterator{contract: _DaveConsensusFactory.contract, event: "DaveConsensusCreated", logs: logs, sub: sub}, nil
}

// WatchDaveConsensusCreated is a free log subscription operation binding the contract event 0x5b4f8016027b688a75b661b4f1846030bda698d4a8a05575434c4d713ca7f3be.
//
// Solidity: event DaveConsensusCreated(address daveConsensus)
func (_DaveConsensusFactory *DaveConsensusFactoryFilterer) WatchDaveConsensusCreated(opts *bind.WatchOpts, sink chan<- *DaveConsensusFactoryDaveConsensusCreated) (event.Subscription, error) {

	logs, sub, err := _DaveConsensusFactory.contract.WatchLogs(opts, "DaveConsensusCreated")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(DaveConsensusFactoryDaveConsensusCreated)
				if err := _DaveConsensusFactory.contract.UnpackLog(event, "DaveConsensusCreated", log); err != nil {
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

// ParseDaveConsensusCreated is a log parse operation binding the contract event 0x5b4f8016027b688a75b661b4f1846030bda698d4a8a05575434c4d713ca7f3be.
//
// Solidity: event DaveConsensusCreated(address daveConsensus)
func (_DaveConsensusFactory *DaveConsensusFactoryFilterer) ParseDaveConsensusCreated(log types.Log) (*DaveConsensusFactoryDaveConsensusCreated, error) {
	event := new(DaveConsensusFactoryDaveConsensusCreated)
	if err := _DaveConsensusFactory.contract.UnpackLog(event, "DaveConsensusCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
