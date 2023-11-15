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

// AuthorityMetaData contains all meta data concerning the Authority contract.
var AuthorityMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_owner\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"AuthorityWithdrawalFailed\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"application\",\"type\":\"address\"}],\"name\":\"ApplicationJoined\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"contractIHistory\",\"name\":\"history\",\"type\":\"address\"}],\"name\":\"NewHistory\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"previousOwner\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"OwnershipTransferred\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_dapp\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"_proofContext\",\"type\":\"bytes\"}],\"name\":\"getClaim\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"},{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getHistory\",\"outputs\":[{\"internalType\":\"contractIHistory\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"join\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_consensus\",\"type\":\"address\"}],\"name\":\"migrateHistoryToConsensus\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"renounceOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"contractIHistory\",\"name\":\"_history\",\"type\":\"address\"}],\"name\":\"setHistory\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"_claimData\",\"type\":\"bytes\"}],\"name\":\"submitClaim\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"transferOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"contractIERC20\",\"name\":\"_token\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"_recipient\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"_amount\",\"type\":\"uint256\"}],\"name\":\"withdrawERC20Tokens\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// AuthorityABI is the input ABI used to generate the binding from.
// Deprecated: Use AuthorityMetaData.ABI instead.
var AuthorityABI = AuthorityMetaData.ABI

// Authority is an auto generated Go binding around an Ethereum contract.
type Authority struct {
	AuthorityCaller     // Read-only binding to the contract
	AuthorityTransactor // Write-only binding to the contract
	AuthorityFilterer   // Log filterer for contract events
}

// AuthorityCaller is an auto generated read-only Go binding around an Ethereum contract.
type AuthorityCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// AuthorityTransactor is an auto generated write-only Go binding around an Ethereum contract.
type AuthorityTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// AuthorityFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type AuthorityFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// AuthoritySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type AuthoritySession struct {
	Contract     *Authority        // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// AuthorityCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type AuthorityCallerSession struct {
	Contract *AuthorityCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts    // Call options to use throughout this session
}

// AuthorityTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type AuthorityTransactorSession struct {
	Contract     *AuthorityTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts    // Transaction auth options to use throughout this session
}

// AuthorityRaw is an auto generated low-level Go binding around an Ethereum contract.
type AuthorityRaw struct {
	Contract *Authority // Generic contract binding to access the raw methods on
}

// AuthorityCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type AuthorityCallerRaw struct {
	Contract *AuthorityCaller // Generic read-only contract binding to access the raw methods on
}

// AuthorityTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type AuthorityTransactorRaw struct {
	Contract *AuthorityTransactor // Generic write-only contract binding to access the raw methods on
}

