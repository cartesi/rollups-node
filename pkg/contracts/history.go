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

// HistoryClaim is an auto generated low-level Go binding around an user-defined struct.
type HistoryClaim struct {
	EpochHash  [32]byte
	FirstIndex *big.Int
	LastIndex  *big.Int
}

// HistoryMetaData contains all meta data concerning the History contract.
var HistoryMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_owner\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"InvalidClaimIndex\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidInputIndices\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"UnclaimedInputs\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"dapp\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"bytes32\",\"name\":\"epochHash\",\"type\":\"bytes32\"},{\"internalType\":\"uint128\",\"name\":\"firstIndex\",\"type\":\"uint128\"},{\"internalType\":\"uint128\",\"name\":\"lastIndex\",\"type\":\"uint128\"}],\"indexed\":false,\"internalType\":\"structHistory.Claim\",\"name\":\"claim\",\"type\":\"tuple\"}],\"name\":\"NewClaimToHistory\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"previousOwner\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"OwnershipTransferred\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_dapp\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"_proofContext\",\"type\":\"bytes\"}],\"name\":\"getClaim\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_consensus\",\"type\":\"address\"}],\"name\":\"migrateToConsensus\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"renounceOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"_claimData\",\"type\":\"bytes\"}],\"name\":\"submitClaim\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"transferOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// HistoryABI is the input ABI used to generate the binding from.
// Deprecated: Use HistoryMetaData.ABI instead.
var HistoryABI = HistoryMetaData.ABI

// History is an auto generated Go binding around an Ethereum contract.
type History struct {
	HistoryCaller     // Read-only binding to the contract
	HistoryTransactor // Write-only binding to the contract
	HistoryFilterer   // Log filterer for contract events
}

// HistoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type HistoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// HistoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type HistoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// HistoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type HistoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// HistorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type HistorySession struct {
	Contract     *History          // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// HistoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type HistoryCallerSession struct {
	Contract *HistoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts  // Call options to use throughout this session
}

// HistoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type HistoryTransactorSession struct {
	Contract     *HistoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts  // Transaction auth options to use throughout this session
}

// HistoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type HistoryRaw struct {
	Contract *History // Generic contract binding to access the raw methods on
}

// HistoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type HistoryCallerRaw struct {
	Contract *HistoryCaller // Generic read-only contract binding to access the raw methods on
}

// HistoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type HistoryTransactorRaw struct {
	Contract *HistoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewHistory creates a new instance of History, bound to a specific deployed contract.
func NewHistory(address common.Address, backend bind.ContractBackend) (*History, error) {
	contract, err := bindHistory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &History{HistoryCaller: HistoryCaller{contract: contract}, HistoryTransactor: HistoryTransactor{contract: contract}, HistoryFilterer: HistoryFilterer{contract: contract}}, nil
}

// NewHistoryCaller creates a new read-only instance of History, bound to a specific deployed contract.
func NewHistoryCaller(address common.Address, caller bind.ContractCaller) (*HistoryCaller, error) {
	contract, err := bindHistory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &HistoryCaller{contract: contract}, nil
}

// NewHistoryTransactor creates a new write-only instance of History, bound to a specific deployed contract.
func NewHistoryTransactor(address common.Address, transactor bind.ContractTransactor) (*HistoryTransactor, error) {
	contract, err := bindHistory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &HistoryTransactor{contract: contract}, nil
}

// NewHistoryFilterer creates a new log filterer instance of History, bound to a specific deployed contract.
func NewHistoryFilterer(address common.Address, filterer bind.ContractFilterer) (*HistoryFilterer, error) {
	contract, err := bindHistory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &HistoryFilterer{contract: contract}, nil
}

// bindHistory binds a generic wrapper to an already deployed contract.
func bindHistory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := HistoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_History *HistoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _History.Contract.HistoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_History *HistoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _History.Contract.HistoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_History *HistoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _History.Contract.HistoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_History *HistoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _History.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_History *HistoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _History.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_History *HistoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _History.Contract.contract.Transact(opts, method, params...)
}

