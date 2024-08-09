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

// IConsensusMetaData contains all meta data concerning the IConsensus contract.
var IConsensusMetaData = &bind.MetaData{
	ABI: "[{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"appContract\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"claim\",\"type\":\"bytes32\"}],\"name\":\"ClaimAcceptance\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"submitter\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"appContract\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"claim\",\"type\":\"bytes32\"}],\"name\":\"ClaimSubmission\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"getEpochLength\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"appContract\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"claim\",\"type\":\"bytes32\"}],\"name\":\"submitClaim\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"appContract\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"claim\",\"type\":\"bytes32\"}],\"name\":\"wasClaimAccepted\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
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

// GetEpochLength is a free data retrieval call binding the contract method 0xcfe8a73b.
//
// Solidity: function getEpochLength() view returns(uint256)
func (_IConsensus *IConsensusCaller) GetEpochLength(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "getEpochLength")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetEpochLength is a free data retrieval call binding the contract method 0xcfe8a73b.
//
// Solidity: function getEpochLength() view returns(uint256)
func (_IConsensus *IConsensusSession) GetEpochLength() (*big.Int, error) {
	return _IConsensus.Contract.GetEpochLength(&_IConsensus.CallOpts)
}

// GetEpochLength is a free data retrieval call binding the contract method 0xcfe8a73b.
//
// Solidity: function getEpochLength() view returns(uint256)
func (_IConsensus *IConsensusCallerSession) GetEpochLength() (*big.Int, error) {
	return _IConsensus.Contract.GetEpochLength(&_IConsensus.CallOpts)
}

// WasClaimAccepted is a free data retrieval call binding the contract method 0x9618f35b.
//
// Solidity: function wasClaimAccepted(address appContract, bytes32 claim) view returns(bool)
func (_IConsensus *IConsensusCaller) WasClaimAccepted(opts *bind.CallOpts, appContract common.Address, claim [32]byte) (bool, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "wasClaimAccepted", appContract, claim)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// WasClaimAccepted is a free data retrieval call binding the contract method 0x9618f35b.
//
// Solidity: function wasClaimAccepted(address appContract, bytes32 claim) view returns(bool)
func (_IConsensus *IConsensusSession) WasClaimAccepted(appContract common.Address, claim [32]byte) (bool, error) {
	return _IConsensus.Contract.WasClaimAccepted(&_IConsensus.CallOpts, appContract, claim)
}

// WasClaimAccepted is a free data retrieval call binding the contract method 0x9618f35b.
//
// Solidity: function wasClaimAccepted(address appContract, bytes32 claim) view returns(bool)
func (_IConsensus *IConsensusCallerSession) WasClaimAccepted(appContract common.Address, claim [32]byte) (bool, error) {
	return _IConsensus.Contract.WasClaimAccepted(&_IConsensus.CallOpts, appContract, claim)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x6470af00.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 claim) returns()
func (_IConsensus *IConsensusTransactor) SubmitClaim(opts *bind.TransactOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, claim [32]byte) (*types.Transaction, error) {
	return _IConsensus.contract.Transact(opts, "submitClaim", appContract, lastProcessedBlockNumber, claim)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x6470af00.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 claim) returns()
func (_IConsensus *IConsensusSession) SubmitClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, claim [32]byte) (*types.Transaction, error) {
	return _IConsensus.Contract.SubmitClaim(&_IConsensus.TransactOpts, appContract, lastProcessedBlockNumber, claim)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x6470af00.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 claim) returns()
func (_IConsensus *IConsensusTransactorSession) SubmitClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, claim [32]byte) (*types.Transaction, error) {
	return _IConsensus.Contract.SubmitClaim(&_IConsensus.TransactOpts, appContract, lastProcessedBlockNumber, claim)
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
	AppContract              common.Address
	LastProcessedBlockNumber *big.Int
	Claim                    [32]byte
	Raw                      types.Log // Blockchain specific contextual infos
}

// FilterClaimAcceptance is a free log retrieval operation binding the contract event 0xd3e4892959c6ddb27e02bcaaebc0c1898d0f677b7360bf80339f10a8717957d3.
//
// Solidity: event ClaimAcceptance(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 claim)
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

// WatchClaimAcceptance is a free log subscription operation binding the contract event 0xd3e4892959c6ddb27e02bcaaebc0c1898d0f677b7360bf80339f10a8717957d3.
//
// Solidity: event ClaimAcceptance(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 claim)
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

// ParseClaimAcceptance is a log parse operation binding the contract event 0xd3e4892959c6ddb27e02bcaaebc0c1898d0f677b7360bf80339f10a8717957d3.
//
// Solidity: event ClaimAcceptance(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 claim)
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
	Submitter                common.Address
	AppContract              common.Address
	LastProcessedBlockNumber *big.Int
	Claim                    [32]byte
	Raw                      types.Log // Blockchain specific contextual infos
}

// FilterClaimSubmission is a free log retrieval operation binding the contract event 0xf5a28e07a1b89d1ca3f9a2a7ef16bd650503a4791baf2e70dc401c21ee505f0a.
//
// Solidity: event ClaimSubmission(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 claim)
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

// WatchClaimSubmission is a free log subscription operation binding the contract event 0xf5a28e07a1b89d1ca3f9a2a7ef16bd650503a4791baf2e70dc401c21ee505f0a.
//
// Solidity: event ClaimSubmission(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 claim)
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

// ParseClaimSubmission is a log parse operation binding the contract event 0xf5a28e07a1b89d1ca3f9a2a7ef16bd650503a4791baf2e70dc401c21ee505f0a.
//
// Solidity: event ClaimSubmission(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 claim)
func (_IConsensus *IConsensusFilterer) ParseClaimSubmission(log types.Log) (*IConsensusClaimSubmission, error) {
	event := new(IConsensusClaimSubmission)
	if err := _IConsensus.contract.UnpackLog(event, "ClaimSubmission", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
