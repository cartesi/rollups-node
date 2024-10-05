// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package iinputbox

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

// IInputBoxMetaData contains all meta data concerning the IInputBox contract.
var IInputBoxMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"appContract\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"inputLength\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"maxInputLength\",\"type\":\"uint256\"}],\"name\":\"InputTooLarge\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"appContract\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"uint256\",\"name\":\"index\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"bytes\",\"name\":\"input\",\"type\":\"bytes\"}],\"name\":\"InputAdded\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"appContract\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"payload\",\"type\":\"bytes\"}],\"name\":\"addInput\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"appContract\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"index\",\"type\":\"uint256\"}],\"name\":\"getInputHash\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"appContract\",\"type\":\"address\"}],\"name\":\"getNumberOfInputs\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// IInputBoxABI is the input ABI used to generate the binding from.
// Deprecated: Use IInputBoxMetaData.ABI instead.
var IInputBoxABI = IInputBoxMetaData.ABI

// IInputBox is an auto generated Go binding around an Ethereum contract.
type IInputBox struct {
	IInputBoxCaller     // Read-only binding to the contract
	IInputBoxTransactor // Write-only binding to the contract
	IInputBoxFilterer   // Log filterer for contract events
}

// IInputBoxCaller is an auto generated read-only Go binding around an Ethereum contract.
type IInputBoxCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IInputBoxTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IInputBoxTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IInputBoxFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IInputBoxFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IInputBoxSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IInputBoxSession struct {
	Contract     *IInputBox        // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IInputBoxCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IInputBoxCallerSession struct {
	Contract *IInputBoxCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts    // Call options to use throughout this session
}

// IInputBoxTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IInputBoxTransactorSession struct {
	Contract     *IInputBoxTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts    // Transaction auth options to use throughout this session
}

// IInputBoxRaw is an auto generated low-level Go binding around an Ethereum contract.
type IInputBoxRaw struct {
	Contract *IInputBox // Generic contract binding to access the raw methods on
}

// IInputBoxCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IInputBoxCallerRaw struct {
	Contract *IInputBoxCaller // Generic read-only contract binding to access the raw methods on
}

// IInputBoxTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IInputBoxTransactorRaw struct {
	Contract *IInputBoxTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIInputBox creates a new instance of IInputBox, bound to a specific deployed contract.
