// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package contracts

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

// CartesiDAppFactoryMetaData contains all meta data concerning the CartesiDAppFactory contract.
var CartesiDAppFactoryMetaData = &bind.MetaData{
	ABI: "[{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"contractIConsensus\",\"name\":\"consensus\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"dappOwner\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"templateHash\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"contractCartesiDApp\",\"name\":\"application\",\"type\":\"address\"}],\"name\":\"ApplicationCreated\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"contractIConsensus\",\"name\":\"_consensus\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_dappOwner\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"_templateHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"_salt\",\"type\":\"bytes32\"}],\"name\":\"calculateApplicationAddress\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"contractIConsensus\",\"name\":\"_consensus\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_dappOwner\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"_templateHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"_salt\",\"type\":\"bytes32\"}],\"name\":\"newApplication\",\"outputs\":[{\"internalType\":\"contractCartesiDApp\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"contractIConsensus\",\"name\":\"_consensus\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_dappOwner\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"_templateHash\",\"type\":\"bytes32\"}],\"name\":\"newApplication\",\"outputs\":[{\"internalType\":\"contractCartesiDApp\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// CartesiDAppFactoryABI is the input ABI used to generate the binding from.
// Deprecated: Use CartesiDAppFactoryMetaData.ABI instead.
var CartesiDAppFactoryABI = CartesiDAppFactoryMetaData.ABI

// CartesiDAppFactory is an auto generated Go binding around an Ethereum contract.
type CartesiDAppFactory struct {
	CartesiDAppFactoryCaller     // Read-only binding to the contract
	CartesiDAppFactoryTransactor // Write-only binding to the contract
	CartesiDAppFactoryFilterer   // Log filterer for contract events
}

// CartesiDAppFactoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type CartesiDAppFactoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CartesiDAppFactoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type CartesiDAppFactoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CartesiDAppFactoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type CartesiDAppFactoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CartesiDAppFactorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type CartesiDAppFactorySession struct {
	Contract     *CartesiDAppFactory // Generic contract binding to set the session for
	CallOpts     bind.CallOpts       // Call options to use throughout this session
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// CartesiDAppFactoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type CartesiDAppFactoryCallerSession struct {
	Contract *CartesiDAppFactoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts             // Call options to use throughout this session
}

// CartesiDAppFactoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type CartesiDAppFactoryTransactorSession struct {
	Contract     *CartesiDAppFactoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts             // Transaction auth options to use throughout this session
}

// CartesiDAppFactoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type CartesiDAppFactoryRaw struct {
	Contract *CartesiDAppFactory // Generic contract binding to access the raw methods on
}

// CartesiDAppFactoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type CartesiDAppFactoryCallerRaw struct {
	Contract *CartesiDAppFactoryCaller // Generic read-only contract binding to access the raw methods on
}

// CartesiDAppFactoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type CartesiDAppFactoryTransactorRaw struct {
	Contract *CartesiDAppFactoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewCartesiDAppFactory creates a new instance of CartesiDAppFactory, bound to a specific deployed contract.
func NewCartesiDAppFactory(address common.Address, backend bind.ContractBackend) (*CartesiDAppFactory, error) {
	contract, err := bindCartesiDAppFactory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &CartesiDAppFactory{CartesiDAppFactoryCaller: CartesiDAppFactoryCaller{contract: contract}, CartesiDAppFactoryTransactor: CartesiDAppFactoryTransactor{contract: contract}, CartesiDAppFactoryFilterer: CartesiDAppFactoryFilterer{contract: contract}}, nil
}

// NewCartesiDAppFactoryCaller creates a new read-only instance of CartesiDAppFactory, bound to a specific deployed contract.
func NewCartesiDAppFactoryCaller(address common.Address, caller bind.ContractCaller) (*CartesiDAppFactoryCaller, error) {
	contract, err := bindCartesiDAppFactory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &CartesiDAppFactoryCaller{contract: contract}, nil
}

// NewCartesiDAppFactoryTransactor creates a new write-only instance of CartesiDAppFactory, bound to a specific deployed contract.
func NewCartesiDAppFactoryTransactor(address common.Address, transactor bind.ContractTransactor) (*CartesiDAppFactoryTransactor, error) {
	contract, err := bindCartesiDAppFactory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &CartesiDAppFactoryTransactor{contract: contract}, nil
}

// NewCartesiDAppFactoryFilterer creates a new log filterer instance of CartesiDAppFactory, bound to a specific deployed contract.
func NewCartesiDAppFactoryFilterer(address common.Address, filterer bind.ContractFilterer) (*CartesiDAppFactoryFilterer, error) {
	contract, err := bindCartesiDAppFactory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &CartesiDAppFactoryFilterer{contract: contract}, nil
}

