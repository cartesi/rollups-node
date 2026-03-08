// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package iauthority

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

// IAuthorityMetaData contains all meta data concerning the IAuthority contract.
var IAuthorityMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"getEpochLength\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNumberOfAcceptedClaims\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isOutputsMerkleRootValid\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"submitClaim\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"supportsInterface\",\"inputs\":[{\"name\":\"interfaceId\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"ClaimAccepted\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ClaimSubmitted\",\"inputs\":[{\"name\":\"submitter\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"appContract\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"NotEpochFinalBlock\",\"inputs\":[{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"epochLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NotFirstClaim\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NotPastBlock\",\"inputs\":[{\"name\":\"lastProcessedBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentBlockNumber\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]",
}

// IAuthorityABI is the input ABI used to generate the binding from.
// Deprecated: Use IAuthorityMetaData.ABI instead.
var IAuthorityABI = IAuthorityMetaData.ABI

// IAuthority is an auto generated Go binding around an Ethereum contract.
type IAuthority struct {
	IAuthorityCaller     // Read-only binding to the contract
	IAuthorityTransactor // Write-only binding to the contract
	IAuthorityFilterer   // Log filterer for contract events
}

// IAuthorityCaller is an auto generated read-only Go binding around an Ethereum contract.
type IAuthorityCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IAuthorityTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IAuthorityTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IAuthorityFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IAuthorityFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IAuthoritySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IAuthoritySession struct {
	Contract     *IAuthority       // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IAuthorityCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IAuthorityCallerSession struct {
	Contract *IAuthorityCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts     // Call options to use throughout this session
}

// IAuthorityTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IAuthorityTransactorSession struct {
	Contract     *IAuthorityTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// IAuthorityRaw is an auto generated low-level Go binding around an Ethereum contract.
type IAuthorityRaw struct {
	Contract *IAuthority // Generic contract binding to access the raw methods on
}

// IAuthorityCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IAuthorityCallerRaw struct {
	Contract *IAuthorityCaller // Generic read-only contract binding to access the raw methods on
}

// IAuthorityTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IAuthorityTransactorRaw struct {
	Contract *IAuthorityTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIAuthority creates a new instance of IAuthority, bound to a specific deployed contract.
