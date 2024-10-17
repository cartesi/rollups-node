// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package iapplicationfactory

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

// IApplicationFactoryMetaData contains all meta data concerning the IApplicationFactory contract.
var IApplicationFactoryMetaData = &bind.MetaData{
	ABI: "[{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"contractIConsensus\",\"name\":\"consensus\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"appOwner\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"templateHash\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"contractIApplication\",\"name\":\"appContract\",\"type\":\"address\"}],\"name\":\"ApplicationCreated\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"contractIConsensus\",\"name\":\"consensus\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"appOwner\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"templateHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"salt\",\"type\":\"bytes32\"}],\"name\":\"calculateApplicationAddress\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"contractIConsensus\",\"name\":\"consensus\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"appOwner\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"templateHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"salt\",\"type\":\"bytes32\"}],\"name\":\"newApplication\",\"outputs\":[{\"internalType\":\"contractIApplication\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"contractIConsensus\",\"name\":\"consensus\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"appOwner\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"templateHash\",\"type\":\"bytes32\"}],\"name\":\"newApplication\",\"outputs\":[{\"internalType\":\"contractIApplication\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// IApplicationFactoryABI is the input ABI used to generate the binding from.
// Deprecated: Use IApplicationFactoryMetaData.ABI instead.
var IApplicationFactoryABI = IApplicationFactoryMetaData.ABI

// IApplicationFactory is an auto generated Go binding around an Ethereum contract.
type IApplicationFactory struct {
	IApplicationFactoryCaller     // Read-only binding to the contract
	IApplicationFactoryTransactor // Write-only binding to the contract
	IApplicationFactoryFilterer   // Log filterer for contract events
}

// IApplicationFactoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type IApplicationFactoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IApplicationFactoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IApplicationFactoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IApplicationFactoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IApplicationFactoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IApplicationFactorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IApplicationFactorySession struct {
	Contract     *IApplicationFactory // Generic contract binding to set the session for
	CallOpts     bind.CallOpts        // Call options to use throughout this session
	TransactOpts bind.TransactOpts    // Transaction auth options to use throughout this session
}

// IApplicationFactoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IApplicationFactoryCallerSession struct {
	Contract *IApplicationFactoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts              // Call options to use throughout this session
}

// IApplicationFactoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IApplicationFactoryTransactorSession struct {
	Contract     *IApplicationFactoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts              // Transaction auth options to use throughout this session
}

// IApplicationFactoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type IApplicationFactoryRaw struct {
	Contract *IApplicationFactory // Generic contract binding to access the raw methods on
}

// IApplicationFactoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IApplicationFactoryCallerRaw struct {
	Contract *IApplicationFactoryCaller // Generic read-only contract binding to access the raw methods on
}

// IApplicationFactoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IApplicationFactoryTransactorRaw struct {
	Contract *IApplicationFactoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIApplicationFactory creates a new instance of IApplicationFactory, bound to a specific deployed contract.
func NewIApplicationFactory(address common.Address, backend bind.ContractBackend) (*IApplicationFactory, error) {
	contract, err := bindIApplicationFactory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IApplicationFactory{IApplicationFactoryCaller: IApplicationFactoryCaller{contract: contract}, IApplicationFactoryTransactor: IApplicationFactoryTransactor{contract: contract}, IApplicationFactoryFilterer: IApplicationFactoryFilterer{contract: contract}}, nil
}

// NewIApplicationFactoryCaller creates a new read-only instance of IApplicationFactory, bound to a specific deployed contract.
func NewIApplicationFactoryCaller(address common.Address, caller bind.ContractCaller) (*IApplicationFactoryCaller, error) {
	contract, err := bindIApplicationFactory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IApplicationFactoryCaller{contract: contract}, nil
}

// NewIApplicationFactoryTransactor creates a new write-only instance of IApplicationFactory, bound to a specific deployed contract.
func NewIApplicationFactoryTransactor(address common.Address, transactor bind.ContractTransactor) (*IApplicationFactoryTransactor, error) {
	contract, err := bindIApplicationFactory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IApplicationFactoryTransactor{contract: contract}, nil
}

// NewIApplicationFactoryFilterer creates a new log filterer instance of IApplicationFactory, bound to a specific deployed contract.
func NewIApplicationFactoryFilterer(address common.Address, filterer bind.ContractFilterer) (*IApplicationFactoryFilterer, error) {
	contract, err := bindIApplicationFactory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IApplicationFactoryFilterer{contract: contract}, nil
}