// bindCartesiDAppFactory binds a generic wrapper to an already deployed contract.
func bindCartesiDAppFactory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := CartesiDAppFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_CartesiDAppFactory *CartesiDAppFactoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _CartesiDAppFactory.Contract.CartesiDAppFactoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_CartesiDAppFactory *CartesiDAppFactoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CartesiDAppFactory.Contract.CartesiDAppFactoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_CartesiDAppFactory *CartesiDAppFactoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _CartesiDAppFactory.Contract.CartesiDAppFactoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_CartesiDAppFactory *CartesiDAppFactoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _CartesiDAppFactory.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_CartesiDAppFactory *CartesiDAppFactoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CartesiDAppFactory.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_CartesiDAppFactory *CartesiDAppFactoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _CartesiDAppFactory.Contract.contract.Transact(opts, method, params...)
}

// CalculateApplicationAddress is a free data retrieval call binding the contract method 0xbd4f1219.
//
// Solidity: function calculateApplicationAddress(address _consensus, address _dappOwner, bytes32 _templateHash, bytes32 _salt) view returns(address)
func (_CartesiDAppFactory *CartesiDAppFactoryCaller) CalculateApplicationAddress(opts *bind.CallOpts, _consensus common.Address, _dappOwner common.Address, _templateHash [32]byte, _salt [32]byte) (common.Address, error) {
	var out []interface{}
	err := _CartesiDAppFactory.contract.Call(opts, &out, "calculateApplicationAddress", _consensus, _dappOwner, _templateHash, _salt)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// CalculateApplicationAddress is a free data retrieval call binding the contract method 0xbd4f1219.
//
// Solidity: function calculateApplicationAddress(address _consensus, address _dappOwner, bytes32 _templateHash, bytes32 _salt) view returns(address)
func (_CartesiDAppFactory *CartesiDAppFactorySession) CalculateApplicationAddress(_consensus common.Address, _dappOwner common.Address, _templateHash [32]byte, _salt [32]byte) (common.Address, error) {
	return _CartesiDAppFactory.Contract.CalculateApplicationAddress(&_CartesiDAppFactory.CallOpts, _consensus, _dappOwner, _templateHash, _salt)
}

// CalculateApplicationAddress is a free data retrieval call binding the contract method 0xbd4f1219.
//
// Solidity: function calculateApplicationAddress(address _consensus, address _dappOwner, bytes32 _templateHash, bytes32 _salt) view returns(address)
func (_CartesiDAppFactory *CartesiDAppFactoryCallerSession) CalculateApplicationAddress(_consensus common.Address, _dappOwner common.Address, _templateHash [32]byte, _salt [32]byte) (common.Address, error) {
	return _CartesiDAppFactory.Contract.CalculateApplicationAddress(&_CartesiDAppFactory.CallOpts, _consensus, _dappOwner, _templateHash, _salt)
}

// NewApplication is a paid mutator transaction binding the contract method 0x0e1a07f5.
//
// Solidity: function newApplication(address _consensus, address _dappOwner, bytes32 _templateHash, bytes32 _salt) returns(address)
func (_CartesiDAppFactory *CartesiDAppFactoryTransactor) NewApplication(opts *bind.TransactOpts, _consensus common.Address, _dappOwner common.Address, _templateHash [32]byte, _salt [32]byte) (*types.Transaction, error) {
	return _CartesiDAppFactory.contract.Transact(opts, "newApplication", _consensus, _dappOwner, _templateHash, _salt)
}

// NewApplication is a paid mutator transaction binding the contract method 0x0e1a07f5.
//
// Solidity: function newApplication(address _consensus, address _dappOwner, bytes32 _templateHash, bytes32 _salt) returns(address)
func (_CartesiDAppFactory *CartesiDAppFactorySession) NewApplication(_consensus common.Address, _dappOwner common.Address, _templateHash [32]byte, _salt [32]byte) (*types.Transaction, error) {
	return _CartesiDAppFactory.Contract.NewApplication(&_CartesiDAppFactory.TransactOpts, _consensus, _dappOwner, _templateHash, _salt)
}

// NewApplication is a paid mutator transaction binding the contract method 0x0e1a07f5.
//
// Solidity: function newApplication(address _consensus, address _dappOwner, bytes32 _templateHash, bytes32 _salt) returns(address)
func (_CartesiDAppFactory *CartesiDAppFactoryTransactorSession) NewApplication(_consensus common.Address, _dappOwner common.Address, _templateHash [32]byte, _salt [32]byte) (*types.Transaction, error) {
	return _CartesiDAppFactory.Contract.NewApplication(&_CartesiDAppFactory.TransactOpts, _consensus, _dappOwner, _templateHash, _salt)
}

// NewApplication0 is a paid mutator transaction binding the contract method 0x3648bfb5.
//
// Solidity: function newApplication(address _consensus, address _dappOwner, bytes32 _templateHash) returns(address)
func (_CartesiDAppFactory *CartesiDAppFactoryTransactor) NewApplication0(opts *bind.TransactOpts, _consensus common.Address, _dappOwner common.Address, _templateHash [32]byte) (*types.Transaction, error) {
	return _CartesiDAppFactory.contract.Transact(opts, "newApplication0", _consensus, _dappOwner, _templateHash)
}

// NewApplication0 is a paid mutator transaction binding the contract method 0x3648bfb5.
//
// Solidity: function newApplication(address _consensus, address _dappOwner, bytes32 _templateHash) returns(address)
func (_CartesiDAppFactory *CartesiDAppFactorySession) NewApplication0(_consensus common.Address, _dappOwner common.Address, _templateHash [32]byte) (*types.Transaction, error) {
	return _CartesiDAppFactory.Contract.NewApplication0(&_CartesiDAppFactory.TransactOpts, _consensus, _dappOwner, _templateHash)
}

// NewApplication0 is a paid mutator transaction binding the contract method 0x3648bfb5.
//
// Solidity: function newApplication(address _consensus, address _dappOwner, bytes32 _templateHash) returns(address)
func (_CartesiDAppFactory *CartesiDAppFactoryTransactorSession) NewApplication0(_consensus common.Address, _dappOwner common.Address, _templateHash [32]byte) (*types.Transaction, error) {
	return _CartesiDAppFactory.Contract.NewApplication0(&_CartesiDAppFactory.TransactOpts, _consensus, _dappOwner, _templateHash)
}

// CartesiDAppFactoryApplicationCreatedIterator is returned from FilterApplicationCreated and is used to iterate over the raw logs and unpacked data for ApplicationCreated events raised by the CartesiDAppFactory contract.
type CartesiDAppFactoryApplicationCreatedIterator struct {
	Event *CartesiDAppFactoryApplicationCreated // Event containing the contract specifics and raw log

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
func (it *CartesiDAppFactoryApplicationCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(CartesiDAppFactoryApplicationCreated)
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
		it.Event = new(CartesiDAppFactoryApplicationCreated)
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
func (it *CartesiDAppFactoryApplicationCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *CartesiDAppFactoryApplicationCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// CartesiDAppFactoryApplicationCreated represents a ApplicationCreated event raised by the CartesiDAppFactory contract.
type CartesiDAppFactoryApplicationCreated struct {
	Consensus    common.Address
	DappOwner    common.Address
	TemplateHash [32]byte
	Application  common.Address
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterApplicationCreated is a free log retrieval operation binding the contract event 0xe73165c2d277daf8713fd08b40845cb6bb7a20b2b543f3d35324a475660fcebd.
//
// Solidity: event ApplicationCreated(address indexed consensus, address dappOwner, bytes32 templateHash, address application)
func (_CartesiDAppFactory *CartesiDAppFactoryFilterer) FilterApplicationCreated(opts *bind.FilterOpts, consensus []common.Address) (*CartesiDAppFactoryApplicationCreatedIterator, error) {

	var consensusRule []interface{}
	for _, consensusItem := range consensus {
		consensusRule = append(consensusRule, consensusItem)
	}

	logs, sub, err := _CartesiDAppFactory.contract.FilterLogs(opts, "ApplicationCreated", consensusRule)
	if err != nil {
		return nil, err
	}
	return &CartesiDAppFactoryApplicationCreatedIterator{contract: _CartesiDAppFactory.contract, event: "ApplicationCreated", logs: logs, sub: sub}, nil
}

// WatchApplicationCreated is a free log subscription operation binding the contract event 0xe73165c2d277daf8713fd08b40845cb6bb7a20b2b543f3d35324a475660fcebd.
//
// Solidity: event ApplicationCreated(address indexed consensus, address dappOwner, bytes32 templateHash, address application)
func (_CartesiDAppFactory *CartesiDAppFactoryFilterer) WatchApplicationCreated(opts *bind.WatchOpts, sink chan<- *CartesiDAppFactoryApplicationCreated, consensus []common.Address) (event.Subscription, error) {

	var consensusRule []interface{}
	for _, consensusItem := range consensus {
		consensusRule = append(consensusRule, consensusItem)
	}

	logs, sub, err := _CartesiDAppFactory.contract.WatchLogs(opts, "ApplicationCreated", consensusRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(CartesiDAppFactoryApplicationCreated)
				if err := _CartesiDAppFactory.contract.UnpackLog(event, "ApplicationCreated", log); err != nil {
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
// Solidity: event ApplicationCreated(address indexed consensus, address dappOwner, bytes32 templateHash, address application)
func (_CartesiDAppFactory *CartesiDAppFactoryFilterer) ParseApplicationCreated(log types.Log) (*CartesiDAppFactoryApplicationCreated, error) {
	event := new(CartesiDAppFactoryApplicationCreated)
	if err := _CartesiDAppFactory.contract.UnpackLog(event, "ApplicationCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