func NewIAuthority(address common.Address, backend bind.ContractBackend) (*IAuthority, error) {
	contract, err := bindIAuthority(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IAuthority{IAuthorityCaller: IAuthorityCaller{contract: contract}, IAuthorityTransactor: IAuthorityTransactor{contract: contract}, IAuthorityFilterer: IAuthorityFilterer{contract: contract}}, nil
}

// NewIAuthorityCaller creates a new read-only instance of IAuthority, bound to a specific deployed contract.
func NewIAuthorityCaller(address common.Address, caller bind.ContractCaller) (*IAuthorityCaller, error) {
	contract, err := bindIAuthority(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IAuthorityCaller{contract: contract}, nil
}

// NewIAuthorityTransactor creates a new write-only instance of IAuthority, bound to a specific deployed contract.
func NewIAuthorityTransactor(address common.Address, transactor bind.ContractTransactor) (*IAuthorityTransactor, error) {
	contract, err := bindIAuthority(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IAuthorityTransactor{contract: contract}, nil
}

// NewIAuthorityFilterer creates a new log filterer instance of IAuthority, bound to a specific deployed contract.
func NewIAuthorityFilterer(address common.Address, filterer bind.ContractFilterer) (*IAuthorityFilterer, error) {
	contract, err := bindIAuthority(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IAuthorityFilterer{contract: contract}, nil
}

// bindIAuthority binds a generic wrapper to an already deployed contract.
func bindIAuthority(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IAuthorityMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IAuthority *IAuthorityRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IAuthority.Contract.IAuthorityCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IAuthority *IAuthorityRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IAuthority.Contract.IAuthorityTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IAuthority *IAuthorityRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IAuthority.Contract.IAuthorityTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IAuthority *IAuthorityCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IAuthority.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IAuthority *IAuthorityTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IAuthority.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IAuthority *IAuthorityTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IAuthority.Contract.contract.Transact(opts, method, params...)
}

// GetEpochLength is a free data retrieval call binding the contract method 0xcfe8a73b.
//
// Solidity: function getEpochLength() view returns(uint256)
func (_IAuthority *IAuthorityCaller) GetEpochLength(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IAuthority.contract.Call(opts, &out, "getEpochLength")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetEpochLength is a free data retrieval call binding the contract method 0xcfe8a73b.
//
// Solidity: function getEpochLength() view returns(uint256)
func (_IAuthority *IAuthoritySession) GetEpochLength() (*big.Int, error) {
	return _IAuthority.Contract.GetEpochLength(&_IAuthority.CallOpts)
}

// GetEpochLength is a free data retrieval call binding the contract method 0xcfe8a73b.
//
// Solidity: function getEpochLength() view returns(uint256)
func (_IAuthority *IAuthorityCallerSession) GetEpochLength() (*big.Int, error) {
	return _IAuthority.Contract.GetEpochLength(&_IAuthority.CallOpts)
}

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0xd574f4d7.
//
// Solidity: function getNumberOfAcceptedClaims() view returns(uint256)
func (_IAuthority *IAuthorityCaller) GetNumberOfAcceptedClaims(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IAuthority.contract.Call(opts, &out, "getNumberOfAcceptedClaims")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0xd574f4d7.
//
// Solidity: function getNumberOfAcceptedClaims() view returns(uint256)
func (_IAuthority *IAuthoritySession) GetNumberOfAcceptedClaims() (*big.Int, error) {
	return _IAuthority.Contract.GetNumberOfAcceptedClaims(&_IAuthority.CallOpts)
}

// GetNumberOfAcceptedClaims is a free data retrieval call binding the contract method 0xd574f4d7.
//
// Solidity: function getNumberOfAcceptedClaims() view returns(uint256)
func (_IAuthority *IAuthorityCallerSession) GetNumberOfAcceptedClaims() (*big.Int, error) {
	return _IAuthority.Contract.GetNumberOfAcceptedClaims(&_IAuthority.CallOpts)
}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IAuthority *IAuthorityCaller) IsOutputsMerkleRootValid(opts *bind.CallOpts, appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	var out []interface{}
	err := _IAuthority.contract.Call(opts, &out, "isOutputsMerkleRootValid", appContract, outputsMerkleRoot)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IAuthority *IAuthoritySession) IsOutputsMerkleRootValid(appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	return _IAuthority.Contract.IsOutputsMerkleRootValid(&_IAuthority.CallOpts, appContract, outputsMerkleRoot)
}

// IsOutputsMerkleRootValid is a free data retrieval call binding the contract method 0xe5cc8664.
//
// Solidity: function isOutputsMerkleRootValid(address appContract, bytes32 outputsMerkleRoot) view returns(bool)
func (_IAuthority *IAuthorityCallerSession) IsOutputsMerkleRootValid(appContract common.Address, outputsMerkleRoot [32]byte) (bool, error) {
	return _IAuthority.Contract.IsOutputsMerkleRootValid(&_IAuthority.CallOpts, appContract, outputsMerkleRoot)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_IAuthority *IAuthorityCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IAuthority.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_IAuthority *IAuthoritySession) Owner() (common.Address, error) {
	return _IAuthority.Contract.Owner(&_IAuthority.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_IAuthority *IAuthorityCallerSession) Owner() (common.Address, error) {
	return _IAuthority.Contract.Owner(&_IAuthority.CallOpts)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IAuthority *IAuthorityCaller) SupportsInterface(opts *bind.CallOpts, interfaceId [4]byte) (bool, error) {
	var out []interface{}
	err := _IAuthority.contract.Call(opts, &out, "supportsInterface", interfaceId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IAuthority *IAuthoritySession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _IAuthority.Contract.SupportsInterface(&_IAuthority.CallOpts, interfaceId)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_IAuthority *IAuthorityCallerSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _IAuthority.Contract.SupportsInterface(&_IAuthority.CallOpts, interfaceId)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_IAuthority *IAuthorityTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IAuthority.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_IAuthority *IAuthoritySession) RenounceOwnership() (*types.Transaction, error) {
	return _IAuthority.Contract.RenounceOwnership(&_IAuthority.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_IAuthority *IAuthorityTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _IAuthority.Contract.RenounceOwnership(&_IAuthority.TransactOpts)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x6470af00.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot) returns()
func (_IAuthority *IAuthorityTransactor) SubmitClaim(opts *bind.TransactOpts, appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IAuthority.contract.Transact(opts, "submitClaim", appContract, lastProcessedBlockNumber, outputsMerkleRoot)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x6470af00.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot) returns()
func (_IAuthority *IAuthoritySession) SubmitClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IAuthority.Contract.SubmitClaim(&_IAuthority.TransactOpts, appContract, lastProcessedBlockNumber, outputsMerkleRoot)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0x6470af00.
//
// Solidity: function submitClaim(address appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot) returns()
func (_IAuthority *IAuthorityTransactorSession) SubmitClaim(appContract common.Address, lastProcessedBlockNumber *big.Int, outputsMerkleRoot [32]byte) (*types.Transaction, error) {
	return _IAuthority.Contract.SubmitClaim(&_IAuthority.TransactOpts, appContract, lastProcessedBlockNumber, outputsMerkleRoot)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_IAuthority *IAuthorityTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _IAuthority.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_IAuthority *IAuthoritySession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _IAuthority.Contract.TransferOwnership(&_IAuthority.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_IAuthority *IAuthorityTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _IAuthority.Contract.TransferOwnership(&_IAuthority.TransactOpts, newOwner)
}

// IAuthorityClaimAcceptedIterator is returned from FilterClaimAccepted and is used to iterate over the raw logs and unpacked data for ClaimAccepted events raised by the IAuthority contract.
type IAuthorityClaimAcceptedIterator struct {
	Event *IAuthorityClaimAccepted // Event containing the contract specifics and raw log

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
func (it *IAuthorityClaimAcceptedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IAuthorityClaimAccepted)
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
		it.Event = new(IAuthorityClaimAccepted)
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
func (it *IAuthorityClaimAcceptedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IAuthorityClaimAcceptedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IAuthorityClaimAccepted represents a ClaimAccepted event raised by the IAuthority contract.
type IAuthorityClaimAccepted struct {
	AppContract              common.Address
	LastProcessedBlockNumber *big.Int
	OutputsMerkleRoot        [32]byte
	Raw                      types.Log // Blockchain specific contextual infos
}

// FilterClaimAccepted is a free log retrieval operation binding the contract event 0x0f2cd00a405c0d1a66050307b6722c4788db6ed57aa3589a5c38da535cc3ce63.
//
// Solidity: event ClaimAccepted(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IAuthority *IAuthorityFilterer) FilterClaimAccepted(opts *bind.FilterOpts, appContract []common.Address) (*IAuthorityClaimAcceptedIterator, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IAuthority.contract.FilterLogs(opts, "ClaimAccepted", appContractRule)
	if err != nil {
		return nil, err
	}
	return &IAuthorityClaimAcceptedIterator{contract: _IAuthority.contract, event: "ClaimAccepted", logs: logs, sub: sub}, nil
}

// WatchClaimAccepted is a free log subscription operation binding the contract event 0x0f2cd00a405c0d1a66050307b6722c4788db6ed57aa3589a5c38da535cc3ce63.
//
// Solidity: event ClaimAccepted(address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IAuthority *IAuthorityFilterer) WatchClaimAccepted(opts *bind.WatchOpts, sink chan<- *IAuthorityClaimAccepted, appContract []common.Address) (event.Subscription, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IAuthority.contract.WatchLogs(opts, "ClaimAccepted", appContractRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IAuthorityClaimAccepted)
				if err := _IAuthority.contract.UnpackLog(event, "ClaimAccepted", log); err != nil {
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
func (_IAuthority *IAuthorityFilterer) ParseClaimAccepted(log types.Log) (*IAuthorityClaimAccepted, error) {
	event := new(IAuthorityClaimAccepted)
	if err := _IAuthority.contract.UnpackLog(event, "ClaimAccepted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IAuthorityClaimSubmittedIterator is returned from FilterClaimSubmitted and is used to iterate over the raw logs and unpacked data for ClaimSubmitted events raised by the IAuthority contract.
type IAuthorityClaimSubmittedIterator struct {
	Event *IAuthorityClaimSubmitted // Event containing the contract specifics and raw log

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
func (it *IAuthorityClaimSubmittedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IAuthorityClaimSubmitted)
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
		it.Event = new(IAuthorityClaimSubmitted)
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
func (it *IAuthorityClaimSubmittedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IAuthorityClaimSubmittedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IAuthorityClaimSubmitted represents a ClaimSubmitted event raised by the IAuthority contract.
type IAuthorityClaimSubmitted struct {
	Submitter                common.Address
	AppContract              common.Address
	LastProcessedBlockNumber *big.Int
	OutputsMerkleRoot        [32]byte
	Raw                      types.Log // Blockchain specific contextual infos
}

// FilterClaimSubmitted is a free log retrieval operation binding the contract event 0xf4ff953641f10e17dd93c0bc51334cb1f711fdcb4e37992021a5973f7a958f09.
//
// Solidity: event ClaimSubmitted(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IAuthority *IAuthorityFilterer) FilterClaimSubmitted(opts *bind.FilterOpts, submitter []common.Address, appContract []common.Address) (*IAuthorityClaimSubmittedIterator, error) {

	var submitterRule []interface{}
	for _, submitterItem := range submitter {
		submitterRule = append(submitterRule, submitterItem)
	}
	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IAuthority.contract.FilterLogs(opts, "ClaimSubmitted", submitterRule, appContractRule)
	if err != nil {
		return nil, err
	}
	return &IAuthorityClaimSubmittedIterator{contract: _IAuthority.contract, event: "ClaimSubmitted", logs: logs, sub: sub}, nil
}

// WatchClaimSubmitted is a free log subscription operation binding the contract event 0xf4ff953641f10e17dd93c0bc51334cb1f711fdcb4e37992021a5973f7a958f09.
//
// Solidity: event ClaimSubmitted(address indexed submitter, address indexed appContract, uint256 lastProcessedBlockNumber, bytes32 outputsMerkleRoot)
func (_IAuthority *IAuthorityFilterer) WatchClaimSubmitted(opts *bind.WatchOpts, sink chan<- *IAuthorityClaimSubmitted, submitter []common.Address, appContract []common.Address) (event.Subscription, error) {

	var submitterRule []interface{}
	for _, submitterItem := range submitter {
		submitterRule = append(submitterRule, submitterItem)
	}
	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}

	logs, sub, err := _IAuthority.contract.WatchLogs(opts, "ClaimSubmitted", submitterRule, appContractRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IAuthorityClaimSubmitted)
				if err := _IAuthority.contract.UnpackLog(event, "ClaimSubmitted", log); err != nil {
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
func (_IAuthority *IAuthorityFilterer) ParseClaimSubmitted(log types.Log) (*IAuthorityClaimSubmitted, error) {
	event := new(IAuthorityClaimSubmitted)
	if err := _IAuthority.contract.UnpackLog(event, "ClaimSubmitted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
