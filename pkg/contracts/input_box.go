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

// InputBoxMetaData contains all meta data concerning the InputBox contract.
var InputBoxMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"InputSizeExceedsLimit\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"dapp\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"uint256\",\"name\":\"inputIndex\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"bytes\",\"name\":\"input\",\"type\":\"bytes\"}],\"name\":\"InputAdded\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_dapp\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"_input\",\"type\":\"bytes\"}],\"name\":\"addInput\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_dapp\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"_index\",\"type\":\"uint256\"}],\"name\":\"getInputHash\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_dapp\",\"type\":\"address\"}],\"name\":\"getNumberOfInputs\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// InputBoxABI is the input ABI used to generate the binding from.
// Deprecated: Use InputBoxMetaData.ABI instead.
var InputBoxABI = InputBoxMetaData.ABI

// InputBox is an auto generated Go binding around an Ethereum contract.
type InputBox struct {
	InputBoxCaller     // Read-only binding to the contract
	InputBoxTransactor // Write-only binding to the contract
	InputBoxFilterer   // Log filterer for contract events
}

// InputBoxCaller is an auto generated read-only Go binding around an Ethereum contract.
type InputBoxCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// InputBoxTransactor is an auto generated write-only Go binding around an Ethereum contract.
type InputBoxTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// InputBoxFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type InputBoxFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// InputBoxSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type InputBoxSession struct {
	Contract     *InputBox         // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// InputBoxCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type InputBoxCallerSession struct {
	Contract *InputBoxCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts   // Call options to use throughout this session
}

// InputBoxTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type InputBoxTransactorSession struct {
	Contract     *InputBoxTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// InputBoxRaw is an auto generated low-level Go binding around an Ethereum contract.
type InputBoxRaw struct {
	Contract *InputBox // Generic contract binding to access the raw methods on
}

// InputBoxCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type InputBoxCallerRaw struct {
	Contract *InputBoxCaller // Generic read-only contract binding to access the raw methods on
}

// InputBoxTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type InputBoxTransactorRaw struct {
	Contract *InputBoxTransactor // Generic write-only contract binding to access the raw methods on
}

