// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package iapplication

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

// OutputValidityProof is an auto generated low-level Go binding around an user-defined struct.
type OutputValidityProof struct {
	OutputIndex          uint64
	OutputHashesSiblings [][32]byte
}

// IApplicationMetaData contains all meta data concerning the IApplication contract.
var IApplicationMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"claim\",\"type\":\"bytes32\"}],\"name\":\"ClaimNotAccepted\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidOutputHashesSiblingsArrayLength\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"output\",\"type\":\"bytes\"}],\"name\":\"OutputNotExecutable\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"output\",\"type\":\"bytes\"}],\"name\":\"OutputNotReexecutable\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"contractIConsensus\",\"name\":\"newConsensus\",\"type\":\"address\"}],\"name\":\"NewConsensus\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"outputIndex\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"bytes\",\"name\":\"output\",\"type\":\"bytes\"}],\"name\":\"OutputExecuted\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"output\",\"type\":\"bytes\"},{\"components\":[{\"internalType\":\"uint64\",\"name\":\"outputIndex\",\"type\":\"uint64\"},{\"internalType\":\"bytes32[]\",\"name\":\"outputHashesSiblings\",\"type\":\"bytes32[]\"}],\"internalType\":\"structOutputValidityProof\",\"name\":\"proof\",\"type\":\"tuple\"}],\"name\":\"executeOutput\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getConsensus\",\"outputs\":[{\"internalType\":\"contractIConsensus\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getTemplateHash\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"contractIConsensus\",\"name\":\"newConsensus\",\"type\":\"address\"}],\"name\":\"migrateToConsensus\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"renounceOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"transferOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"output\",\"type\":\"bytes\"},{\"components\":[{\"internalType\":\"uint64\",\"name\":\"outputIndex\",\"type\":\"uint64\"},{\"internalType\":\"bytes32[]\",\"name\":\"outputHashesSiblings\",\"type\":\"bytes32[]\"}],\"internalType\":\"structOutputValidityProof\",\"name\":\"proof\",\"type\":\"tuple\"}],\"name\":\"validateOutput\",\"outputs\":[],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"outputHash\",\"type\":\"bytes32\"},{\"components\":[{\"internalType\":\"uint64\",\"name\":\"outputIndex\",\"type\":\"uint64\"},{\"internalType\":\"bytes32[]\",\"name\":\"outputHashesSiblings\",\"type\":\"bytes32[]\"}],\"internalType\":\"structOutputValidityProof\",\"name\":\"proof\",\"type\":\"tuple\"}],\"name\":\"validateOutputHash\",\"outputs\":[],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"outputIndex\",\"type\":\"uint256\"}],\"name\":\"wasOutputExecuted\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// IApplicationABI is the input ABI used to generate the binding from.
// Deprecated: Use IApplicationMetaData.ABI instead.
var IApplicationABI = IApplicationMetaData.ABI

// IApplication is an auto generated Go binding around an Ethereum contract.
type IApplication struct {
	IApplicationCaller     // Read-only binding to the contract
	IApplicationTransactor // Write-only binding to the contract
	IApplicationFilterer   // Log filterer for contract events
}

