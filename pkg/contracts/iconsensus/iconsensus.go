// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package iconsensus

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

// InputRange is an auto generated low-level Go binding around an user-defined struct.
type InputRange struct {
	FirstIndex uint64
	LastIndex  uint64
}

// IConsensusMetaData contains all meta data concerning the IConsensus contract.
var IConsensusMetaData = &bind.MetaData{
	ABI: "[{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"appContract\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"uint64\",\"name\":\"firstIndex\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"lastIndex\",\"type\":\"uint64\"}],\"indexed\":false,\"internalType\":\"structInputRange\",\"name\":\"inputRange\",\"type\":\"tuple\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"epochHash\",\"type\":\"bytes32\"}],\"name\":\"ClaimAcceptance\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"submitter\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"appContract\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"uint64\",\"name\":\"firstIndex\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"lastIndex\",\"type\":\"uint64\"}],\"indexed\":false,\"internalType\":\"structInputRange\",\"name\":\"inputRange\",\"type\":\"tuple\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"epochHash\",\"type\":\"bytes32\"}],\"name\":\"ClaimSubmission\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"appContract\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"uint64\",\"name\":\"firstIndex\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"lastIndex\",\"type\":\"uint64\"}],\"internalType\":\"structInputRange\",\"name\":\"inputRange\",\"type\":\"tuple\"}],\"name\":\"getEpochHash\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"epochHash\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"appContract\",\"type\":\"address\"},{\"components\":[{\"internalType\":\"uint64\",\"name\":\"firstIndex\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"lastIndex\",\"type\":\"uint64\"}],\"internalType\":\"structInputRange\",\"name\":\"inputRange\",\"type\":\"tuple\"},{\"internalType\":\"bytes32\",\"name\":\"epochHash\",\"type\":\"bytes32\"}],\"name\":\"submitClaim\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// IConsensusABI is the input ABI used to generate the binding from.
// Deprecated: Use IConsensusMetaData.ABI instead.
var IConsensusABI = IConsensusMetaData.ABI

// IConsensus is an auto generated Go binding around an Ethereum contract.
type IConsensus struct {
	IConsensusCaller     // Read-only binding to the contract
	IConsensusTransactor // Write-only binding to the contract
	IConsensusFilterer   // Log filterer for contract events
}

// IConsensusCaller is an auto generated read-only Go binding around an Ethereum contract.
type IConsensusCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IConsensusTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IConsensusTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IConsensusFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IConsensusFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IConsensusSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IConsensusSession struct {
	Contract     *IConsensus       // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IConsensusCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IConsensusCallerSession struct {
	Contract *IConsensusCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts     // Call options to use throughout this session
}

// IConsensusTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IConsensusTransactorSession struct {
	Contract     *IConsensusTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// IConsensusRaw is an auto generated low-level Go binding around an Ethereum contract.
type IConsensusRaw struct {
	Contract *IConsensus // Generic contract binding to access the raw methods on
}

// IConsensusCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IConsensusCallerRaw struct {
	Contract *IConsensusCaller // Generic read-only contract binding to access the raw methods on
}

// IConsensusTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IConsensusTransactorRaw struct {
	Contract *IConsensusTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIConsensus creates a new instance of IConsensus, bound to a specific deployed contract.