// NewInputBox creates a new instance of InputBox, bound to a specific deployed contract.
func NewInputBox(address common.Address, backend bind.ContractBackend) (*InputBox, error) {
	contract, err := bindInputBox(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &InputBox{InputBoxCaller: InputBoxCaller{contract: contract}, InputBoxTransactor: InputBoxTransactor{contract: contract}, InputBoxFilterer: InputBoxFilterer{contract: contract}}, nil
}

// NewInputBoxCaller creates a new read-only instance of InputBox, bound to a specific deployed contract.
func NewInputBoxCaller(address common.Address, caller bind.ContractCaller) (*InputBoxCaller, error) {
	contract, err := bindInputBox(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &InputBoxCaller{contract: contract}, nil
}

// NewInputBoxTransactor creates a new write-only instance of InputBox, bound to a specific deployed contract.
func NewInputBoxTransactor(address common.Address, transactor bind.ContractTransactor) (*InputBoxTransactor, error) {
	contract, err := bindInputBox(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &InputBoxTransactor{contract: contract}, nil
}

// NewInputBoxFilterer creates a new log filterer instance of InputBox, bound to a specific deployed contract.
func NewInputBoxFilterer(address common.Address, filterer bind.ContractFilterer) (*InputBoxFilterer, error) {
	contract, err := bindInputBox(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &InputBoxFilterer{contract: contract}, nil
}

// bindInputBox binds a generic wrapper to an already deployed contract.
func bindInputBox(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := InputBoxMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_InputBox *InputBoxRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _InputBox.Contract.InputBoxCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_InputBox *InputBoxRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _InputBox.Contract.InputBoxTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_InputBox *InputBoxRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _InputBox.Contract.InputBoxTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_InputBox *InputBoxCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _InputBox.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_InputBox *InputBoxTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _InputBox.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_InputBox *InputBoxTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _InputBox.Contract.contract.Transact(opts, method, params...)
}

// GetInputHash is a free data retrieval call binding the contract method 0x677087c9.
//
// Solidity: function getInputHash(address _dapp, uint256 _index) view returns(bytes32)
func (_InputBox *InputBoxCaller) GetInputHash(opts *bind.CallOpts, _dapp common.Address, _index *big.Int) ([32]byte, error) {
	var out []interface{}
	err := _InputBox.contract.Call(opts, &out, "getInputHash", _dapp, _index)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetInputHash is a free data retrieval call binding the contract method 0x677087c9.
//
// Solidity: function getInputHash(address _dapp, uint256 _index) view returns(bytes32)
func (_InputBox *InputBoxSession) GetInputHash(_dapp common.Address, _index *big.Int) ([32]byte, error) {
	return _InputBox.Contract.GetInputHash(&_InputBox.CallOpts, _dapp, _index)
}

// GetInputHash is a free data retrieval call binding the contract method 0x677087c9.
//
// Solidity: function getInputHash(address _dapp, uint256 _index) view returns(bytes32)
func (_InputBox *InputBoxCallerSession) GetInputHash(_dapp common.Address, _index *big.Int) ([32]byte, error) {
	return _InputBox.Contract.GetInputHash(&_InputBox.CallOpts, _dapp, _index)
}

// GetNumberOfInputs is a free data retrieval call binding the contract method 0x61a93c87.
//
// Solidity: function getNumberOfInputs(address _dapp) view returns(uint256)
func (_InputBox *InputBoxCaller) GetNumberOfInputs(opts *bind.CallOpts, _dapp common.Address) (*big.Int, error) {
	var out []interface{}
	err := _InputBox.contract.Call(opts, &out, "getNumberOfInputs", _dapp)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNumberOfInputs is a free data retrieval call binding the contract method 0x61a93c87.
//
// Solidity: function getNumberOfInputs(address _dapp) view returns(uint256)
func (_InputBox *InputBoxSession) GetNumberOfInputs(_dapp common.Address) (*big.Int, error) {
	return _InputBox.Contract.GetNumberOfInputs(&_InputBox.CallOpts, _dapp)
}

// GetNumberOfInputs is a free data retrieval call binding the contract method 0x61a93c87.
//
// Solidity: function getNumberOfInputs(address _dapp) view returns(uint256)
func (_InputBox *InputBoxCallerSession) GetNumberOfInputs(_dapp common.Address) (*big.Int, error) {
	return _InputBox.Contract.GetNumberOfInputs(&_InputBox.CallOpts, _dapp)
}

// AddInput is a paid mutator transaction binding the contract method 0x1789cd63.
//
// Solidity: function addInput(address _dapp, bytes _input) returns(bytes32)
func (_InputBox *InputBoxTransactor) AddInput(opts *bind.TransactOpts, _dapp common.Address, _input []byte) (*types.Transaction, error) {
	return _InputBox.contract.Transact(opts, "addInput", _dapp, _input)
}

// AddInput is a paid mutator transaction binding the contract method 0x1789cd63.
//
// Solidity: function addInput(address _dapp, bytes _input) returns(bytes32)
func (_InputBox *InputBoxSession) AddInput(_dapp common.Address, _input []byte) (*types.Transaction, error) {
	return _InputBox.Contract.AddInput(&_InputBox.TransactOpts, _dapp, _input)
}

// AddInput is a paid mutator transaction binding the contract method 0x1789cd63.
//
// Solidity: function addInput(address _dapp, bytes _input) returns(bytes32)
func (_InputBox *InputBoxTransactorSession) AddInput(_dapp common.Address, _input []byte) (*types.Transaction, error) {
	return _InputBox.Contract.AddInput(&_InputBox.TransactOpts, _dapp, _input)
}

// InputBoxInputAddedIterator is returned from FilterInputAdded and is used to iterate over the raw logs and unpacked data for InputAdded events raised by the InputBox contract.
type InputBoxInputAddedIterator struct {
	Event *InputBoxInputAdded // Event containing the contract specifics and raw log

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
func (it *InputBoxInputAddedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(InputBoxInputAdded)
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
		it.Event = new(InputBoxInputAdded)
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
func (it *InputBoxInputAddedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *InputBoxInputAddedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// InputBoxInputAdded represents a InputAdded event raised by the InputBox contract.
type InputBoxInputAdded struct {
	Dapp       common.Address
	InputIndex *big.Int
	Sender     common.Address
	Input      []byte
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterInputAdded is a free log retrieval operation binding the contract event 0x6aaa400068bf4ca337265e2a1e1e841f66b8597fd5b452fdc52a44bed28a0784.
//
// Solidity: event InputAdded(address indexed dapp, uint256 indexed inputIndex, address sender, bytes input)
func (_InputBox *InputBoxFilterer) FilterInputAdded(opts *bind.FilterOpts, dapp []common.Address, inputIndex []*big.Int) (*InputBoxInputAddedIterator, error) {

	var dappRule []interface{}
	for _, dappItem := range dapp {
		dappRule = append(dappRule, dappItem)
	}
	var inputIndexRule []interface{}
	for _, inputIndexItem := range inputIndex {
		inputIndexRule = append(inputIndexRule, inputIndexItem)
	}

	logs, sub, err := _InputBox.contract.FilterLogs(opts, "InputAdded", dappRule, inputIndexRule)
	if err != nil {
		return nil, err
	}
	return &InputBoxInputAddedIterator{contract: _InputBox.contract, event: "InputAdded", logs: logs, sub: sub}, nil
}

// WatchInputAdded is a free log subscription operation binding the contract event 0x6aaa400068bf4ca337265e2a1e1e841f66b8597fd5b452fdc52a44bed28a0784.
//
// Solidity: event InputAdded(address indexed dapp, uint256 indexed inputIndex, address sender, bytes input)
func (_InputBox *InputBoxFilterer) WatchInputAdded(opts *bind.WatchOpts, sink chan<- *InputBoxInputAdded, dapp []common.Address, inputIndex []*big.Int) (event.Subscription, error) {

	var dappRule []interface{}
	for _, dappItem := range dapp {
		dappRule = append(dappRule, dappItem)
	}
	var inputIndexRule []interface{}
	for _, inputIndexItem := range inputIndex {
		inputIndexRule = append(inputIndexRule, inputIndexItem)
	}

	logs, sub, err := _InputBox.contract.WatchLogs(opts, "InputAdded", dappRule, inputIndexRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(InputBoxInputAdded)
				if err := _InputBox.contract.UnpackLog(event, "InputAdded", log); err != nil {
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

// ParseInputAdded is a log parse operation binding the contract event 0x6aaa400068bf4ca337265e2a1e1e841f66b8597fd5b452fdc52a44bed28a0784.
//
// Solidity: event InputAdded(address indexed dapp, uint256 indexed inputIndex, address sender, bytes input)
func (_InputBox *InputBoxFilterer) ParseInputAdded(log types.Log) (*InputBoxInputAdded, error) {
	event := new(InputBoxInputAdded)
	if err := _InputBox.contract.UnpackLog(event, "InputAdded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
