// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package ierc20errors

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

// IERC20ErrorsMetaData contains all meta data concerning the IERC20Errors contract.
var IERC20ErrorsMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"error\",\"name\":\"ERC20InsufficientAllowance\",\"inputs\":[{\"name\":\"spender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"allowance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"needed\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ERC20InsufficientBalance\",\"inputs\":[{\"name\":\"sender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"balance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"needed\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ERC20InvalidApprover\",\"inputs\":[{\"name\":\"approver\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ERC20InvalidReceiver\",\"inputs\":[{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ERC20InvalidSender\",\"inputs\":[{\"name\":\"sender\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ERC20InvalidSpender\",\"inputs\":[{\"name\":\"spender\",\"type\":\"address\",\"internalType\":\"address\"}]}]",
}

// IERC20ErrorsABI is the input ABI used to generate the binding from.
// Deprecated: Use IERC20ErrorsMetaData.ABI instead.
var IERC20ErrorsABI = IERC20ErrorsMetaData.ABI

// IERC20Errors is an auto generated Go binding around an Ethereum contract.
type IERC20Errors struct {
	IERC20ErrorsCaller     // Read-only binding to the contract
	IERC20ErrorsTransactor // Write-only binding to the contract
	IERC20ErrorsFilterer   // Log filterer for contract events
}

// IERC20ErrorsCaller is an auto generated read-only Go binding around an Ethereum contract.
type IERC20ErrorsCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IERC20ErrorsTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IERC20ErrorsTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IERC20ErrorsFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IERC20ErrorsFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IERC20ErrorsSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IERC20ErrorsSession struct {
	Contract     *IERC20Errors     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IERC20ErrorsCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IERC20ErrorsCallerSession struct {
	Contract *IERC20ErrorsCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// IERC20ErrorsTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IERC20ErrorsTransactorSession struct {
	Contract     *IERC20ErrorsTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// IERC20ErrorsRaw is an auto generated low-level Go binding around an Ethereum contract.
type IERC20ErrorsRaw struct {
	Contract *IERC20Errors // Generic contract binding to access the raw methods on
}

// IERC20ErrorsCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IERC20ErrorsCallerRaw struct {
	Contract *IERC20ErrorsCaller // Generic read-only contract binding to access the raw methods on
}

// IERC20ErrorsTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IERC20ErrorsTransactorRaw struct {
	Contract *IERC20ErrorsTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIERC20Errors creates a new instance of IERC20Errors, bound to a specific deployed contract.
func NewIERC20Errors(address common.Address, backend bind.ContractBackend) (*IERC20Errors, error) {
	contract, err := bindIERC20Errors(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IERC20Errors{IERC20ErrorsCaller: IERC20ErrorsCaller{contract: contract}, IERC20ErrorsTransactor: IERC20ErrorsTransactor{contract: contract}, IERC20ErrorsFilterer: IERC20ErrorsFilterer{contract: contract}}, nil
}

// NewIERC20ErrorsCaller creates a new read-only instance of IERC20Errors, bound to a specific deployed contract.
func NewIERC20ErrorsCaller(address common.Address, caller bind.ContractCaller) (*IERC20ErrorsCaller, error) {
	contract, err := bindIERC20Errors(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IERC20ErrorsCaller{contract: contract}, nil
}

// NewIERC20ErrorsTransactor creates a new write-only instance of IERC20Errors, bound to a specific deployed contract.
func NewIERC20ErrorsTransactor(address common.Address, transactor bind.ContractTransactor) (*IERC20ErrorsTransactor, error) {
	contract, err := bindIERC20Errors(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IERC20ErrorsTransactor{contract: contract}, nil
}

// NewIERC20ErrorsFilterer creates a new log filterer instance of IERC20Errors, bound to a specific deployed contract.
func NewIERC20ErrorsFilterer(address common.Address, filterer bind.ContractFilterer) (*IERC20ErrorsFilterer, error) {
	contract, err := bindIERC20Errors(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IERC20ErrorsFilterer{contract: contract}, nil
}

// bindIERC20Errors binds a generic wrapper to an already deployed contract.
func bindIERC20Errors(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IERC20ErrorsMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IERC20Errors *IERC20ErrorsRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IERC20Errors.Contract.IERC20ErrorsCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IERC20Errors *IERC20ErrorsRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IERC20Errors.Contract.IERC20ErrorsTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IERC20Errors *IERC20ErrorsRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IERC20Errors.Contract.IERC20ErrorsTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IERC20Errors *IERC20ErrorsCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IERC20Errors.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IERC20Errors *IERC20ErrorsTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IERC20Errors.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IERC20Errors *IERC20ErrorsTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IERC20Errors.Contract.contract.Transact(opts, method, params...)
}