// bindIApplicationFactory binds a generic wrapper to an already deployed contract.
func bindIApplicationFactory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IApplicationFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IApplicationFactory *IApplicationFactoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IApplicationFactory.Contract.IApplicationFactoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IApplicationFactory *IApplicationFactoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IApplicationFactory.Contract.IApplicationFactoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IApplicationFactory *IApplicationFactoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IApplicationFactory.Contract.IApplicationFactoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IApplicationFactory *IApplicationFactoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IApplicationFactory.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IApplicationFactory *IApplicationFactoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IApplicationFactory.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IApplicationFactory *IApplicationFactoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IApplicationFactory.Contract.contract.Transact(opts, method, params...)
}

// CalculateApplicationAddress is a free data retrieval call binding the contract method 0xbd4f1219.
//
// Solidity: function calculateApplicationAddress(address consensus, address appOwner, bytes32 templateHash, bytes32 salt) view returns(address)
func (_IApplicationFactory *IApplicationFactoryCaller) CalculateApplicationAddress(opts *bind.CallOpts, consensus common.Address, appOwner common.Address, templateHash [32]byte, salt [32]byte) (common.Address, error) {
	var out []interface{}
	err := _IApplicationFactory.contract.Call(opts, &out, "calculateApplicationAddress", consensus, appOwner, templateHash, salt)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// CalculateApplicationAddress is a free data retrieval call binding the contract method 0xbd4f1219.
//
// Solidity: function calculateApplicationAddress(address consensus, address appOwner, bytes32 templateHash, bytes32 salt) view returns(address)
func (_IApplicationFactory *IApplicationFactorySession) CalculateApplicationAddress(consensus common.Address, appOwner common.Address, templateHash [32]byte, salt [32]byte) (common.Address, error) {
	return _IApplicationFactory.Contract.CalculateApplicationAddress(&_IApplicationFactory.CallOpts, consensus, appOwner, templateHash, salt)
}

// CalculateApplicationAddress is a free data retrieval call binding the contract method 0xbd4f1219.
//
// Solidity: function calculateApplicationAddress(address consensus, address appOwner, bytes32 templateHash, bytes32 salt) view returns(address)
func (_IApplicationFactory *IApplicationFactoryCallerSession) CalculateApplicationAddress(consensus common.Address, appOwner common.Address, templateHash [32]byte, salt [32]byte) (common.Address, error) {
	return _IApplicationFactory.Contract.CalculateApplicationAddress(&_IApplicationFactory.CallOpts, consensus, appOwner, templateHash, salt)
}

// NewApplication is a paid mutator transaction binding the contract method 0x0e1a07f5.
//
// Solidity: function newApplication(address consensus, address appOwner, bytes32 templateHash, bytes32 salt) returns(address)
func (_IApplicationFactory *IApplicationFactoryTransactor) NewApplication(opts *bind.TransactOpts, consensus common.Address, appOwner common.Address, templateHash [32]byte, salt [32]byte) (*types.Transaction, error) {
	return _IApplicationFactory.contract.Transact(opts, "newApplication", consensus, appOwner, templateHash, salt)
}

// NewApplication is a paid mutator transaction binding the contract method 0x0e1a07f5.
//
// Solidity: function newApplication(address consensus, address appOwner, bytes32 templateHash, bytes32 salt) returns(address)
func (_IApplicationFactory *IApplicationFactorySession) NewApplication(consensus common.Address, appOwner common.Address, templateHash [32]byte, salt [32]byte) (*types.Transaction, error) {
	return _IApplicationFactory.Contract.NewApplication(&_IApplicationFactory.TransactOpts, consensus, appOwner, templateHash, salt)
}

// NewApplication is a paid mutator transaction binding the contract method 0x0e1a07f5.
//
// Solidity: function newApplication(address consensus, address appOwner, bytes32 templateHash, bytes32 salt) returns(address)
func (_IApplicationFactory *IApplicationFactoryTransactorSession) NewApplication(consensus common.Address, appOwner common.Address, templateHash [32]byte, salt [32]byte) (*types.Transaction, error) {
	return _IApplicationFactory.Contract.NewApplication(&_IApplicationFactory.TransactOpts, consensus, appOwner, templateHash, salt)
}

// NewApplication0 is a paid mutator transaction binding the contract method 0x3648bfb5.
//
// Solidity: function newApplication(address consensus, address appOwner, bytes32 templateHash) returns(address)
func (_IApplicationFactory *IApplicationFactoryTransactor) NewApplication0(opts *bind.TransactOpts, consensus common.Address, appOwner common.Address, templateHash [32]byte) (*types.Transaction, error) {
	return _IApplicationFactory.contract.Transact(opts, "newApplication0", consensus, appOwner, templateHash)
}

// NewApplication0 is a paid mutator transaction binding the contract method 0x3648bfb5.
//
// Solidity: function newApplication(address consensus, address appOwner, bytes32 templateHash) returns(address)
func (_IApplicationFactory *IApplicationFactorySession) NewApplication0(consensus common.Address, appOwner common.Address, templateHash [32]byte) (*types.Transaction, error) {
	return _IApplicationFactory.Contract.NewApplication0(&_IApplicationFactory.TransactOpts, consensus, appOwner, templateHash)
}

// NewApplication0 is a paid mutator transaction binding the contract method 0x3648bfb5.
//
// Solidity: function newApplication(address consensus, address appOwner, bytes32 templateHash) returns(address)
func (_IApplicationFactory *IApplicationFactoryTransactorSession) NewApplication0(consensus common.Address, appOwner common.Address, templateHash [32]byte) (*types.Transaction, error) {
	return _IApplicationFactory.Contract.NewApplication0(&_IApplicationFactory.TransactOpts, consensus, appOwner, templateHash)
}

// IApplicationFactoryApplicationCreatedIterator is returned from FilterApplicationCreated and is used to iterate over the raw logs and unpacked data for ApplicationCreated events raised by the IApplicationFactory contract.
type IApplicationFactoryApplicationCreatedIterator struct {
	Event *IApplicationFactoryApplicationCreated // Event containing the contract specifics and raw log

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
func (it *IApplicationFactoryApplicationCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IApplicationFactoryApplicationCreated)
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
		it.Event = new(IApplicationFactoryApplicationCreated)
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
func (it *IApplicationFactoryApplicationCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IApplicationFactoryApplicationCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IApplicationFactoryApplicationCreated represents a ApplicationCreated event raised by the IApplicationFactory contract.
type IApplicationFactoryApplicationCreated struct {
	Consensus    common.Address
	AppOwner     common.Address
	TemplateHash [32]byte
	AppContract  common.Address
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterApplicationCreated is a free log retrieval operation binding the contract event 0xe73165c2d277daf8713fd08b40845cb6bb7a20b2b543f3d35324a475660fcebd.
//
// Solidity: event ApplicationCreated(address indexed consensus, address appOwner, bytes32 templateHash, address appContract)
func (_IApplicationFactory *IApplicationFactoryFilterer) FilterApplicationCreated(opts *bind.FilterOpts, consensus []common.Address) (*IApplicationFactoryApplicationCreatedIterator, error) {

	var consensusRule []interface{}
	for _, consensusItem := range consensus {
		consensusRule = append(consensusRule, consensusItem)
	}

	logs, sub, err := _IApplicationFactory.contract.FilterLogs(opts, "ApplicationCreated", consensusRule)
	if err != nil {
		return nil, err
	}
	return &IApplicationFactoryApplicationCreatedIterator{contract: _IApplicationFactory.contract, event: "ApplicationCreated", logs: logs, sub: sub}, nil
}

// WatchApplicationCreated is a free log subscription operation binding the contract event 0xe73165c2d277daf8713fd08b40845cb6bb7a20b2b543f3d35324a475660fcebd.
//
// Solidity: event ApplicationCreated(address indexed consensus, address appOwner, bytes32 templateHash, address appContract)
func (_IApplicationFactory *IApplicationFactoryFilterer) WatchApplicationCreated(opts *bind.WatchOpts, sink chan<- *IApplicationFactoryApplicationCreated, consensus []common.Address) (event.Subscription, error) {

	var consensusRule []interface{}
	for _, consensusItem := range consensus {
		consensusRule = append(consensusRule, consensusItem)
	}

	logs, sub, err := _IApplicationFactory.contract.WatchLogs(opts, "ApplicationCreated", consensusRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IApplicationFactoryApplicationCreated)
				if err := _IApplicationFactory.contract.UnpackLog(event, "ApplicationCreated", log); err != nil {
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

// ParseApplicationCreated is a log parse operation binding the contract event 0xe73165c2d277daf8713fd08b40845cb6bb7a20b2b543f3d35324a475660fcebd.
//
// Solidity: event ApplicationCreated(address indexed consensus, address appOwner, bytes32 templateHash, address appContract)
func (_IApplicationFactory *IApplicationFactoryFilterer) ParseApplicationCreated(log types.Log) (*IApplicationFactoryApplicationCreated, error) {
	event := new(IApplicationFactoryApplicationCreated)
	if err := _IApplicationFactory.contract.UnpackLog(event, "ApplicationCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
