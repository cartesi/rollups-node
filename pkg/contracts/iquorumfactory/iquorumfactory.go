// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package iquorumfactory

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

// IQuorumFactoryMetaData contains all meta data concerning the IQuorumFactory contract.
var IQuorumFactoryMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"calculateQuorumAddress\",\"inputs\":[{\"name\":\"validators\",\"type\":\"address[]\",\"internalType\":\"address[]\"},{\"name\":\"epochLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"claimStagingPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"newQuorum\",\"inputs\":[{\"name\":\"validators\",\"type\":\"address[]\",\"internalType\":\"address[]\"},{\"name\":\"epochLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"claimStagingPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractIQuorum\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"newQuorum\",\"inputs\":[{\"name\":\"validators\",\"type\":\"address[]\",\"internalType\":\"address[]\"},{\"name\":\"epochLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"claimStagingPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractIQuorum\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"major\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"minor\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"patch\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"preRelease\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"buildMetadata\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"QuorumCreated\",\"inputs\":[{\"name\":\"quorum\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractIQuorum\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"EmptyQuorum\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ZeroAddressValidator\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ZeroEpochLength\",\"inputs\":[]}]",
}

// IQuorumFactoryABI is the input ABI used to generate the binding from.
// Deprecated: Use IQuorumFactoryMetaData.ABI instead.
var IQuorumFactoryABI = IQuorumFactoryMetaData.ABI

// IQuorumFactory is an auto generated Go binding around an Ethereum contract.
type IQuorumFactory struct {
	IQuorumFactoryCaller     // Read-only binding to the contract
	IQuorumFactoryTransactor // Write-only binding to the contract
	IQuorumFactoryFilterer   // Log filterer for contract events
}

// IQuorumFactoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type IQuorumFactoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IQuorumFactoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IQuorumFactoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IQuorumFactoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IQuorumFactoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IQuorumFactorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IQuorumFactorySession struct {
	Contract     *IQuorumFactory   // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IQuorumFactoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IQuorumFactoryCallerSession struct {
	Contract *IQuorumFactoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts         // Call options to use throughout this session
}

// IQuorumFactoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IQuorumFactoryTransactorSession struct {
	Contract     *IQuorumFactoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts         // Transaction auth options to use throughout this session
}

// IQuorumFactoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type IQuorumFactoryRaw struct {
	Contract *IQuorumFactory // Generic contract binding to access the raw methods on
}

// IQuorumFactoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IQuorumFactoryCallerRaw struct {
	Contract *IQuorumFactoryCaller // Generic read-only contract binding to access the raw methods on
}

// IQuorumFactoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IQuorumFactoryTransactorRaw struct {
	Contract *IQuorumFactoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIQuorumFactory creates a new instance of IQuorumFactory, bound to a specific deployed contract.
func NewIQuorumFactory(address common.Address, backend bind.ContractBackend) (*IQuorumFactory, error) {
	contract, err := bindIQuorumFactory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IQuorumFactory{IQuorumFactoryCaller: IQuorumFactoryCaller{contract: contract}, IQuorumFactoryTransactor: IQuorumFactoryTransactor{contract: contract}, IQuorumFactoryFilterer: IQuorumFactoryFilterer{contract: contract}}, nil
}

// NewIQuorumFactoryCaller creates a new read-only instance of IQuorumFactory, bound to a specific deployed contract.
func NewIQuorumFactoryCaller(address common.Address, caller bind.ContractCaller) (*IQuorumFactoryCaller, error) {
	contract, err := bindIQuorumFactory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IQuorumFactoryCaller{contract: contract}, nil
}

// NewIQuorumFactoryTransactor creates a new write-only instance of IQuorumFactory, bound to a specific deployed contract.
func NewIQuorumFactoryTransactor(address common.Address, transactor bind.ContractTransactor) (*IQuorumFactoryTransactor, error) {
	contract, err := bindIQuorumFactory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IQuorumFactoryTransactor{contract: contract}, nil
}

// NewIQuorumFactoryFilterer creates a new log filterer instance of IQuorumFactory, bound to a specific deployed contract.
func NewIQuorumFactoryFilterer(address common.Address, filterer bind.ContractFilterer) (*IQuorumFactoryFilterer, error) {
	contract, err := bindIQuorumFactory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IQuorumFactoryFilterer{contract: contract}, nil
}

// bindIQuorumFactory binds a generic wrapper to an already deployed contract.
func bindIQuorumFactory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IQuorumFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IQuorumFactory *IQuorumFactoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IQuorumFactory.Contract.IQuorumFactoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IQuorumFactory *IQuorumFactoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IQuorumFactory.Contract.IQuorumFactoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IQuorumFactory *IQuorumFactoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IQuorumFactory.Contract.IQuorumFactoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IQuorumFactory *IQuorumFactoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IQuorumFactory.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IQuorumFactory *IQuorumFactoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IQuorumFactory.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IQuorumFactory *IQuorumFactoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IQuorumFactory.Contract.contract.Transact(opts, method, params...)
}