// IApplicationCaller is an auto generated read-only Go binding around an Ethereum contract.
type IApplicationCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IApplicationTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IApplicationTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IApplicationFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IApplicationFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IApplicationSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IApplicationSession struct {
	Contract     *IApplication     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IApplicationCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IApplicationCallerSession struct {
	Contract *IApplicationCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// IApplicationTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IApplicationTransactorSession struct {
	Contract     *IApplicationTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// IApplicationRaw is an auto generated low-level Go binding around an Ethereum contract.
type IApplicationRaw struct {
	Contract *IApplication // Generic contract binding to access the raw methods on
}

// IApplicationCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IApplicationCallerRaw struct {
	Contract *IApplicationCaller // Generic read-only contract binding to access the raw methods on
}

// IApplicationTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IApplicationTransactorRaw struct {
	Contract *IApplicationTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIApplication creates a new instance of IApplication, bound to a specific deployed contract.
func NewIApplication(address common.Address, backend bind.ContractBackend) (*IApplication, error) {
	contract, err := bindIApplication(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IApplication{IApplicationCaller: IApplicationCaller{contract: contract}, IApplicationTransactor: IApplicationTransactor{contract: contract}, IApplicationFilterer: IApplicationFilterer{contract: contract}}, nil
}

// NewIApplicationCaller creates a new read-only instance of IApplication, bound to a specific deployed contract.
func NewIApplicationCaller(address common.Address, caller bind.ContractCaller) (*IApplicationCaller, error) {
	contract, err := bindIApplication(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IApplicationCaller{contract: contract}, nil
}

// NewIApplicationTransactor creates a new write-only instance of IApplication, bound to a specific deployed contract.
func NewIApplicationTransactor(address common.Address, transactor bind.ContractTransactor) (*IApplicationTransactor, error) {
	contract, err := bindIApplication(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IApplicationTransactor{contract: contract}, nil
}

// NewIApplicationFilterer creates a new log filterer instance of IApplication, bound to a specific deployed contract.
func NewIApplicationFilterer(address common.Address, filterer bind.ContractFilterer) (*IApplicationFilterer, error) {
	contract, err := bindIApplication(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IApplicationFilterer{contract: contract}, nil
}

// bindIApplication binds a generic wrapper to an already deployed contract.
func bindIApplication(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IApplicationMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IApplication *IApplicationRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IApplication.Contract.IApplicationCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IApplication *IApplicationRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IApplication.Contract.IApplicationTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IApplication *IApplicationRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IApplication.Contract.IApplicationTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IApplication *IApplicationCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IApplication.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IApplication *IApplicationTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IApplication.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IApplication *IApplicationTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IApplication.Contract.contract.Transact(opts, method, params...)
}

// GetConsensus is a free data retrieval call binding the contract method 0x179e740b.
//
// Solidity: function getConsensus() view returns(address)
func (_IApplication *IApplicationCaller) GetConsensus(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "getConsensus")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetConsensus is a free data retrieval call binding the contract method 0x179e740b.
//
// Solidity: function getConsensus() view returns(address)
func (_IApplication *IApplicationSession) GetConsensus() (common.Address, error) {
	return _IApplication.Contract.GetConsensus(&_IApplication.CallOpts)
}

// GetConsensus is a free data retrieval call binding the contract method 0x179e740b.
//
// Solidity: function getConsensus() view returns(address)
func (_IApplication *IApplicationCallerSession) GetConsensus() (common.Address, error) {
	return _IApplication.Contract.GetConsensus(&_IApplication.CallOpts)
}

// GetTemplateHash is a free data retrieval call binding the contract method 0x61b12c66.
//
// Solidity: function getTemplateHash() view returns(bytes32)
func (_IApplication *IApplicationCaller) GetTemplateHash(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "getTemplateHash")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetTemplateHash is a free data retrieval call binding the contract method 0x61b12c66.
//
// Solidity: function getTemplateHash() view returns(bytes32)
func (_IApplication *IApplicationSession) GetTemplateHash() ([32]byte, error) {
	return _IApplication.Contract.GetTemplateHash(&_IApplication.CallOpts)
}

// GetTemplateHash is a free data retrieval call binding the contract method 0x61b12c66.
//
// Solidity: function getTemplateHash() view returns(bytes32)
func (_IApplication *IApplicationCallerSession) GetTemplateHash() ([32]byte, error) {
	return _IApplication.Contract.GetTemplateHash(&_IApplication.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_IApplication *IApplicationCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_IApplication *IApplicationSession) Owner() (common.Address, error) {
	return _IApplication.Contract.Owner(&_IApplication.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_IApplication *IApplicationCallerSession) Owner() (common.Address, error) {
	return _IApplication.Contract.Owner(&_IApplication.CallOpts)
}

// ValidateOutput is a free data retrieval call binding the contract method 0xe88d39c0.
//
// Solidity: function validateOutput(bytes output, (uint64,bytes32[]) proof) view returns()
func (_IApplication *IApplicationCaller) ValidateOutput(opts *bind.CallOpts, output []byte, proof OutputValidityProof) error {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "validateOutput", output, proof)

	if err != nil {
		return err
	}

	return err

}

// ValidateOutput is a free data retrieval call binding the contract method 0xe88d39c0.
//
// Solidity: function validateOutput(bytes output, (uint64,bytes32[]) proof) view returns()
func (_IApplication *IApplicationSession) ValidateOutput(output []byte, proof OutputValidityProof) error {
	return _IApplication.Contract.ValidateOutput(&_IApplication.CallOpts, output, proof)
}

// ValidateOutput is a free data retrieval call binding the contract method 0xe88d39c0.
//
// Solidity: function validateOutput(bytes output, (uint64,bytes32[]) proof) view returns()
func (_IApplication *IApplicationCallerSession) ValidateOutput(output []byte, proof OutputValidityProof) error {
	return _IApplication.Contract.ValidateOutput(&_IApplication.CallOpts, output, proof)
}

// ValidateOutputHash is a free data retrieval call binding the contract method 0x08eb89ab.
//
// Solidity: function validateOutputHash(bytes32 outputHash, (uint64,bytes32[]) proof) view returns()
func (_IApplication *IApplicationCaller) ValidateOutputHash(opts *bind.CallOpts, outputHash [32]byte, proof OutputValidityProof) error {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "validateOutputHash", outputHash, proof)

	if err != nil {
		return err
	}

	return err

}

// ValidateOutputHash is a free data retrieval call binding the contract method 0x08eb89ab.
//
// Solidity: function validateOutputHash(bytes32 outputHash, (uint64,bytes32[]) proof) view returns()
func (_IApplication *IApplicationSession) ValidateOutputHash(outputHash [32]byte, proof OutputValidityProof) error {
	return _IApplication.Contract.ValidateOutputHash(&_IApplication.CallOpts, outputHash, proof)
}

// ValidateOutputHash is a free data retrieval call binding the contract method 0x08eb89ab.
//
// Solidity: function validateOutputHash(bytes32 outputHash, (uint64,bytes32[]) proof) view returns()
func (_IApplication *IApplicationCallerSession) ValidateOutputHash(outputHash [32]byte, proof OutputValidityProof) error {
	return _IApplication.Contract.ValidateOutputHash(&_IApplication.CallOpts, outputHash, proof)
}

// WasOutputExecuted is a free data retrieval call binding the contract method 0x71891db0.
//
// Solidity: function wasOutputExecuted(uint256 outputIndex) view returns(bool)
func (_IApplication *IApplicationCaller) WasOutputExecuted(opts *bind.CallOpts, outputIndex *big.Int) (bool, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "wasOutputExecuted", outputIndex)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// WasOutputExecuted is a free data retrieval call binding the contract method 0x71891db0.
//
// Solidity: function wasOutputExecuted(uint256 outputIndex) view returns(bool)
func (_IApplication *IApplicationSession) WasOutputExecuted(outputIndex *big.Int) (bool, error) {
	return _IApplication.Contract.WasOutputExecuted(&_IApplication.CallOpts, outputIndex)
}

// WasOutputExecuted is a free data retrieval call binding the contract method 0x71891db0.
//
// Solidity: function wasOutputExecuted(uint256 outputIndex) view returns(bool)
func (_IApplication *IApplicationCallerSession) WasOutputExecuted(outputIndex *big.Int) (bool, error) {
	return _IApplication.Contract.WasOutputExecuted(&_IApplication.CallOpts, outputIndex)
}

// ExecuteOutput is a paid mutator transaction binding the contract method 0x33137b76.
//
// Solidity: function executeOutput(bytes output, (uint64,bytes32[]) proof) returns()
func (_IApplication *IApplicationTransactor) ExecuteOutput(opts *bind.TransactOpts, output []byte, proof OutputValidityProof) (*types.Transaction, error) {
	return _IApplication.contract.Transact(opts, "executeOutput", output, proof)
}

// ExecuteOutput is a paid mutator transaction binding the contract method 0x33137b76.
//
// Solidity: function executeOutput(bytes output, (uint64,bytes32[]) proof) returns()
func (_IApplication *IApplicationSession) ExecuteOutput(output []byte, proof OutputValidityProof) (*types.Transaction, error) {
	return _IApplication.Contract.ExecuteOutput(&_IApplication.TransactOpts, output, proof)
}

// ExecuteOutput is a paid mutator transaction binding the contract method 0x33137b76.
//
// Solidity: function executeOutput(bytes output, (uint64,bytes32[]) proof) returns()
func (_IApplication *IApplicationTransactorSession) ExecuteOutput(output []byte, proof OutputValidityProof) (*types.Transaction, error) {
	return _IApplication.Contract.ExecuteOutput(&_IApplication.TransactOpts, output, proof)
}

// MigrateToConsensus is a paid mutator transaction binding the contract method 0xfc411683.
//
// Solidity: function migrateToConsensus(address newConsensus) returns()
func (_IApplication *IApplicationTransactor) MigrateToConsensus(opts *bind.TransactOpts, newConsensus common.Address) (*types.Transaction, error) {
	return _IApplication.contract.Transact(opts, "migrateToConsensus", newConsensus)
}

// MigrateToConsensus is a paid mutator transaction binding the contract method 0xfc411683.
//
// Solidity: function migrateToConsensus(address newConsensus) returns()
func (_IApplication *IApplicationSession) MigrateToConsensus(newConsensus common.Address) (*types.Transaction, error) {
	return _IApplication.Contract.MigrateToConsensus(&_IApplication.TransactOpts, newConsensus)
}

// MigrateToConsensus is a paid mutator transaction binding the contract method 0xfc411683.
//
// Solidity: function migrateToConsensus(address newConsensus) returns()
func (_IApplication *IApplicationTransactorSession) MigrateToConsensus(newConsensus common.Address) (*types.Transaction, error) {
	return _IApplication.Contract.MigrateToConsensus(&_IApplication.TransactOpts, newConsensus)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_IApplication *IApplicationTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IApplication.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_IApplication *IApplicationSession) RenounceOwnership() (*types.Transaction, error) {
	return _IApplication.Contract.RenounceOwnership(&_IApplication.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_IApplication *IApplicationTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _IApplication.Contract.RenounceOwnership(&_IApplication.TransactOpts)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_IApplication *IApplicationTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _IApplication.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_IApplication *IApplicationSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _IApplication.Contract.TransferOwnership(&_IApplication.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_IApplication *IApplicationTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _IApplication.Contract.TransferOwnership(&_IApplication.TransactOpts, newOwner)
}

// IApplicationNewConsensusIterator is returned from FilterNewConsensus and is used to iterate over the raw logs and unpacked data for NewConsensus events raised by the IApplication contract.
type IApplicationNewConsensusIterator struct {
	Event *IApplicationNewConsensus // Event containing the contract specifics and raw log

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
func (it *IApplicationNewConsensusIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IApplicationNewConsensus)
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
		it.Event = new(IApplicationNewConsensus)
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
func (it *IApplicationNewConsensusIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IApplicationNewConsensusIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IApplicationNewConsensus represents a NewConsensus event raised by the IApplication contract.
type IApplicationNewConsensus struct {
	NewConsensus common.Address
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterNewConsensus is a free log retrieval operation binding the contract event 0x4991c6f37185659e276ff918a96f3e20e6c5abcd8c9aab450dc19c2f7ad35cb5.
//
// Solidity: event NewConsensus(address newConsensus)
func (_IApplication *IApplicationFilterer) FilterNewConsensus(opts *bind.FilterOpts) (*IApplicationNewConsensusIterator, error) {

	logs, sub, err := _IApplication.contract.FilterLogs(opts, "NewConsensus")
	if err != nil {
		return nil, err
	}
	return &IApplicationNewConsensusIterator{contract: _IApplication.contract, event: "NewConsensus", logs: logs, sub: sub}, nil
}

// WatchNewConsensus is a free log subscription operation binding the contract event 0x4991c6f37185659e276ff918a96f3e20e6c5abcd8c9aab450dc19c2f7ad35cb5.
//
// Solidity: event NewConsensus(address newConsensus)
func (_IApplication *IApplicationFilterer) WatchNewConsensus(opts *bind.WatchOpts, sink chan<- *IApplicationNewConsensus) (event.Subscription, error) {

	logs, sub, err := _IApplication.contract.WatchLogs(opts, "NewConsensus")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IApplicationNewConsensus)
				if err := _IApplication.contract.UnpackLog(event, "NewConsensus", log); err != nil {
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

// ParseNewConsensus is a log parse operation binding the contract event 0x4991c6f37185659e276ff918a96f3e20e6c5abcd8c9aab450dc19c2f7ad35cb5.
//
// Solidity: event NewConsensus(address newConsensus)
func (_IApplication *IApplicationFilterer) ParseNewConsensus(log types.Log) (*IApplicationNewConsensus, error) {
	event := new(IApplicationNewConsensus)
	if err := _IApplication.contract.UnpackLog(event, "NewConsensus", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IApplicationOutputExecutedIterator is returned from FilterOutputExecuted and is used to iterate over the raw logs and unpacked data for OutputExecuted events raised by the IApplication contract.
type IApplicationOutputExecutedIterator struct {
	Event *IApplicationOutputExecuted // Event containing the contract specifics and raw log

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
func (it *IApplicationOutputExecutedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IApplicationOutputExecuted)
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
		it.Event = new(IApplicationOutputExecuted)
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
func (it *IApplicationOutputExecutedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IApplicationOutputExecutedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IApplicationOutputExecuted represents a OutputExecuted event raised by the IApplication contract.
type IApplicationOutputExecuted struct {
	OutputIndex uint64
	Output      []byte
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterOutputExecuted is a free log retrieval operation binding the contract event 0xcad1f361c6e84664e892230291c8e8eb9555683e0a6a5ce8ea7b204ac0ac3676.
//
// Solidity: event OutputExecuted(uint64 outputIndex, bytes output)
func (_IApplication *IApplicationFilterer) FilterOutputExecuted(opts *bind.FilterOpts) (*IApplicationOutputExecutedIterator, error) {

	logs, sub, err := _IApplication.contract.FilterLogs(opts, "OutputExecuted")
	if err != nil {
		return nil, err
	}
	return &IApplicationOutputExecutedIterator{contract: _IApplication.contract, event: "OutputExecuted", logs: logs, sub: sub}, nil
}

// WatchOutputExecuted is a free log subscription operation binding the contract event 0xcad1f361c6e84664e892230291c8e8eb9555683e0a6a5ce8ea7b204ac0ac3676.
//
// Solidity: event OutputExecuted(uint64 outputIndex, bytes output)
func (_IApplication *IApplicationFilterer) WatchOutputExecuted(opts *bind.WatchOpts, sink chan<- *IApplicationOutputExecuted) (event.Subscription, error) {

	logs, sub, err := _IApplication.contract.WatchLogs(opts, "OutputExecuted")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IApplicationOutputExecuted)
				if err := _IApplication.contract.UnpackLog(event, "OutputExecuted", log); err != nil {
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

// ParseOutputExecuted is a log parse operation binding the contract event 0xcad1f361c6e84664e892230291c8e8eb9555683e0a6a5ce8ea7b204ac0ac3676.
//
// Solidity: event OutputExecuted(uint64 outputIndex, bytes output)
func (_IApplication *IApplicationFilterer) ParseOutputExecuted(log types.Log) (*IApplicationOutputExecuted, error) {
	event := new(IApplicationOutputExecuted)
	if err := _IApplication.contract.UnpackLog(event, "OutputExecuted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
