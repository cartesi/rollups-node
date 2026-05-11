// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package idaveappfactory

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

// WithdrawalConfig is an auto generated low-level Go binding around an user-defined struct.
type WithdrawalConfig struct {
	Guardian                common.Address
	Log2LeavesPerAccount    uint8
	Log2MaxNumOfAccounts    uint8
	AccountsDriveStartIndex uint64
	WithdrawalOutputBuilder common.Address
}

// IDaveAppFactoryMetaData contains all meta data concerning the IDaveAppFactory contract.
var IDaveAppFactoryMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"calculateDaveAppAddress\",\"inputs\":[{\"name\":\"templateHash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"withdrawalConfig\",\"type\":\"tuple\",\"internalType\":\"structWithdrawalConfig\",\"components\":[{\"name\":\"guardian\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"log2LeavesPerAccount\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"log2MaxNumOfAccounts\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"accountsDriveStartIndex\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"withdrawalOutputBuilder\",\"type\":\"address\",\"internalType\":\"contractIWithdrawalOutputBuilder\"}]},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"appContractAddress\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"daveConsensusAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"newDaveApp\",\"inputs\":[{\"name\":\"templateHash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"withdrawalConfig\",\"type\":\"tuple\",\"internalType\":\"structWithdrawalConfig\",\"components\":[{\"name\":\"guardian\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"log2LeavesPerAccount\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"log2MaxNumOfAccounts\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"accountsDriveStartIndex\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"withdrawalOutputBuilder\",\"type\":\"address\",\"internalType\":\"contractIWithdrawalOutputBuilder\"}]},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"contractIApplication\"},{\"name\":\"daveConsensus\",\"type\":\"address\",\"internalType\":\"contractIDaveConsensus\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"DaveAppCreated\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractIApplication\"},{\"name\":\"daveConsensus\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractIDaveConsensus\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"InvalidWithdrawalConfig\",\"inputs\":[{\"name\":\"withdrawalConfig\",\"type\":\"tuple\",\"internalType\":\"structWithdrawalConfig\",\"components\":[{\"name\":\"guardian\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"log2LeavesPerAccount\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"log2MaxNumOfAccounts\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"accountsDriveStartIndex\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"withdrawalOutputBuilder\",\"type\":\"address\",\"internalType\":\"contractIWithdrawalOutputBuilder\"}]}]}]",
}

// IDaveAppFactoryABI is the input ABI used to generate the binding from.
// Deprecated: Use IDaveAppFactoryMetaData.ABI instead.
var IDaveAppFactoryABI = IDaveAppFactoryMetaData.ABI

// IDaveAppFactory is an auto generated Go binding around an Ethereum contract.
type IDaveAppFactory struct {
	IDaveAppFactoryCaller     // Read-only binding to the contract
	IDaveAppFactoryTransactor // Write-only binding to the contract
	IDaveAppFactoryFilterer   // Log filterer for contract events
}

// IDaveAppFactoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type IDaveAppFactoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IDaveAppFactoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IDaveAppFactoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IDaveAppFactoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IDaveAppFactoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IDaveAppFactorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IDaveAppFactorySession struct {
	Contract     *IDaveAppFactory  // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IDaveAppFactoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IDaveAppFactoryCallerSession struct {
	Contract *IDaveAppFactoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts          // Call options to use throughout this session
}

// IDaveAppFactoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IDaveAppFactoryTransactorSession struct {
	Contract     *IDaveAppFactoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts          // Transaction auth options to use throughout this session
}

// IDaveAppFactoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type IDaveAppFactoryRaw struct {
	Contract *IDaveAppFactory // Generic contract binding to access the raw methods on
}

// IDaveAppFactoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IDaveAppFactoryCallerRaw struct {
	Contract *IDaveAppFactoryCaller // Generic read-only contract binding to access the raw methods on
}

// IDaveAppFactoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IDaveAppFactoryTransactorRaw struct {
	Contract *IDaveAppFactoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIDaveAppFactory creates a new instance of IDaveAppFactory, bound to a specific deployed contract.
func NewIDaveAppFactory(address common.Address, backend bind.ContractBackend) (*IDaveAppFactory, error) {
	contract, err := bindIDaveAppFactory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IDaveAppFactory{IDaveAppFactoryCaller: IDaveAppFactoryCaller{contract: contract}, IDaveAppFactoryTransactor: IDaveAppFactoryTransactor{contract: contract}, IDaveAppFactoryFilterer: IDaveAppFactoryFilterer{contract: contract}}, nil
}

