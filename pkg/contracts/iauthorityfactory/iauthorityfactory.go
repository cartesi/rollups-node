// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package iauthorityfactory

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

// IAuthorityFactoryMetaData contains all meta data concerning the IAuthorityFactory contract.
var IAuthorityFactoryMetaData = &bind.MetaData{
	ABI: "[{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"contractIAuthority\",\"name\":\"authority\",\"type\":\"address\"}],\"name\":\"AuthorityCreated\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"authorityOwner\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"epochLength\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"salt\",\"type\":\"bytes32\"}],\"name\":\"calculateAuthorityAddress\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"authorityOwner\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"epochLength\",\"type\":\"uint256\"}],\"name\":\"newAuthority\",\"outputs\":[{\"internalType\":\"contractIAuthority\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"authorityOwner\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"epochLength\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"salt\",\"type\":\"bytes32\"}],\"name\":\"newAuthority\",\"outputs\":[{\"internalType\":\"contractIAuthority\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// IAuthorityFactoryABI is the input ABI used to generate the binding from.
// Deprecated: Use IAuthorityFactoryMetaData.ABI instead.
var IAuthorityFactoryABI = IAuthorityFactoryMetaData.ABI

// IAuthorityFactory is an auto generated Go binding around an Ethereum contract.
type IAuthorityFactory struct {
	IAuthorityFactoryCaller     // Read-only binding to the contract
	IAuthorityFactoryTransactor // Write-only binding to the contract
	IAuthorityFactoryFilterer   // Log filterer for contract events
}

// IAuthorityFactoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type IAuthorityFactoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IAuthorityFactoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IAuthorityFactoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IAuthorityFactoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IAuthorityFactoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IAuthorityFactorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IAuthorityFactorySession struct {
	Contract     *IAuthorityFactory // Generic contract binding to set the session for
	CallOpts     bind.CallOpts      // Call options to use throughout this session
	TransactOpts bind.TransactOpts  // Transaction auth options to use throughout this session
}

// IAuthorityFactoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IAuthorityFactoryCallerSession struct {
	Contract *IAuthorityFactoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts            // Call options to use throughout this session
}

// IAuthorityFactoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IAuthorityFactoryTransactorSession struct {
	Contract     *IAuthorityFactoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts            // Transaction auth options to use throughout this session
}

// IAuthorityFactoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type IAuthorityFactoryRaw struct {
	Contract *IAuthorityFactory // Generic contract binding to access the raw methods on
}

// IAuthorityFactoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IAuthorityFactoryCallerRaw struct {
	Contract *IAuthorityFactoryCaller // Generic read-only contract binding to access the raw methods on
}

// IAuthorityFactoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IAuthorityFactoryTransactorRaw struct {
	Contract *IAuthorityFactoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIAuthorityFactory creates a new instance of IAuthorityFactory, bound to a specific deployed contract.
func NewIAuthorityFactory(address common.Address, backend bind.ContractBackend) (*IAuthorityFactory, error) {
	contract, err := bindIAuthorityFactory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IAuthorityFactory{IAuthorityFactoryCaller: IAuthorityFactoryCaller{contract: contract}, IAuthorityFactoryTransactor: IAuthorityFactoryTransactor{contract: contract}, IAuthorityFactoryFilterer: IAuthorityFactoryFilterer{contract: contract}}, nil
}

// NewIAuthorityFactoryCaller creates a new read-only instance of IAuthorityFactory, bound to a specific deployed contract.
func NewIAuthorityFactoryCaller(address common.Address, caller bind.ContractCaller) (*IAuthorityFactoryCaller, error) {
	contract, err := bindIAuthorityFactory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IAuthorityFactoryCaller{contract: contract}, nil
}

// NewIAuthorityFactoryTransactor creates a new write-only instance of IAuthorityFactory, bound to a specific deployed contract.
func NewIAuthorityFactoryTransactor(address common.Address, transactor bind.ContractTransactor) (*IAuthorityFactoryTransactor, error) {
	contract, err := bindIAuthorityFactory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IAuthorityFactoryTransactor{contract: contract}, nil
}

// NewIAuthorityFactoryFilterer creates a new log filterer instance of IAuthorityFactory, bound to a specific deployed contract.
func NewIAuthorityFactoryFilterer(address common.Address, filterer bind.ContractFilterer) (*IAuthorityFactoryFilterer, error) {
	contract, err := bindIAuthorityFactory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IAuthorityFactoryFilterer{contract: contract}, nil
}