// NewAuthority creates a new instance of Authority, bound to a specific deployed contract.
func NewAuthority(address common.Address, backend bind.ContractBackend) (*Authority, error) {
	contract, err := bindAuthority(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Authority{AuthorityCaller: AuthorityCaller{contract: contract}, AuthorityTransactor: AuthorityTransactor{contract: contract}, AuthorityFilterer: AuthorityFilterer{contract: contract}}, nil
}

// NewAuthorityCaller creates a new read-only instance of Authority, bound to a specific deployed contract.
func NewAuthorityCaller(address common.Address, caller bind.ContractCaller) (*AuthorityCaller, error) {
	contract, err := bindAuthority(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &AuthorityCaller{contract: contract}, nil
}

// NewAuthorityTransactor creates a new write-only instance of Authority, bound to a specific deployed contract.
func NewAuthorityTransactor(address common.Address, transactor bind.ContractTransactor) (*AuthorityTransactor, error) {
	contract, err := bindAuthority(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &AuthorityTransactor{contract: contract}, nil
}

// NewAuthorityFilterer creates a new log filterer instance of Authority, bound to a specific deployed contract.
func NewAuthorityFilterer(address common.Address, filterer bind.ContractFilterer) (*AuthorityFilterer, error) {
	contract, err := bindAuthority(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &AuthorityFilterer{contract: contract}, nil
}

// bindAuthority binds a generic wrapper to an already deployed contract.
func bindAuthority(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := AuthorityMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Authority *AuthorityRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Authority.Contract.AuthorityCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Authority *AuthorityRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Authority.Contract.AuthorityTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Authority *AuthorityRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Authority.Contract.AuthorityTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Authority *AuthorityCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Authority.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Authority *AuthorityTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Authority.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Authority *AuthorityTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Authority.Contract.contract.Transact(opts, method, params...)
}

// GetClaim is a free data retrieval call binding the contract method 0xd79a8240.
//
// Solidity: function getClaim(address _dapp, bytes _proofContext) view returns(bytes32, uint256, uint256)
func (_Authority *AuthorityCaller) GetClaim(opts *bind.CallOpts, _dapp common.Address, _proofContext []byte) ([32]byte, *big.Int, *big.Int, error) {
	var out []interface{}
	err := _Authority.contract.Call(opts, &out, "getClaim", _dapp, _proofContext)

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
func (_Authority *AuthoritySession) GetClaim(_dapp common.Address, _proofContext []byte) ([32]byte, *big.Int, *big.Int, error) {
	return _Authority.Contract.GetClaim(&_Authority.CallOpts, _dapp, _proofContext)
}

// GetClaim is a free data retrieval call binding the contract method 0xd79a8240.
//
// Solidity: function getClaim(address _dapp, bytes _proofContext) view returns(bytes32, uint256, uint256)
func (_Authority *AuthorityCallerSession) GetClaim(_dapp common.Address, _proofContext []byte) ([32]byte, *big.Int, *big.Int, error) {
	return _Authority.Contract.GetClaim(&_Authority.CallOpts, _dapp, _proofContext)
}

// GetHistory is a free data retrieval call binding the contract method 0xaa15efc8.
//
// Solidity: function getHistory() view returns(address)
func (_Authority *AuthorityCaller) GetHistory(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Authority.contract.Call(opts, &out, "getHistory")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetHistory is a free data retrieval call binding the contract method 0xaa15efc8.
//
// Solidity: function getHistory() view returns(address)
func (_Authority *AuthoritySession) GetHistory() (common.Address, error) {
	return _Authority.Contract.GetHistory(&_Authority.CallOpts)
}

// GetHistory is a free data retrieval call binding the contract method 0xaa15efc8.
//
// Solidity: function getHistory() view returns(address)
func (_Authority *AuthorityCallerSession) GetHistory() (common.Address, error) {
	return _Authority.Contract.GetHistory(&_Authority.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Authority *AuthorityCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Authority.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Authority *AuthoritySession) Owner() (common.Address, error) {
	return _Authority.Contract.Owner(&_Authority.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Authority *AuthorityCallerSession) Owner() (common.Address, error) {
	return _Authority.Contract.Owner(&_Authority.CallOpts)
}

// Join is a paid mutator transaction binding the contract method 0xb688a363.
//
// Solidity: function join() returns()
func (_Authority *AuthorityTransactor) Join(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Authority.contract.Transact(opts, "join")
}

// Join is a paid mutator transaction binding the contract method 0xb688a363.
//
// Solidity: function join() returns()
func (_Authority *AuthoritySession) Join() (*types.Transaction, error) {
	return _Authority.Contract.Join(&_Authority.TransactOpts)
}

// Join is a paid mutator transaction binding the contract method 0xb688a363.
//
// Solidity: function join() returns()
func (_Authority *AuthorityTransactorSession) Join() (*types.Transaction, error) {
	return _Authority.Contract.Join(&_Authority.TransactOpts)
}

// MigrateHistoryToConsensus is a paid mutator transaction binding the contract method 0x9368a3d3.
//
// Solidity: function migrateHistoryToConsensus(address _consensus) returns()
func (_Authority *AuthorityTransactor) MigrateHistoryToConsensus(opts *bind.TransactOpts, _consensus common.Address) (*types.Transaction, error) {
	return _Authority.contract.Transact(opts, "migrateHistoryToConsensus", _consensus)
}

// MigrateHistoryToConsensus is a paid mutator transaction binding the contract method 0x9368a3d3.
//
// Solidity: function migrateHistoryToConsensus(address _consensus) returns()
func (_Authority *AuthoritySession) MigrateHistoryToConsensus(_consensus common.Address) (*types.Transaction, error) {
	return _Authority.Contract.MigrateHistoryToConsensus(&_Authority.TransactOpts, _consensus)
}

// MigrateHistoryToConsensus is a paid mutator transaction binding the contract method 0x9368a3d3.
//
// Solidity: function migrateHistoryToConsensus(address _consensus) returns()
func (_Authority *AuthorityTransactorSession) MigrateHistoryToConsensus(_consensus common.Address) (*types.Transaction, error) {
	return _Authority.Contract.MigrateHistoryToConsensus(&_Authority.TransactOpts, _consensus)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_Authority *AuthorityTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Authority.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_Authority *AuthoritySession) RenounceOwnership() (*types.Transaction, error) {
	return _Authority.Contract.RenounceOwnership(&_Authority.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_Authority *AuthorityTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _Authority.Contract.RenounceOwnership(&_Authority.TransactOpts)
}

// SetHistory is a paid mutator transaction binding the contract method 0x159c5ea1.
//
// Solidity: function setHistory(address _history) returns()
func (_Authority *AuthorityTransactor) SetHistory(opts *bind.TransactOpts, _history common.Address) (*types.Transaction, error) {
	return _Authority.contract.Transact(opts, "setHistory", _history)
}

// SetHistory is a paid mutator transaction binding the contract method 0x159c5ea1.
//
// Solidity: function setHistory(address _history) returns()
func (_Authority *AuthoritySession) SetHistory(_history common.Address) (*types.Transaction, error) {
	return _Authority.Contract.SetHistory(&_Authority.TransactOpts, _history)
}

// SetHistory is a paid mutator transaction binding the contract method 0x159c5ea1.
//
// Solidity: function setHistory(address _history) returns()
func (_Authority *AuthorityTransactorSession) SetHistory(_history common.Address) (*types.Transaction, error) {
	return _Authority.Contract.SetHistory(&_Authority.TransactOpts, _history)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0xddfdfbb0.
//
// Solidity: function submitClaim(bytes _claimData) returns()
func (_Authority *AuthorityTransactor) SubmitClaim(opts *bind.TransactOpts, _claimData []byte) (*types.Transaction, error) {
	return _Authority.contract.Transact(opts, "submitClaim", _claimData)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0xddfdfbb0.
//
// Solidity: function submitClaim(bytes _claimData) returns()
func (_Authority *AuthoritySession) SubmitClaim(_claimData []byte) (*types.Transaction, error) {
	return _Authority.Contract.SubmitClaim(&_Authority.TransactOpts, _claimData)
}

// SubmitClaim is a paid mutator transaction binding the contract method 0xddfdfbb0.
//
// Solidity: function submitClaim(bytes _claimData) returns()
func (_Authority *AuthorityTransactorSession) SubmitClaim(_claimData []byte) (*types.Transaction, error) {
	return _Authority.Contract.SubmitClaim(&_Authority.TransactOpts, _claimData)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_Authority *AuthorityTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _Authority.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_Authority *AuthoritySession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _Authority.Contract.TransferOwnership(&_Authority.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_Authority *AuthorityTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _Authority.Contract.TransferOwnership(&_Authority.TransactOpts, newOwner)
}

// WithdrawERC20Tokens is a paid mutator transaction binding the contract method 0xbcdd1e13.
//
// Solidity: function withdrawERC20Tokens(address _token, address _recipient, uint256 _amount) returns()
func (_Authority *AuthorityTransactor) WithdrawERC20Tokens(opts *bind.TransactOpts, _token common.Address, _recipient common.Address, _amount *big.Int) (*types.Transaction, error) {
	return _Authority.contract.Transact(opts, "withdrawERC20Tokens", _token, _recipient, _amount)
}

// WithdrawERC20Tokens is a paid mutator transaction binding the contract method 0xbcdd1e13.
//
// Solidity: function withdrawERC20Tokens(address _token, address _recipient, uint256 _amount) returns()
func (_Authority *AuthoritySession) WithdrawERC20Tokens(_token common.Address, _recipient common.Address, _amount *big.Int) (*types.Transaction, error) {
	return _Authority.Contract.WithdrawERC20Tokens(&_Authority.TransactOpts, _token, _recipient, _amount)
}

// WithdrawERC20Tokens is a paid mutator transaction binding the contract method 0xbcdd1e13.
//
// Solidity: function withdrawERC20Tokens(address _token, address _recipient, uint256 _amount) returns()
func (_Authority *AuthorityTransactorSession) WithdrawERC20Tokens(_token common.Address, _recipient common.Address, _amount *big.Int) (*types.Transaction, error) {
	return _Authority.Contract.WithdrawERC20Tokens(&_Authority.TransactOpts, _token, _recipient, _amount)
}

// AuthorityApplicationJoinedIterator is returned from FilterApplicationJoined and is used to iterate over the raw logs and unpacked data for ApplicationJoined events raised by the Authority contract.
type AuthorityApplicationJoinedIterator struct {
	Event *AuthorityApplicationJoined // Event containing the contract specifics and raw log

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
func (it *AuthorityApplicationJoinedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AuthorityApplicationJoined)
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
		it.Event = new(AuthorityApplicationJoined)
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
func (it *AuthorityApplicationJoinedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AuthorityApplicationJoinedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AuthorityApplicationJoined represents a ApplicationJoined event raised by the Authority contract.
type AuthorityApplicationJoined struct {
	Application common.Address
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterApplicationJoined is a free log retrieval operation binding the contract event 0x27c2b702d3bff195a18baca2daf00b20a986177c5f1449af4e2d46a3c3e02ce5.
//
// Solidity: event ApplicationJoined(address application)
func (_Authority *AuthorityFilterer) FilterApplicationJoined(opts *bind.FilterOpts) (*AuthorityApplicationJoinedIterator, error) {

	logs, sub, err := _Authority.contract.FilterLogs(opts, "ApplicationJoined")
	if err != nil {
		return nil, err
	}
	return &AuthorityApplicationJoinedIterator{contract: _Authority.contract, event: "ApplicationJoined", logs: logs, sub: sub}, nil
}

// WatchApplicationJoined is a free log subscription operation binding the contract event 0x27c2b702d3bff195a18baca2daf00b20a986177c5f1449af4e2d46a3c3e02ce5.
//
// Solidity: event ApplicationJoined(address application)
func (_Authority *AuthorityFilterer) WatchApplicationJoined(opts *bind.WatchOpts, sink chan<- *AuthorityApplicationJoined) (event.Subscription, error) {

	logs, sub, err := _Authority.contract.WatchLogs(opts, "ApplicationJoined")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AuthorityApplicationJoined)
				if err := _Authority.contract.UnpackLog(event, "ApplicationJoined", log); err != nil {
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

// ParseApplicationJoined is a log parse operation binding the contract event 0x27c2b702d3bff195a18baca2daf00b20a986177c5f1449af4e2d46a3c3e02ce5.
//
// Solidity: event ApplicationJoined(address application)
func (_Authority *AuthorityFilterer) ParseApplicationJoined(log types.Log) (*AuthorityApplicationJoined, error) {
	event := new(AuthorityApplicationJoined)
	if err := _Authority.contract.UnpackLog(event, "ApplicationJoined", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// AuthorityNewHistoryIterator is returned from FilterNewHistory and is used to iterate over the raw logs and unpacked data for NewHistory events raised by the Authority contract.
type AuthorityNewHistoryIterator struct {
	Event *AuthorityNewHistory // Event containing the contract specifics and raw log

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
func (it *AuthorityNewHistoryIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AuthorityNewHistory)
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
		it.Event = new(AuthorityNewHistory)
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
func (it *AuthorityNewHistoryIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AuthorityNewHistoryIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AuthorityNewHistory represents a NewHistory event raised by the Authority contract.
type AuthorityNewHistory struct {
	History common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterNewHistory is a free log retrieval operation binding the contract event 0x2bcd43869347a1d42f97ac6042f3d129817abd05a6125f9750fe3724e321d23e.
//
// Solidity: event NewHistory(address history)
func (_Authority *AuthorityFilterer) FilterNewHistory(opts *bind.FilterOpts) (*AuthorityNewHistoryIterator, error) {

	logs, sub, err := _Authority.contract.FilterLogs(opts, "NewHistory")
	if err != nil {
		return nil, err
	}
	return &AuthorityNewHistoryIterator{contract: _Authority.contract, event: "NewHistory", logs: logs, sub: sub}, nil
}

// WatchNewHistory is a free log subscription operation binding the contract event 0x2bcd43869347a1d42f97ac6042f3d129817abd05a6125f9750fe3724e321d23e.
//
// Solidity: event NewHistory(address history)
func (_Authority *AuthorityFilterer) WatchNewHistory(opts *bind.WatchOpts, sink chan<- *AuthorityNewHistory) (event.Subscription, error) {

	logs, sub, err := _Authority.contract.WatchLogs(opts, "NewHistory")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AuthorityNewHistory)
				if err := _Authority.contract.UnpackLog(event, "NewHistory", log); err != nil {
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

// ParseNewHistory is a log parse operation binding the contract event 0x2bcd43869347a1d42f97ac6042f3d129817abd05a6125f9750fe3724e321d23e.
//
// Solidity: event NewHistory(address history)
func (_Authority *AuthorityFilterer) ParseNewHistory(log types.Log) (*AuthorityNewHistory, error) {
	event := new(AuthorityNewHistory)
	if err := _Authority.contract.UnpackLog(event, "NewHistory", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// AuthorityOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the Authority contract.
type AuthorityOwnershipTransferredIterator struct {
	Event *AuthorityOwnershipTransferred // Event containing the contract specifics and raw log

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
func (it *AuthorityOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AuthorityOwnershipTransferred)
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
		it.Event = new(AuthorityOwnershipTransferred)
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
func (it *AuthorityOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AuthorityOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AuthorityOwnershipTransferred represents a OwnershipTransferred event raised by the Authority contract.
type AuthorityOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_Authority *AuthorityFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*AuthorityOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _Authority.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &AuthorityOwnershipTransferredIterator{contract: _Authority.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_Authority *AuthorityFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *AuthorityOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _Authority.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AuthorityOwnershipTransferred)
				if err := _Authority.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
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
func (_Authority *AuthorityFilterer) ParseOwnershipTransferred(log types.Log) (*AuthorityOwnershipTransferred, error) {
	event := new(AuthorityOwnershipTransferred)
	if err := _Authority.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