func NewIConsensus(address common.Address, backend bind.ContractBackend) (*IConsensus, error) {
	contract, err := bindIConsensus(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IConsensus{IConsensusCaller: IConsensusCaller{contract: contract}, IConsensusTransactor: IConsensusTransactor{contract: contract}, IConsensusFilterer: IConsensusFilterer{contract: contract}}, nil
}

// NewIConsensusCaller creates a new read-only instance of IConsensus, bound to a specific deployed contract.
func NewIConsensusCaller(address common.Address, caller bind.ContractCaller) (*IConsensusCaller, error) {
	contract, err := bindIConsensus(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IConsensusCaller{contract: contract}, nil
}

// NewIConsensusTransactor creates a new write-only instance of IConsensus, bound to a specific deployed contract.
func NewIConsensusTransactor(address common.Address, transactor bind.ContractTransactor) (*IConsensusTransactor, error) {
	contract, err := bindIConsensus(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IConsensusTransactor{contract: contract}, nil
}

// NewIConsensusFilterer creates a new log filterer instance of IConsensus, bound to a specific deployed contract.
func NewIConsensusFilterer(address common.Address, filterer bind.ContractFilterer) (*IConsensusFilterer, error) {
	contract, err := bindIConsensus(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IConsensusFilterer{contract: contract}, nil
}

// bindIConsensus binds a generic wrapper to an already deployed contract.
func bindIConsensus(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IConsensusMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IConsensus *IConsensusRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IConsensus.Contract.IConsensusCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IConsensus *IConsensusRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IConsensus.Contract.IConsensusTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IConsensus *IConsensusRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IConsensus.Contract.IConsensusTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IConsensus *IConsensusCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IConsensus.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IConsensus *IConsensusTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IConsensus.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IConsensus *IConsensusTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IConsensus.Contract.contract.Transact(opts, method, params...)
}

// GetEpochHash is a free data retrieval call binding the contract method 0xc1f59afc.
//
// Solidity: function getEpochHash(address appContract, (uint64,uint64) inputRange) view returns(bytes32 epochHash)
func (_IConsensus *IConsensusCaller) GetEpochHash(opts *bind.CallOpts, appContract common.Address, inputRange InputRange) ([32]byte, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "getEpochHash", appContract, inputRange)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetEpochHash is a free data retrieval call binding the contract method 0xc1f59afc.
//
// Solidity: function getEpochHash(address appContract, (uint64,uint64) inputRange) view returns(bytes32 epochHash)
func (_IConsensus *IConsensusSession) GetEpochHash(appContract common.Address, inputRange InputRange) ([32]byte, error) {
	return _IConsensus.Contract.GetEpochHash(&_IConsensus.CallOpts, appContract, inputRange)
}

// GetEpochHash is a free data retrieval call binding the contract method 0xc1f59afc.
//
// Solidity: function getEpochHash(address appContract, (uint64,uint64) inputRange) view returns(bytes32 epochHash)
func (_IConsensus *IConsensusCallerSession) GetEpochHash(appContract common.Address, inputRange InputRange) ([32]byte, error) {
	return _IConsensus.Contract.GetEpochHash(&_IConsensus.CallOpts, appContract, inputRange)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x866b85fa.
//
// Solidity: function submitClaim(address appContract, (uint64,uint64) inputRange, bytes32 epochHash) returns()
func (_IConsensus *IConsensusTransactor) SubmitClaim(opts *bind.TransactOpts, appContract common.Address, inputRange InputRange, epochHash [32]byte) (*types.Transaction, error) {
	return _IConsensus.contract.Transact(opts, "submitClaim", appContract, inputRange, epochHash)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x866b85fa.
//
// Solidity: function submitClaim(address appContract, (uint64,uint64) inputRange, bytes32 epochHash) returns()
func (_IConsensus *IConsensusSession) SubmitClaim(appContract common.Address, inputRange InputRange, epochHash [32]byte) (*types.Transaction, error) {
	return _IConsensus.Contract.SubmitClaim(&_IConsensus.TransactOpts, appContract, inputRange, epochHash)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x866b85fa.
//
// Solidity: function submitClaim(address appContract, (uint64,uint64) inputRange, bytes32 epochHash) returns()
func (_IConsensus *IConsensusTransactorSession) SubmitClaim(appContract common.Address, inputRange InputRange, epochHash [32]byte) (*types.Transaction, error) {
	return _IConsensus.Contract.SubmitClaim(&_IConsensus.TransactOpts, appContract, inputRange, epochHash)
}

// IConsensusClaimAcceptanceIterator is returned from FilterClaimAcceptance and is used to iterate over the raw logs and unpacked data for ClaimAcceptance events raised by the IConsensus contract.
type IConsensusClaimAcceptanceIterator struct {
	Event *IConsensusClaimAcceptance // Event containing the contract specifics and raw log

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
func (it *IConsensusClaimAcceptanceIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IConsensusClaimAcceptance)
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
		it.Event = new(IConsensusClaimAcceptance)
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
func (it *IConsensusClaimAcceptanceIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IConsensusClaimAcceptanceIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IConsensusClaimAcceptance represents a ClaimAcceptance event raised by the IConsensus contract.
type IConsensusClaimAcceptance struct {
	AppContract common.Address
	InputRange  InputRange
	EpochHash   [32]byte
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterClaimAcceptance is a free log retrieval operation binding the contract event 0x4e068a6b8ed35e6ee03244135874f91ccebb5cd1f3a258a6dc2ad0ebd2988476.
//
// Solidity: event ClaimAcceptance(address indexed appContract, (uint64,uint64) inputRange, bytes32 epochHash)
func (_IConsensus *IConsensusFilterer) FilterClaimAcceptance(opts *bind.FilterOpts, appContract []common.Address) (*IConsensusClaimAcceptanceIterator, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IConsensus.contract.FilterLogs(opts, "ClaimAcceptance", appContractRule)
	if err != nil {
		return nil, err
	}
	return &IConsensusClaimAcceptanceIterator{contract: _IConsensus.contract, event: "ClaimAcceptance", logs: logs, sub: sub}, nil
}

// WatchClaimAcceptance is a free log subscription operation binding the contract event 0x4e068a6b8ed35e6ee03244135874f91ccebb5cd1f3a258a6dc2ad0ebd2988476.
//
// Solidity: event ClaimAcceptance(address indexed appContract, (uint64,uint64) inputRange, bytes32 epochHash)
func (_IConsensus *IConsensusFilterer) WatchClaimAcceptance(opts *bind.WatchOpts, sink chan<- *IConsensusClaimAcceptance, appContract []common.Address) (event.Subscription, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IConsensus.contract.WatchLogs(opts, "ClaimAcceptance", appContractRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IConsensusClaimAcceptance)
				if err := _IConsensus.contract.UnpackLog(event, "ClaimAcceptance", log); err != nil {
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

// ParseClaimAcceptance is a log parse operation binding the contract event 0x4e068a6b8ed35e6ee03244135874f91ccebb5cd1f3a258a6dc2ad0ebd2988476.
//
// Solidity: event ClaimAcceptance(address indexed appContract, (uint64,uint64) inputRange, bytes32 epochHash)
func (_IConsensus *IConsensusFilterer) ParseClaimAcceptance(log types.Log) (*IConsensusClaimAcceptance, error) {
	event := new(IConsensusClaimAcceptance)
	if err := _IConsensus.contract.UnpackLog(event, "ClaimAcceptance", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IConsensusClaimSubmissionIterator is returned from FilterClaimSubmission and is used to iterate over the raw logs and unpacked data for ClaimSubmission events raised by the IConsensus contract.
type IConsensusClaimSubmissionIterator struct {
	Event *IConsensusClaimSubmission // Event containing the contract specifics and raw log

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
func (it *IConsensusClaimSubmissionIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IConsensusClaimSubmission)
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
		it.Event = new(IConsensusClaimSubmission)
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
func (it *IConsensusClaimSubmissionIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IConsensusClaimSubmissionIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IConsensusClaimSubmission represents a ClaimSubmission event raised by the IConsensus contract.
type IConsensusClaimSubmission struct {
	Submitter   common.Address
	AppContract common.Address
	InputRange  InputRange
	EpochHash   [32]byte
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterClaimSubmission is a free log retrieval operation binding the contract event 0x940326476a755934b6ae9d2b36ffcf1f447c3a8223f6d9f8a796b54fbfcce582.
//
// Solidity: event ClaimSubmission(address indexed submitter, address indexed appContract, (uint64,uint64) inputRange, bytes32 epochHash)
func (_IConsensus *IConsensusFilterer) FilterClaimSubmission(opts *bind.FilterOpts, submitter []common.Address, appContract []common.Address) (*IConsensusClaimSubmissionIterator, error) {

	var submitterRule []interface{}
	for _, submitterItem := range submitter {
		submitterRule = append(submitterRule, submitterItem)
	}
	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IConsensus.contract.FilterLogs(opts, "ClaimSubmission", submitterRule, appContractRule)
	if err != nil {
		return nil, err
	}
	return &IConsensusClaimSubmissionIterator{contract: _IConsensus.contract, event: "ClaimSubmission", logs: logs, sub: sub}, nil
}

// WatchClaimSubmission is a free log subscription operation binding the contract event 0x940326476a755934b6ae9d2b36ffcf1f447c3a8223f6d9f8a796b54fbfcce582.
//
// Solidity: event ClaimSubmission(address indexed submitter, address indexed appContract, (uint64,uint64) inputRange, bytes32 epochHash)
func (_IConsensus *IConsensusFilterer) WatchClaimSubmission(opts *bind.WatchOpts, sink chan<- *IConsensusClaimSubmission, submitter []common.Address, appContract []common.Address) (event.Subscription, error) {

	var submitterRule []interface{}
	for _, submitterItem := range submitter {
		submitterRule = append(submitterRule, submitterItem)
	}
	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IConsensus.contract.WatchLogs(opts, "ClaimSubmission", submitterRule, appContractRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IConsensusClaimSubmission)
				if err := _IConsensus.contract.UnpackLog(event, "ClaimSubmission", log); err != nil {
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

// ParseClaimSubmission is a log parse operation binding the contract event 0x940326476a755934b6ae9d2b36ffcf1f447c3a8223f6d9f8a796b54fbfcce582.
//
// Solidity: event ClaimSubmission(address indexed submitter, address indexed appContract, (uint64,uint64) inputRange, bytes32 epochHash)
func (_IConsensus *IConsensusFilterer) ParseClaimSubmission(log types.Log) (*IConsensusClaimSubmission, error) {
	event := new(IConsensusClaimSubmission)
	if err := _IConsensus.contract.UnpackLog(event, "ClaimSubmission", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