// bindIAuthorityFactory binds a generic wrapper to an already deployed contract.
func bindIAuthorityFactory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IAuthorityFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IAuthorityFactory *IAuthorityFactoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IAuthorityFactory.Contract.IAuthorityFactoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IAuthorityFactory *IAuthorityFactoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IAuthorityFactory.Contract.IAuthorityFactoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IAuthorityFactory *IAuthorityFactoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IAuthorityFactory.Contract.IAuthorityFactoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IAuthorityFactory *IAuthorityFactoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IAuthorityFactory.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IAuthorityFactory *IAuthorityFactoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IAuthorityFactory.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IAuthorityFactory *IAuthorityFactoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IAuthorityFactory.Contract.contract.Transact(opts, method, params...)
}

// CalculateAuthorityAddress is a free data retrieval call binding the contract method 0x1442f7bb.
//
// Solidity: function calculateAuthorityAddress(address authorityOwner, uint256 epochLength, bytes32 salt) view returns(address)
func (_IAuthorityFactory *IAuthorityFactoryCaller) CalculateAuthorityAddress(opts *bind.CallOpts, authorityOwner common.Address, epochLength *big.Int, salt [32]byte) (common.Address, error) {
	var out []interface{}
	err := _IAuthorityFactory.contract.Call(opts, &out, "calculateAuthorityAddress", authorityOwner, epochLength, salt)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// CalculateAuthorityAddress is a free data retrieval call binding the contract method 0x1442f7bb.
//
// Solidity: function calculateAuthorityAddress(address authorityOwner, uint256 epochLength, bytes32 salt) view returns(address)
func (_IAuthorityFactory *IAuthorityFactorySession) CalculateAuthorityAddress(authorityOwner common.Address, epochLength *big.Int, salt [32]byte) (common.Address, error) {
	return _IAuthorityFactory.Contract.CalculateAuthorityAddress(&_IAuthorityFactory.CallOpts, authorityOwner, epochLength, salt)
}

// CalculateAuthorityAddress is a free data retrieval call binding the contract method 0x1442f7bb.
//
// Solidity: function calculateAuthorityAddress(address authorityOwner, uint256 epochLength, bytes32 salt) view returns(address)
func (_IAuthorityFactory *IAuthorityFactoryCallerSession) CalculateAuthorityAddress(authorityOwner common.Address, epochLength *big.Int, salt [32]byte) (common.Address, error) {
	return _IAuthorityFactory.Contract.CalculateAuthorityAddress(&_IAuthorityFactory.CallOpts, authorityOwner, epochLength, salt)
}

// NewAuthority is a paid mutator transaction binding the contract method 0x93d7217c.
//
// Solidity: function newAuthority(address authorityOwner, uint256 epochLength) returns(address)
func (_IAuthorityFactory *IAuthorityFactoryTransactor) NewAuthority(opts *bind.TransactOpts, authorityOwner common.Address, epochLength *big.Int) (*types.Transaction, error) {
	return _IAuthorityFactory.contract.Transact(opts, "newAuthority", authorityOwner, epochLength)
}

// NewAuthority is a paid mutator transaction binding the contract method 0x93d7217c.
//
// Solidity: function newAuthority(address authorityOwner, uint256 epochLength) returns(address)
func (_IAuthorityFactory *IAuthorityFactorySession) NewAuthority(authorityOwner common.Address, epochLength *big.Int) (*types.Transaction, error) {
	return _IAuthorityFactory.Contract.NewAuthority(&_IAuthorityFactory.TransactOpts, authorityOwner, epochLength)
}

// NewAuthority is a paid mutator transaction binding the contract method 0x93d7217c.
//
// Solidity: function newAuthority(address authorityOwner, uint256 epochLength) returns(address)
func (_IAuthorityFactory *IAuthorityFactoryTransactorSession) NewAuthority(authorityOwner common.Address, epochLength *big.Int) (*types.Transaction, error) {
	return _IAuthorityFactory.Contract.NewAuthority(&_IAuthorityFactory.TransactOpts, authorityOwner, epochLength)
}

// NewAuthority0 is a paid mutator transaction binding the contract method 0xec992668.
//
// Solidity: function newAuthority(address authorityOwner, uint256 epochLength, bytes32 salt) returns(address)
func (_IAuthorityFactory *IAuthorityFactoryTransactor) NewAuthority0(opts *bind.TransactOpts, authorityOwner common.Address, epochLength *big.Int, salt [32]byte) (*types.Transaction, error) {
	return _IAuthorityFactory.contract.Transact(opts, "newAuthority0", authorityOwner, epochLength, salt)
}

// NewAuthority0 is a paid mutator transaction binding the contract method 0xec992668.
//
// Solidity: function newAuthority(address authorityOwner, uint256 epochLength, bytes32 salt) returns(address)
func (_IAuthorityFactory *IAuthorityFactorySession) NewAuthority0(authorityOwner common.Address, epochLength *big.Int, salt [32]byte) (*types.Transaction, error) {
	return _IAuthorityFactory.Contract.NewAuthority0(&_IAuthorityFactory.TransactOpts, authorityOwner, epochLength, salt)
}

// NewAuthority0 is a paid mutator transaction binding the contract method 0xec992668.
//
// Solidity: function newAuthority(address authorityOwner, uint256 epochLength, bytes32 salt) returns(address)
func (_IAuthorityFactory *IAuthorityFactoryTransactorSession) NewAuthority0(authorityOwner common.Address, epochLength *big.Int, salt [32]byte) (*types.Transaction, error) {
	return _IAuthorityFactory.Contract.NewAuthority0(&_IAuthorityFactory.TransactOpts, authorityOwner, epochLength, salt)
}

// IAuthorityFactoryAuthorityCreatedIterator is returned from FilterAuthorityCreated and is used to iterate over the raw logs and unpacked data for AuthorityCreated events raised by the IAuthorityFactory contract.
type IAuthorityFactoryAuthorityCreatedIterator struct {
	Event *IAuthorityFactoryAuthorityCreated // Event containing the contract specifics and raw log

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
func (it *IAuthorityFactoryAuthorityCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IAuthorityFactoryAuthorityCreated)
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
		it.Event = new(IAuthorityFactoryAuthorityCreated)
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
func (it *IAuthorityFactoryAuthorityCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IAuthorityFactoryAuthorityCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IAuthorityFactoryAuthorityCreated represents a AuthorityCreated event raised by the IAuthorityFactory contract.
type IAuthorityFactoryAuthorityCreated struct {
	Authority common.Address
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterAuthorityCreated is a free log retrieval operation binding the contract event 0xdca1fad70bee4ba7a4e17a1c6e99e657d2251af7a279124758bc01588abe2d2f.
//
// Solidity: event AuthorityCreated(address authority)
func (_IAuthorityFactory *IAuthorityFactoryFilterer) FilterAuthorityCreated(opts *bind.FilterOpts) (*IAuthorityFactoryAuthorityCreatedIterator, error) {

	logs, sub, err := _IAuthorityFactory.contract.FilterLogs(opts, "AuthorityCreated")
	if err != nil {
		return nil, err
	}
	return &IAuthorityFactoryAuthorityCreatedIterator{contract: _IAuthorityFactory.contract, event: "AuthorityCreated", logs: logs, sub: sub}, nil
}

// WatchAuthorityCreated is a free log subscription operation binding the contract event 0xdca1fad70bee4ba7a4e17a1c6e99e657d2251af7a279124758bc01588abe2d2f.
//
// Solidity: event AuthorityCreated(address authority)
func (_IAuthorityFactory *IAuthorityFactoryFilterer) WatchAuthorityCreated(opts *bind.WatchOpts, sink chan<- *IAuthorityFactoryAuthorityCreated) (event.Subscription, error) {

	logs, sub, err := _IAuthorityFactory.contract.WatchLogs(opts, "AuthorityCreated")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IAuthorityFactoryAuthorityCreated)
				if err := _IAuthorityFactory.contract.UnpackLog(event, "AuthorityCreated", log); err != nil {
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

// ParseAuthorityCreated is a log parse operation binding the contract event 0xdca1fad70bee4ba7a4e17a1c6e99e657d2251af7a279124758bc01588abe2d2f.
//
// Solidity: event AuthorityCreated(address authority)
func (_IAuthorityFactory *IAuthorityFactoryFilterer) ParseAuthorityCreated(log types.Log) (*IAuthorityFactoryAuthorityCreated, error) {
	event := new(IAuthorityFactoryAuthorityCreated)
	if err := _IAuthorityFactory.contract.UnpackLog(event, "AuthorityCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
