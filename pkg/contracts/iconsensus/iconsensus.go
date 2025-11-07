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
	ABI: "[{\"type\":\"function\",\"name\":\"getEpochLength\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNumberOfAcceptedClaims\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isOutputsMerkleRootValid\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"submitClaim\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"supportsInterface\",\"inputs\":[{\"name\":\"interfaceId\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"ClaimAccepted\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ClaimSubmitted\",\"inputs\":[{\"name\":\"submitter\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"NotEpochFinalBlock\",\"inputs\":[{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"epochLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NotFirstClaim\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NotPastBlock\",\"inputs\":[{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]",
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

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0xd574f4d7.
//
// Solidity: function getNumberOfAcceptedClaims() view returns(uint256)
func (_IConsensus *IConsensusCaller) GetNumberOfAcceptedClaims(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "getNumberOfAcceptedClaims")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0xd574f4d7.
//
// Solidity: function getNumberOfAcceptedClaims() view returns(uint256)
func (_IConsensus *IConsensusSession) GetNumberOfAcceptedClaims() (*big.Int, error) {
	return _IConsensus.Contract.GetNumberOfAcceptedClaims(&_IConsensus.CallOpts)
}

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0xd574f4d7.
//
// Solidity: function getNumberOfAcceptedClaims() view returns(uint256)
func (_IConsensus *IConsensusCallerSession) GetNumberOfAcceptedClaims() (*big.Int, error) {
	return _IConsensus.Contract.GetNumberOfAcceptedClaims(&_IConsensus.CallOpts)
}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IConsensus *IConsensusCaller) IsOutputsMerkleRootValid(opts *bind.CallOpts, appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "isOutputsMerkleRootValid", appContract, outputsMerkleRoot)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IConsensus *IConsensusSession) IsOutputsMerkleRootValid(appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	return _IConsensus.Contract.IsOutputsMerkleRootValid(&_IConsensus.CallOpts, appContract, outputsMerkleRoot)
}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IConsensus *IConsensusCallerSession) IsOutputsMerkleRootValid(appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	return _IConsensus.Contract.IsOutputsMerkleRootValid(&_IConsensus.CallOpts, appContract, outputsMerkleRoot)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IConsensus *IConsensusCaller) SupportsInterface(opts *bind.CallOpts, interfaceId [4]byte) (bool, error) {
	var out []interface{}
	err := _IConsensus.contract.Call(opts, &out, "supportsInterface", interfaceId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IConsensus *IConsensusSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _IConsensus.Contract.SupportsInterface(&_IConsensus.CallOpts, interfaceId)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IConsensus *IConsensusCallerSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _IConsensus.Contract.SupportsInterface(&_IConsensus.CallOpts, interfaceId)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x6470af00.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot) returns()
func (_IConsensus *IConsensusTransactor) SubmitClaim(opts *bind.TransactOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IConsensus.contract.Transact(opts, "submitClaim", appContract, lastProcessedBlockNumber, outputsMerkleRoot)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x6470af00.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot) returns()
func (_IConsensus *IConsensusSession) SubmitClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IConsensus.Contract.SubmitClaim(&_IConsensus.TransactOpts, appContract, lastProcessedBlockNumber, outputsMerkleRoot)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x6470af00.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot) returns()
func (_IConsensus *IConsensusTransactorSession) SubmitClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IConsensus.Contract.SubmitClaim(&_IConsensus.TransactOpts, appContract, lastProcessedBlockNumber, outputsMerkleRoot)
}