func NewIInputBox(address common.Address, backend bind.ContractBackend) (*IInputBox, error) {
	contract, err := bindIInputBox(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IInputBox{IInputBoxCaller: IInputBoxCaller{contract: contract}, IInputBoxTransactor: IInputBoxTransactor{contract: contract}, IInputBoxFilterer: IInputBoxFilterer{contract: contract}}, nil
}

// NewIInputBoxCaller creates a new read-only instance of IInputBox, bound to a specific deployed contract.
func NewIInputBoxCaller(address common.Address, caller bind.ContractCaller) (*IInputBoxCaller, error) {
	contract, err := bindIInputBox(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IInputBoxCaller{contract: contract}, nil
}

// NewIInputBoxTransactor creates a new write-only instance of IInputBox, bound to a specific deployed contract.
func NewIInputBoxTransactor(address common.Address, transactor bind.ContractTransactor) (*IInputBoxTransactor, error) {
	contract, err := bindIInputBox(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IInputBoxTransactor{contract: contract}, nil
}

// NewIInputBoxFilterer creates a new log filterer instance of IInputBox, bound to a specific deployed contract.
func NewIInputBoxFilterer(address common.Address, filterer bind.ContractFilterer) (*IInputBoxFilterer, error) {
	contract, err := bindIInputBox(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IInputBoxFilterer{contract: contract}, nil
}

// bindIInputBox binds a generic wrapper to an already deployed contract.
func bindIInputBox(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IInputBoxMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IInputBox *IInputBoxRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IInputBox.Contract.IInputBoxCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IInputBox *IInputBoxRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IInputBox.Contract.IInputBoxTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IInputBox *IInputBoxRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IInputBox.Contract.IInputBoxTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IInputBox *IInputBoxCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IInputBox.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IInputBox *IInputBoxTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IInputBox.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IInputBox *IInputBoxTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IInputBox.Contract.contract.Transact(opts, method, params...)
}

// GetInputHash is a free data retrieval call binding the contract method 0x677087c9.
//
// Solidity: function getInputHash(address appContract, uint256 index) view returns(bytes32)
func (_IInputBox *IInputBoxCaller) GetInputHash(opts *bind.CallOpts, appContract common.Address, index *big.Int) ([32]byte, error) {
	var out []interface{}
	err := _IInputBox.contract.Call(opts, &out, "getInputHash", appContract, index)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetInputHash is a free data retrieval call binding the contract method 0x677087c9.
//
// Solidity: function getInputHash(address appContract, uint256 index) view returns(bytes32)
func (_IInputBox *IInputBoxSession) GetInputHash(appContract common.Address, index *big.Int) ([32]byte, error) {
	return _IInputBox.Contract.GetInputHash(&_IInputBox.CallOpts, appContract, index)
}

// GetInputHash is a free data retrieval call binding the contract method 0x677087c9.
//
// Solidity: function getInputHash(address appContract, uint256 index) view returns(bytes32)
func (_IInputBox *IInputBoxCallerSession) GetInputHash(appContract common.Address, index *big.Int) ([32]byte, error) {
	return _IInputBox.Contract.GetInputHash(&_IInputBox.CallOpts, appContract, index)
}

// GetNumberOfInputs is a free data retrieval call binding the contract method 0x61a93c87.
//
// Solidity: function getNumberOfInputs(address appContract) view returns(uint256)
func (_IInputBox *IInputBoxCaller) GetNumberOfInputs(opts *bind.CallOpts, appContract common.Address) (*big.Int, error) {
	var out []interface{}
	err := _IInputBox.contract.Call(opts, &out, "getNumberOfInputs", appContract)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNumberOfInputs is a free data retrieval call binding the contract method 0x61a93c87.
//
// Solidity: function getNumberOfInputs(address appContract) view returns(uint256)
func (_IInputBox *IInputBoxSession) GetNumberOfInputs(appContract common.Address) (*big.Int, error) {
	return _IInputBox.Contract.GetNumberOfInputs(&_IInputBox.CallOpts, appContract)
}

// GetNumberOfInputs is a free data retrieval call binding the contract method 0x61a93c87.
//
// Solidity: function getNumberOfInputs(address appContract) view returns(uint256)
func (_IInputBox *IInputBoxCallerSession) GetNumberOfInputs(appContract common.Address) (*big.Int, error) {
	return _IInputBox.Contract.GetNumberOfInputs(&_IInputBox.CallOpts, appContract)
}

// AddInput is a paid mutator transaction binding the contract method 0x1789cd63.
//
// Solidity: function addInput(address appContract, bytes payload) returns(bytes32)
func (_IInputBox *IInputBoxTransactor) AddInput(opts *bind.TransactOpts, appContract common.Address, payload []byte) (*types.Transaction, error) {
	return _IInputBox.contract.Transact(opts, "addInput", appContract, payload)
}

// AddInput is a paid mutator transaction binding the contract method 0x1789cd63.
//
// Solidity: function addInput(address appContract, bytes payload) returns(bytes32)
func (_IInputBox *IInputBoxSession) AddInput(appContract common.Address, payload []byte) (*types.Transaction, error) {
	return _IInputBox.Contract.AddInput(&_IInputBox.TransactOpts, appContract, payload)
}

// AddInput is a paid mutator transaction binding the contract method 0x1789cd63.
//
// Solidity: function addInput(address appContract, bytes payload) returns(bytes32)
func (_IInputBox *IInputBoxTransactorSession) AddInput(appContract common.Address, payload []byte) (*types.Transaction, error) {
	return _IInputBox.Contract.AddInput(&_IInputBox.TransactOpts, appContract, payload)
}

// IInputBoxInputAddedIterator is returned from FilterInputAdded and is used to iterate over the raw logs and unpacked data for InputAdded events raised by the IInputBox contract.
type IInputBoxInputAddedIterator struct {
	Event *IInputBoxInputAdded // Event containing the contract specifics and raw log

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
func (it *IInputBoxInputAddedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IInputBoxInputAdded)
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
		it.Event = new(IInputBoxInputAdded)
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
func (it *IInputBoxInputAddedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IInputBoxInputAddedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IInputBoxInputAdded represents a InputAdded event raised by the IInputBox contract.
type IInputBoxInputAdded struct {
	AppContract common.Address
	Index       *big.Int
	Input       []byte
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterInputAdded is a free log retrieval operation binding the contract event 0xc05d337121a6e8605c6ec0b72aa29c4210ffe6e5b9cefdd6a7058188a8f66f98.
//
// Solidity: event InputAdded(address indexed appContract, uint256 indexed index, bytes input)
func (_IInputBox *IInputBoxFilterer) FilterInputAdded(opts *bind.FilterOpts, appContract []common.Address, index []*big.Int) (*IInputBoxInputAddedIterator, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}
	var indexRule []interface{}
	for _, indexItem := range index {
		indexRule = append(indexRule, indexItem)
	}

	logs, sub, err := _IInputBox.contract.FilterLogs(opts, "InputAdded", appContractRule, indexRule)
	if err != nil {
		return nil, err
	}
	return &IInputBoxInputAddedIterator{contract: _IInputBox.contract, event: "InputAdded", logs: logs, sub: sub}, nil
}

// WatchInputAdded is a free log subscription operation binding the contract event 0xc05d337121a6e8605c6ec0b72aa29c4210ffe6e5b9cefdd6a7058188a8f66f98.
//
// Solidity: event InputAdded(address indexed appContract, uint256 indexed index, bytes input)
func (_IInputBox *IInputBoxFilterer) WatchInputAdded(opts *bind.WatchOpts, sink chan<- *IInputBoxInputAdded, appContract []common.Address, index []*big.Int) (event.Subscription, error) {

	var appContractRule []interface{}
	for _, appContractItem := range appContract {
		appContractRule = append(appContractRule, appContractItem)
	}
	var indexRule []interface{}
	for _, indexItem := range index {
		indexRule = append(indexRule, indexItem)
	}

	logs, sub, err := _IInputBox.contract.WatchLogs(opts, "InputAdded", appContractRule, indexRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IInputBoxInputAdded)
				if err := _IInputBox.contract.UnpackLog(event, "InputAdded", log); err != nil {
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

// ParseInputAdded is a log parse operation binding the contract event 0xc05d337121a6e8605c6ec0b72aa29c4210ffe6e5b9cefdd6a7058188a8f66f98.
//
// Solidity: event InputAdded(address indexed appContract, uint256 indexed index, bytes input)
func (_IInputBox *IInputBoxFilterer) ParseInputAdded(log types.Log) (*IInputBoxInputAdded, error) {
	event := new(IInputBoxInputAdded)
	if err := _IInputBox.contract.UnpackLog(event, "InputAdded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