// NewIDaveAppFactoryCaller creates a new read-only instance of IDaveAppFactory, bound to a specific deployed contract.
func NewIDaveAppFactoryCaller(address common.Address, caller bind.ContractCaller) (*IDaveAppFactoryCaller, error) {
	contract, err := bindIDaveAppFactory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IDaveAppFactoryCaller{contract: contract}, nil
}

// NewIDaveAppFactoryTransactor creates a new write-only instance of IDaveAppFactory, bound to a specific deployed contract.
func NewIDaveAppFactoryTransactor(address common.Address, transactor bind.ContractTransactor) (*IDaveAppFactoryTransactor, error) {
	contract, err := bindIDaveAppFactory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IDaveAppFactoryTransactor{contract: contract}, nil
}

// NewIDaveAppFactoryFilterer creates a new log filterer instance of IDaveAppFactory, bound to a specific deployed contract.
func NewIDaveAppFactoryFilterer(address common.Address, filterer bind.ContractFilterer) (*IDaveAppFactoryFilterer, error) {
	contract, err := bindIDaveAppFactory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IDaveAppFactoryFilterer{contract: contract}, nil
}

// bindIDaveAppFactory binds a generic wrapper to an already deployed contract.
func bindIDaveAppFactory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IDaveAppFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IDaveAppFactory *IDaveAppFactoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IDaveAppFactory.Contract.IDaveAppFactoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IDaveAppFactory *IDaveAppFactoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IDaveAppFactory.Contract.IDaveAppFactoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IDaveAppFactory *IDaveAppFactoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IDaveAppFactory.Contract.IDaveAppFactoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IDaveAppFactory *IDaveAppFactoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IDaveAppFactory.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IDaveAppFactory *IDaveAppFactoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IDaveAppFactory.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IDaveAppFactory *IDaveAppFactoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IDaveAppFactory.Contract.contract.Transact(opts, method, params...)
}