// CalculateQuorumAddress is a free data retrieval call binding the contract method 0xdbf30807.
//
// Solidity: function calculateQuorumAddress(address[] validators, uint256 epochLength, uint256 claimStagingPeriod, bytes32 salt) view returns(address)
func (_IQuorumFactory *IQuorumFactoryCaller) CalculateQuorumAddress(opts *bind.CallOpts, validators []common.Address, epochLength *big.Int, claimStagingPeriod *big.Int, salt [32]byte) (common.Address, error) {
	var out []interface{}
	err := _IQuorumFactory.contract.Call(opts, &out, "calculateQuorumAddress", validators, epochLength, claimStagingPeriod, salt)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// CalculateQuorumAddress is a free data retrieval call binding the contract method 0xdbf30807.
//
// Solidity: function calculateQuorumAddress(address[] validators, uint256 epochLength, uint256 claimStagingPeriod, bytes32 salt) view returns(address)
func (_IQuorumFactory *IQuorumFactorySession) CalculateQuorumAddress(validators []common.Address, epochLength *big.Int, claimStagingPeriod *big.Int, salt [32]byte) (common.Address, error) {
	return _IQuorumFactory.Contract.CalculateQuorumAddress(&_IQuorumFactory.CallOpts, validators, epochLength, claimStagingPeriod, salt)
}

// CalculateQuorumAddress is a free data retrieval call binding the contract method 0xdbf30807.
//
// Solidity: function calculateQuorumAddress(address[] validators, uint256 epochLength, uint256 claimStagingPeriod, bytes32 salt) view returns(address)
func (_IQuorumFactory *IQuorumFactoryCallerSession) CalculateQuorumAddress(validators []common.Address, epochLength *big.Int, claimStagingPeriod *big.Int, salt [32]byte) (common.Address, error) {
	return _IQuorumFactory.Contract.CalculateQuorumAddress(&_IQuorumFactory.CallOpts, validators, epochLength, claimStagingPeriod, salt)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IQuorumFactory *IQuorumFactoryCaller) Version(opts *bind.CallOpts) (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	var out []interface{}
	err := _IQuorumFactory.contract.Call(opts, &out, "version")

	outstruct := new(struct {
		Major         uint64
		Minor         uint64
		Patch         uint64
		PreRelease    string
		BuildMetadata string
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Major = *abi.ConvertType(out[0], new(uint64)).(*uint64)
	outstruct.Minor = *abi.ConvertType(out[1], new(uint64)).(*uint64)
	outstruct.Patch = *abi.ConvertType(out[2], new(uint64)).(*uint64)
	outstruct.PreRelease = *abi.ConvertType(out[3], new(string)).(*string)
	outstruct.BuildMetadata = *abi.ConvertType(out[4], new(string)).(*string)

	return *outstruct, err

}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IQuorumFactory *IQuorumFactorySession) Version() (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	return _IQuorumFactory.Contract.Version(&_IQuorumFactory.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IQuorumFactory *IQuorumFactoryCallerSession) Version() (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	return _IQuorumFactory.Contract.Version(&_IQuorumFactory.CallOpts)
}

// NewQuorum is a paid mutator transaction binding the contract method 0x0f726dd4.
//
// Solidity: function newQuorum(address[] validators, uint256 epochLength, uint256 claimStagingPeriod, bytes32 salt) returns(address)
func (_IQuorumFactory *IQuorumFactoryTransactor) NewQuorum(opts *bind.TransactOpts, validators []common.Address, epochLength *big.Int, claimStagingPeriod *big.Int, salt [32]byte) (*types.Transaction, error) {
	return _IQuorumFactory.contract.Transact(opts, "newQuorum", validators, epochLength, claimStagingPeriod, salt)
}

// NewQuorum is a paid mutator transaction binding the contract method 0x0f726dd4.
//
// Solidity: function newQuorum(address[] validators, uint256 epochLength, uint256 claimStagingPeriod, bytes32 salt) returns(address)
func (_IQuorumFactory *IQuorumFactorySession) NewQuorum(validators []common.Address, epochLength *big.Int, claimStagingPeriod *big.Int, salt [32]byte) (*types.Transaction, error) {
	return _IQuorumFactory.Contract.NewQuorum(&_IQuorumFactory.TransactOpts, validators, epochLength, claimStagingPeriod, salt)
}

// NewQuorum is a paid mutator transaction binding the contract method 0x0f726dd4.
//
// Solidity: function newQuorum(address[] validators, uint256 epochLength, uint256 claimStagingPeriod, bytes32 salt) returns(address)
func (_IQuorumFactory *IQuorumFactoryTransactorSession) NewQuorum(validators []common.Address, epochLength *big.Int, claimStagingPeriod *big.Int, salt [32]byte) (*types.Transaction, error) {
	return _IQuorumFactory.Contract.NewQuorum(&_IQuorumFactory.TransactOpts, validators, epochLength, claimStagingPeriod, salt)
}

// NewQuorum0 is a paid mutator transaction binding the contract method 0xae123219.
//
// Solidity: function newQuorum(address[] validators, uint256 epochLength, uint256 claimStagingPeriod) returns(address)
func (_IQuorumFactory *IQuorumFactoryTransactor) NewQuorum0(opts *bind.TransactOpts, validators []common.Address, epochLength *big.Int, claimStagingPeriod *big.Int) (*types.Transaction, error) {
	return _IQuorumFactory.contract.Transact(opts, "newQuorum0", validators, epochLength, claimStagingPeriod)
}

// NewQuorum0 is a paid mutator transaction binding the contract method 0xae123219.
//
// Solidity: function newQuorum(address[] validators, uint256 epochLength, uint256 claimStagingPeriod) returns(address)
func (_IQuorumFactory *IQuorumFactorySession) NewQuorum0(validators []common.Address, epochLength *big.Int, claimStagingPeriod *big.Int) (*types.Transaction, error) {
	return _IQuorumFactory.Contract.NewQuorum0(&_IQuorumFactory.TransactOpts, validators, epochLength, claimStagingPeriod)
}

// NewQuorum0 is a paid mutator transaction binding the contract method 0xae123219.
//
// Solidity: function newQuorum(address[] validators, uint256 epochLength, uint256 claimStagingPeriod) returns(address)
func (_IQuorumFactory *IQuorumFactoryTransactorSession) NewQuorum0(validators []common.Address, epochLength *big.Int, claimStagingPeriod *big.Int) (*types.Transaction, error) {
	return _IQuorumFactory.Contract.NewQuorum0(&_IQuorumFactory.TransactOpts, validators, epochLength, claimStagingPeriod)
}

// IQuorumFactoryQuorumCreatedIterator is returned from FilterQuorumCreated and is used to iterate over the raw logs and unpacked data for QuorumCreated events raised by the IQuorumFactory contract.
type IQuorumFactoryQuorumCreatedIterator struct {
	Event *IQuorumFactoryQuorumCreated // Event containing the contract specifics and raw log

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
func (it *IQuorumFactoryQuorumCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IQuorumFactoryQuorumCreated)
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
		it.Event = new(IQuorumFactoryQuorumCreated)
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
func (it *IQuorumFactoryQuorumCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IQuorumFactoryQuorumCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IQuorumFactoryQuorumCreated represents a QuorumCreated event raised by the IQuorumFactory contract.
type IQuorumFactoryQuorumCreated struct {
	Quorum common.Address
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterQuorumCreated is a free log retrieval operation binding the contract event 0x446698b70271bce331e53210572bd37ac8c590b6cdca2e6763e6448243cba802.
//
// Solidity: event QuorumCreated(address quorum)
func (_IQuorumFactory *IQuorumFactoryFilterer) FilterQuorumCreated(opts *bind.FilterOpts) (*IQuorumFactoryQuorumCreatedIterator, error) {

	logs, sub, err := _IQuorumFactory.contract.FilterLogs(opts, "QuorumCreated")
	if err != nil {
		return nil, err
	}
	return &IQuorumFactoryQuorumCreatedIterator{contract: _IQuorumFactory.contract, event: "QuorumCreated", logs: logs, sub: sub}, nil
}

// WatchQuorumCreated is a free log subscription operation binding the contract event 0x446698b70271bce331e53210572bd37ac8c590b6cdca2e6763e6448243cba802.
//
// Solidity: event QuorumCreated(address quorum)
func (_IQuorumFactory *IQuorumFactoryFilterer) WatchQuorumCreated(opts *bind.WatchOpts, sink chan<- *IQuorumFactoryQuorumCreated) (event.Subscription, error) {

	logs, sub, err := _IQuorumFactory.contract.WatchLogs(opts, "QuorumCreated")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IQuorumFactoryQuorumCreated)
				if err := _IQuorumFactory.contract.UnpackLog(event, "QuorumCreated", log); err != nil {
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

// ParseQuorumCreated is a log parse operation binding the contract event 0x446698b70271bce331e53210572bd37ac8c590b6cdca2e6763e6448243cba802.
//
// Solidity: event QuorumCreated(address quorum)
func (_IQuorumFactory *IQuorumFactoryFilterer) ParseQuorumCreated(log types.Log) (*IQuorumFactoryQuorumCreated, error) {
	event := new(IQuorumFactoryQuorumCreated)
	if err := _IQuorumFactory.contract.UnpackLog(event, "QuorumCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