// IConsensusClaimAcceptedIterator is returned from FilterClaimAccepted and is used to iterate over the raw logs and unpacked data for ClaimAccepted events raised by the IConsensus contract.
type IConsensusClaimAcceptedIterator struct {
	Event *IConsensusClaimAccepted // Event containing the contract specifics and raw log

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
func (it *IConsensusClaimAcceptedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IConsensusClaimAccepted)
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
		it.Event = new(IConsensusClaimAccepted)
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
func (it *IConsensusClaimAcceptedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IConsensusClaimAcceptedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IConsensusClaimAccepted represents a ClaimAccepted event raised by the IConsensus contract.
type IConsensusClaimAccepted struct {
	AppContract              common.Address
	LastProcessedBlockNumber *big.Int
	OutputsMerkleRoot        [32]byte
	Raw                      types.Log // Blockchain specific contextual infos
}

// FilterClaimAccepted is a free log retrieval operation binding the contract event 0x0f2cd00a405c0d1a66050307b6722c4788db6ed57aa3589a5c38da535cc3ce63.
//
// Solidity: event ClaimAccepted(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IConsensus *IConsensusFilterer) FilterClaimAccepted(opts *bind.FilterOpts, appContract []common.Address) (*IConsensusClaimAcceptedIterator, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IConsensus.contract.FilterLogs(opts, "ClaimAccepted", appContractRule)
	if err != nil {
		return nil, err
	}
	return &IConsensusClaimAcceptedIterator{contract: _IConsensus.contract, event: "ClaimAccepted", logs: logs, sub: sub}, nil
}

// WatchClaimAccepted is a free log subscription operation binding the contract event 0x0f2cd00a405c0d1a66050307b6722c4788db6ed57aa3589a5c38da535cc3ce63.
//
// Solidity: event ClaimAccepted(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IConsensus *IConsensusFilterer) WatchClaimAccepted(opts *bind.WatchOpts, sink chan<- *IConsensusClaimAccepted, appContract []common.Address) (event.Subscription, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IConsensus.contract.WatchLogs(opts, "ClaimAccepted", appContractRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IConsensusClaimAccepted)
				if err := _IConsensus.contract.UnpackLog(event, "ClaimAccepted", log); err != nil {
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

// ParseClaimAccepted is a log parse operation binding the contract event 0x0f2cd00a405c0d1a66050307b6722c4788db6ed57aa3589a5c38da535cc3ce63.
//
// Solidity: event ClaimAccepted(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IConsensus *IConsensusFilterer) ParseClaimAccepted(log types.Log) (*IConsensusClaimAccepted, error) {
	event := new(IConsensusClaimAccepted)
	if err := _IConsensus.contract.UnpackLog(event, "ClaimAccepted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IConsensusClaimSubmittedIterator is returned from FilterClaimSubmitted and is used to iterate over the raw logs and unpacked data for ClaimSubmitted events raised by the IConsensus contract.
type IConsensusClaimSubmittedIterator struct {
	Event *IConsensusClaimSubmitted // Event containing the contract specifics and raw log

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
func (it *IConsensusClaimSubmittedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IConsensusClaimSubmitted)
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
		it.Event = new(IConsensusClaimSubmitted)
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
func (it *IConsensusClaimSubmittedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IConsensusClaimSubmittedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IConsensusClaimSubmitted represents a ClaimSubmitted event raised by the IConsensus contract.
type IConsensusClaimSubmitted struct {
	Submitter                common.Address
	AppContract              common.Address
	LastProcessedBlockNumber *big.Int
	OutputsMerkleRoot        [32]byte
	Raw                      types.Log // Blockchain specific contextual infos
}

// FilterClaimSubmitted is a free log retrieval operation binding the contract event 0xf4ff953641f10e17dd93c0bc51334cb1f711fdcb4e37992021a5973f7a958f09.
//
// Solidity: event ClaimSubmitted(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IConsensus *IConsensusFilterer) FilterClaimSubmitted(opts *bind.FilterOpts, submitter []common.Address, appContract []common.Address) (*IConsensusClaimSubmittedIterator, error) {

	var submitterRule []interface{}
	for _, submitterItem := range submitter {
		submitterRule = append(submitterRule, submitterItem)
	}
	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IConsensus.contract.FilterLogs(opts, "ClaimSubmitted", submitterRule, appContractRule)
	if err != nil {
		return nil, err
	}
	return &IConsensusClaimSubmittedIterator{contract: _IConsensus.contract, event: "ClaimSubmitted", logs: logs, sub: sub}, nil
}

// WatchClaimSubmitted is a free log subscription operation binding the contract event 0xf4ff953641f10e17dd93c0bc51334cb1f711fdcb4e37992021a5973f7a958f09.
//
// Solidity: event ClaimSubmitted(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IConsensus *IConsensusFilterer) WatchClaimSubmitted(opts *bind.WatchOpts, sink chan<- *IConsensusClaimSubmitted, submitter []common.Address, appContract []common.Address) (event.Subscription, error) {

	var submitterRule []interface{}
	for _, submitterItem := range submitter {
		submitterRule = append(submitterRule, submitterItem)
	}
	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IConsensus.contract.WatchLogs(opts, "ClaimSubmitted", submitterRule, appContractRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IConsensusClaimSubmitted)
				if err := _IConsensus.contract.UnpackLog(event, "ClaimSubmitted", log); err != nil {
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

// ParseClaimSubmitted is a log parse operation binding the contract event 0xf4ff953641f10e17dd93c0bc51334cb1f711fdcb4e37992021a5973f7a958f09.
//
// Solidity: event ClaimSubmitted(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IConsensus *IConsensusFilterer) ParseClaimSubmitted(log types.Log) (*IConsensusClaimSubmitted, error) {
	event := new(IConsensusClaimSubmitted)
	if err := _IConsensus.contract.UnpackLog(event, "ClaimSubmitted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