// CalculateDaveAppAddress is a free data retrieval call binding the contract method 0x4d3b6acb.
//
// Solidity: function calculateDaveAppAddress(bytes32 templateHash, (address,uint8,uint8,uint64,address) withdrawalConfig, bytes32 salt) view returns(address appContractAddress, address daveConsensusAddress)
func (_IDaveAppFactory *IDaveAppFactoryCaller) CalculateDaveAppAddress(opts *bind.CallOpts, templateHash [32]byte, withdrawalConfig WithdrawalConfig, salt [32]byte) (struct {
	AppContractAddress   common.Address
	DaveConsensusAddress common.Address
}, error) {
	var out []interface{}
	err := _IDaveAppFactory.contract.Call(opts, &out, "calculateDaveAppAddress", templateHash, withdrawalConfig, salt)

	outstruct := new(struct {
		AppContractAddress   common.Address
		DaveConsensusAddress common.Address
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.AppContractAddress = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.DaveConsensusAddress = *abi.ConvertType(out[1], new(common.Address)).(*common.Address)

	return *outstruct, err

}

// CalculateDaveAppAddress is a free data retrieval call binding the contract method 0x4d3b6acb.
//
// Solidity: function calculateDaveAppAddress(bytes32 templateHash, (address,uint8,uint8,uint64,address) withdrawalConfig, bytes32 salt) view returns(address appContractAddress, address daveConsensusAddress)
func (_IDaveAppFactory *IDaveAppFactorySession) CalculateDaveAppAddress(templateHash [32]byte, withdrawalConfig WithdrawalConfig, salt [32]byte) (struct {
	AppContractAddress   common.Address
	DaveConsensusAddress common.Address
}, error) {
	return _IDaveAppFactory.Contract.CalculateDaveAppAddress(&_IDaveAppFactory.CallOpts, templateHash, withdrawalConfig, salt)
}

// CalculateDaveAppAddress is a free data retrieval call binding the contract method 0x4d3b6acb.
//
// Solidity: function calculateDaveAppAddress(bytes32 templateHash, (address,uint8,uint8,uint64,address) withdrawalConfig, bytes32 salt) view returns(address appContractAddress, address daveConsensusAddress)
func (_IDaveAppFactory *IDaveAppFactoryCallerSession) CalculateDaveAppAddress(templateHash [32]byte, withdrawalConfig WithdrawalConfig, salt [32]byte) (struct {
	AppContractAddress   common.Address
	DaveConsensusAddress common.Address
}, error) {
	return _IDaveAppFactory.Contract.CalculateDaveAppAddress(&_IDaveAppFactory.CallOpts, templateHash, withdrawalConfig, salt)
}

// NewDaveApp is a paid mutator transaction binding the contract method 0xc119e684.
//
// Solidity: function newDaveApp(bytes32 templateHash, (address,uint8,uint8,uint64,address) withdrawalConfig, bytes32 salt) returns(address appContract, address daveConsensus)
func (_IDaveAppFactory *IDaveAppFactoryTransactor) NewDaveApp(opts *bind.TransactOpts, templateHash [32]byte, withdrawalConfig WithdrawalConfig, salt [32]byte) (*types.Transaction, error) {
	return _IDaveAppFactory.contract.Transact(opts, "newDaveApp", templateHash, withdrawalConfig, salt)
}

// NewDaveApp is a paid mutator transaction binding the contract method 0xc119e684.
//
// Solidity: function newDaveApp(bytes32 templateHash, (address,uint8,uint8,uint64,address) withdrawalConfig, bytes32 salt) returns(address appContract, address daveConsensus)
func (_IDaveAppFactory *IDaveAppFactorySession) NewDaveApp(templateHash [32]byte, withdrawalConfig WithdrawalConfig, salt [32]byte) (*types.Transaction, error) {
	return _IDaveAppFactory.Contract.NewDaveApp(&_IDaveAppFactory.TransactOpts, templateHash, withdrawalConfig, salt)
}

// NewDaveApp is a paid mutator transaction binding the contract method 0xc119e684.
//
// Solidity: function newDaveApp(bytes32 templateHash, (address,uint8,uint8,uint64,address) withdrawalConfig, bytes32 salt) returns(address appContract, address daveConsensus)
func (_IDaveAppFactory *IDaveAppFactoryTransactorSession) NewDaveApp(templateHash [32]byte, withdrawalConfig WithdrawalConfig, salt [32]byte) (*types.Transaction, error) {
	return _IDaveAppFactory.Contract.NewDaveApp(&_IDaveAppFactory.TransactOpts, templateHash, withdrawalConfig, salt)
}

// IDaveAppFactoryDaveAppCreatedIterator is returned from FilterDaveAppCreated and is used to iterate over the raw logs and unpacked data for DaveAppCreated events raised by the IDaveAppFactory contract.
type IDaveAppFactoryDaveAppCreatedIterator struct {
	Event *IDaveAppFactoryDaveAppCreated // Event containing the contract specifics and raw log

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
func (it *IDaveAppFactoryDaveAppCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IDaveAppFactoryDaveAppCreated)
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
		it.Event = new(IDaveAppFactoryDaveAppCreated)
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
func (it *IDaveAppFactoryDaveAppCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IDaveAppFactoryDaveAppCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IDaveAppFactoryDaveAppCreated represents a DaveAppCreated event raised by the IDaveAppFactory contract.
type IDaveAppFactoryDaveAppCreated struct {
	AppContract   common.Address
	DaveConsensus common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterDaveAppCreated is a free log retrieval operation binding the contract event 0xdf2ebeb5a7d7df0100c0274c7cee9570954d7bebeef37db55b27204a57f65602.
//
// Solidity: event DaveAppCreated(address appContract, address daveConsensus)
func (_IDaveAppFactory *IDaveAppFactoryFilterer) FilterDaveAppCreated(opts *bind.FilterOpts) (*IDaveAppFactoryDaveAppCreatedIterator, error) {

	logs, sub, err := _IDaveAppFactory.contract.FilterLogs(opts, "DaveAppCreated")
	if err != nil {
		return nil, err
	}
	return &IDaveAppFactoryDaveAppCreatedIterator{contract: _IDaveAppFactory.contract, event: "DaveAppCreated", logs: logs, sub: sub}, nil
}

// WatchDaveAppCreated is a free log subscription operation binding the contract event 0xdf2ebeb5a7d7df0100c0274c7cee9570954d7bebeef37db55b27204a57f65602.
//
// Solidity: event DaveAppCreated(address appContract, address daveConsensus)
func (_IDaveAppFactory *IDaveAppFactoryFilterer) WatchDaveAppCreated(opts *bind.WatchOpts, sink chan<- *IDaveAppFactoryDaveAppCreated) (event.Subscription, error) {

	logs, sub, err := _IDaveAppFactory.contract.WatchLogs(opts, "DaveAppCreated")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IDaveAppFactoryDaveAppCreated)
				if err := _IDaveAppFactory.contract.UnpackLog(event, "DaveAppCreated", log); err != nil {
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

// ParseDaveAppCreated is a log parse operation binding the contract event 0xdf2ebeb5a7d7df0100c0274c7cee9570954d7bebeef37db55b27204a57f65602.
//
// Solidity: event DaveAppCreated(address appContract, address daveConsensus)
func (_IDaveAppFactory *IDaveAppFactoryFilterer) ParseDaveAppCreated(log types.Log) (*IDaveAppFactoryDaveAppCreated, error) {
	event := new(IDaveAppFactoryDaveAppCreated)
	if err := _IDaveAppFactory.contract.UnpackLog(event, "DaveAppCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