// GetClaim is a free data retrieval call binding the contract method 0xd79a8240.
//
// Solidity: function getClaim(address _dapp, bytes _proofContext) view returns(bytes32, uint256, uint256)
func (_History *HistoryCaller) GetClaim(opts *bind.CallOpts, _dapp common.Address, _proofContext []byte) ([32]byte, *big.Int, *big.Int, error) {
	var out []interface{}
	err := _History.contract.Call(opts, &out, "getClaim", _dapp, _proofContext)

	if err != nil {
		return *new([32]byte), *new(*big.Int), *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)
	out1 := *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)
	out2 := *abi.ConvertType(out[2], new(*big.Int)).(**big.Int)

	return out0, out1, out2, err

}

// GetClaim is a free data retrieval call binding the contract method 0xd79a8240.
//
// Solidity: function getClaim(address _dapp, bytes _proofContext) view returns(bytes32, uint256, uint256)
func (_History *HistorySession) GetClaim(_dapp common.Address, _proofContext []byte) ([32]byte, *big.Int, *big.Int, error) {
	return _History.Contract.GetClaim(&_History.CallOpts, _dapp, _proofContext)
}

// GetClaim is a free data retrieval call binding the contract method 0xd79a8240.
//
// Solidity: function getClaim(address _dapp, bytes _proofContext) view returns(bytes32, uint256, uint256)
func (_History *HistoryCallerSession) GetClaim(_dapp common.Address, _proofContext []byte) ([32]byte, *big.Int, *big.Int, error) {
	return _History.Contract.GetClaim(&_History.CallOpts, _dapp, _proofContext)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_History *HistoryCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _History.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_History *HistorySession) Owner() (common.Address, error) {
	return _History.Contract.Owner(&_History.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_History *HistoryCallerSession) Owner() (common.Address, error) {
	return _History.Contract.Owner(&_History.CallOpts)
}

// MigrateToConsensus is a paid mutator transaction binding the contract method 0xfc411683.
//
// Solidity: function migrateToConsensus(address _consensus) returns()
func (_History *HistoryTransactor) MigrateToConsensus(opts *bind.TransactOpts, _consensus common.Address) (*types.Transaction, error) {
	return _History.contract.Transact(opts, "migrateToConsensus", _consensus)
}

// MigrateToConsensus is a paid mutator transaction binding the contract method 0xfc411683.
//
// Solidity: function migrateToConsensus(address _consensus) returns()
func (_History *HistorySession) MigrateToConsensus(_consensus common.Address) (*types.Transaction, error) {
	return _History.Contract.MigrateToConsensus(&_History.TransactOpts, _consensus)
}

// MigrateToConsensus is a paid mutator transaction binding the contract method 0xfc411683.
//
// Solidity: function migrateToConsensus(address _consensus) returns()
func (_History *HistoryTransactorSession) MigrateToConsensus(_consensus common.Address) (*types.Transaction, error) {
	return _History.Contract.MigrateToConsensus(&_History.TransactOpts, _consensus)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_History *HistoryTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _History.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_History *HistorySession) RenounceOwnership() (*types.Transaction, error) {
	return _History.Contract.RenounceOwnership(&_History.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_History *HistoryTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _History.Contract.RenounceOwnership(&_History.TransactOpts)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0xddfdfbb0.
//
// Solidity: function submitClaim(bytes _claimData) returns()
func (_History *HistoryTransactor) SubmitClaim(opts *bind.TransactOpts, _claimData []byte) (*types.Transaction, error) {
	return _History.contract.Transact(opts, "submitClaim", _claimData)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0xddfdfbb0.
//
// Solidity: function submitClaim(bytes _claimData) returns()
func (_History *HistorySession) SubmitClaim(_claimData []byte) (*types.Transaction, error) {
	return _History.Contract.SubmitClaim(&_History.TransactOpts, _claimData)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0xddfdfbb0.
//
// Solidity: function submitClaim(bytes _claimData) returns()
func (_History *HistoryTransactorSession) SubmitClaim(_claimData []byte) (*types.Transaction, error) {
	return _History.Contract.SubmitClaim(&_History.TransactOpts, _claimData)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_History *HistoryTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _History.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_History *HistorySession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _History.Contract.TransferOwnership(&_History.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_History *HistoryTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _History.Contract.TransferOwnership(&_History.TransactOpts, newOwner)
}

// HistoryNewClaimToHistoryIterator is returned from FilterNewClaimToHistory and is used to iterate over the raw logs and unpacked data for NewClaimToHistory events raised by the History contract.
type HistoryNewClaimToHistoryIterator struct {
	Event *HistoryNewClaimToHistory // Event containing the contract specifics and raw log

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
func (it *HistoryNewClaimToHistoryIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(HistoryNewClaimToHistory)
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
		it.Event = new(HistoryNewClaimToHistory)
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
func (it *HistoryNewClaimToHistoryIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *HistoryNewClaimToHistoryIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// HistoryNewClaimToHistory represents a NewClaimToHistory event raised by the History contract.
type HistoryNewClaimToHistory struct {
	Dapp  common.Address
	Claim HistoryClaim
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterNewClaimToHistory is a free log retrieval operation binding the contract event 0xb71880d7a0c514d48c0296b2721b0a4f9641a45117960f2ca86b5b7873c4ab2f.
//
// Solidity: event NewClaimToHistory(address indexed dapp, (bytes32,uint128,uint128) claim)
func (_History *HistoryFilterer) FilterNewClaimToHistory(opts *bind.FilterOpts, dapp []common.Address) (*HistoryNewClaimToHistoryIterator, error) {

	var dappRule []interface{}
	for _, dappItem := range dapp {
		dappRule = append(dappRule, dappItem)
	}

	logs, sub, err := _History.contract.FilterLogs(opts, "NewClaimToHistory", dappRule)
	if err != nil {
		return nil, err
	}
	return &HistoryNewClaimToHistoryIterator{contract: _History.contract, event: "NewClaimToHistory", logs: logs, sub: sub}, nil
}

// WatchNewClaimToHistory is a free log subscription operation binding the contract event 0xb71880d7a0c514d48c0296b2721b0a4f9641a45117960f2ca86b5b7873c4ab2f.
//
// Solidity: event NewClaimToHistory(address indexed dapp, (bytes32,uint128,uint128) claim)
func (_History *HistoryFilterer) WatchNewClaimToHistory(opts *bind.WatchOpts, sink chan<- *HistoryNewClaimToHistory, dapp []common.Address) (event.Subscription, error) {

	var dappRule []interface{}
	for _, dappItem := range dapp {
		dappRule = append(dappRule, dappItem)
	}

	logs, sub, err := _History.contract.WatchLogs(opts, "NewClaimToHistory", dappRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(HistoryNewClaimToHistory)
				if err := _History.contract.UnpackLog(event, "NewClaimToHistory", log); err != nil {
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

// ParseNewClaimToHistory is a log parse operation binding the contract event 0xb71880d7a0c514d48c0296b2721b0a4f9641a45117960f2ca86b5b7873c4ab2f.
//
// Solidity: event NewClaimToHistory(address indexed dapp, (bytes32,uint128,uint128) claim)
func (_History *HistoryFilterer) ParseNewClaimToHistory(log types.Log) (*HistoryNewClaimToHistory, error) {
	event := new(HistoryNewClaimToHistory)
	if err := _History.contract.UnpackLog(event, "NewClaimToHistory", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// HistoryOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the History contract.
type HistoryOwnershipTransferredIterator struct {
	Event *HistoryOwnershipTransferred // Event containing the contract specifics and raw log

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
func (it *HistoryOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(HistoryOwnershipTransferred)
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
		it.Event = new(HistoryOwnershipTransferred)
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
func (it *HistoryOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *HistoryOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// HistoryOwnershipTransferred represents a OwnershipTransferred event raised by the History contract.
type HistoryOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_History *HistoryFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*HistoryOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _History.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &HistoryOwnershipTransferredIterator{contract: _History.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_History *HistoryFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *HistoryOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _History.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(HistoryOwnershipTransferred)
				if err := _History.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
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

// ParseOwnershipTransferred is a log parse operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_History *HistoryFilterer) ParseOwnershipTransferred(log types.Log) (*HistoryOwnershipTransferred, error) {
	event := new(HistoryOwnershipTransferred)
	if err := _History.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
