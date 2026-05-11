// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package iusdwithdrawaloutputbuilder

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

// IUsdWithdrawalOutputBuilderMetaData contains all meta data concerning the IUsdWithdrawalOutputBuilder contract.
var IUsdWithdrawalOutputBuilderMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"buildWithdrawalOutput\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"account\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"output\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"token\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractIERC20\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"major\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"minor\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"patch\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"preRelease\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"buildMetadata\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"error\",\"name\":\"AccountTooShort\",\"inputs\":[{\"name\":\"attemptedAccountSize\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"minAccountSize\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]}]",
}

// IUsdWithdrawalOutputBuilderABI is the input ABI used to generate the binding from.
// Deprecated: Use IUsdWithdrawalOutputBuilderMetaData.ABI instead.
var IUsdWithdrawalOutputBuilderABI = IUsdWithdrawalOutputBuilderMetaData.ABI

// IUsdWithdrawalOutputBuilder is an auto generated Go binding around an Ethereum contract.
type IUsdWithdrawalOutputBuilder struct {
	IUsdWithdrawalOutputBuilderCaller     // Read-only binding to the contract
	IUsdWithdrawalOutputBuilderTransactor // Write-only binding to the contract
	IUsdWithdrawalOutputBuilderFilterer   // Log filterer for contract events
}

// IUsdWithdrawalOutputBuilderCaller is an auto generated read-only Go binding around an Ethereum contract.
type IUsdWithdrawalOutputBuilderCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IUsdWithdrawalOutputBuilderTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IUsdWithdrawalOutputBuilderTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IUsdWithdrawalOutputBuilderFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IUsdWithdrawalOutputBuilderFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IUsdWithdrawalOutputBuilderSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IUsdWithdrawalOutputBuilderSession struct {
	Contract     *IUsdWithdrawalOutputBuilder // Generic contract binding to set the session for
	CallOpts     bind.CallOpts                // Call options to use throughout this session
	TransactOpts bind.TransactOpts            // Transaction auth options to use throughout this session
}

// IUsdWithdrawalOutputBuilderCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IUsdWithdrawalOutputBuilderCallerSession struct {
	Contract *IUsdWithdrawalOutputBuilderCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                      // Call options to use throughout this session
}

// IUsdWithdrawalOutputBuilderTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IUsdWithdrawalOutputBuilderTransactorSession struct {
	Contract     *IUsdWithdrawalOutputBuilderTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                      // Transaction auth options to use throughout this session
}

// IUsdWithdrawalOutputBuilderRaw is an auto generated low-level Go binding around an Ethereum contract.
type IUsdWithdrawalOutputBuilderRaw struct {
	Contract *IUsdWithdrawalOutputBuilder // Generic contract binding to access the raw methods on
}

// IUsdWithdrawalOutputBuilderCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IUsdWithdrawalOutputBuilderCallerRaw struct {
	Contract *IUsdWithdrawalOutputBuilderCaller // Generic read-only contract binding to access the raw methods on
}

// IUsdWithdrawalOutputBuilderTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IUsdWithdrawalOutputBuilderTransactorRaw struct {
	Contract *IUsdWithdrawalOutputBuilderTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIUsdWithdrawalOutputBuilder creates a new instance of IUsdWithdrawalOutputBuilder, bound to a specific deployed contract.
func NewIUsdWithdrawalOutputBuilder(address common.Address, backend bind.ContractBackend) (*IUsdWithdrawalOutputBuilder, error) {
	contract, err := bindIUsdWithdrawalOutputBuilder(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IUsdWithdrawalOutputBuilder{IUsdWithdrawalOutputBuilderCaller: IUsdWithdrawalOutputBuilderCaller{contract: contract}, IUsdWithdrawalOutputBuilderTransactor: IUsdWithdrawalOutputBuilderTransactor{contract: contract}, IUsdWithdrawalOutputBuilderFilterer: IUsdWithdrawalOutputBuilderFilterer{contract: contract}}, nil
}

// NewIUsdWithdrawalOutputBuilderCaller creates a new read-only instance of IUsdWithdrawalOutputBuilder, bound to a specific deployed contract.
func NewIUsdWithdrawalOutputBuilderCaller(address common.Address, caller bind.ContractCaller) (*IUsdWithdrawalOutputBuilderCaller, error) {
	contract, err := bindIUsdWithdrawalOutputBuilder(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IUsdWithdrawalOutputBuilderCaller{contract: contract}, nil
}

// NewIUsdWithdrawalOutputBuilderTransactor creates a new write-only instance of IUsdWithdrawalOutputBuilder, bound to a specific deployed contract.
func NewIUsdWithdrawalOutputBuilderTransactor(address common.Address, transactor bind.ContractTransactor) (*IUsdWithdrawalOutputBuilderTransactor, error) {
	contract, err := bindIUsdWithdrawalOutputBuilder(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IUsdWithdrawalOutputBuilderTransactor{contract: contract}, nil
}

// NewIUsdWithdrawalOutputBuilderFilterer creates a new log filterer instance of IUsdWithdrawalOutputBuilder, bound to a specific deployed contract.
func NewIUsdWithdrawalOutputBuilderFilterer(address common.Address, filterer bind.ContractFilterer) (*IUsdWithdrawalOutputBuilderFilterer, error) {
	contract, err := bindIUsdWithdrawalOutputBuilder(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IUsdWithdrawalOutputBuilderFilterer{contract: contract}, nil
}

// bindIUsdWithdrawalOutputBuilder binds a generic wrapper to an already deployed contract.
func bindIUsdWithdrawalOutputBuilder(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IUsdWithdrawalOutputBuilderMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IUsdWithdrawalOutputBuilder.Contract.IUsdWithdrawalOutputBuilderCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IUsdWithdrawalOutputBuilder.Contract.IUsdWithdrawalOutputBuilderTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IUsdWithdrawalOutputBuilder.Contract.IUsdWithdrawalOutputBuilderTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IUsdWithdrawalOutputBuilder.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IUsdWithdrawalOutputBuilder.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IUsdWithdrawalOutputBuilder.Contract.contract.Transact(opts, method, params...)
}

// BuildWithdrawalOutput is a free data retrieval call binding the contract method 0x1d2675a3.
//
// Solidity: function buildWithdrawalOutput(address appContract, bytes account) view returns(bytes output)
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderCaller) BuildWithdrawalOutput(opts *bind.CallOpts, appContract common.Address, account []byte) ([]byte, error) {
	var out []interface{}
	err := _IUsdWithdrawalOutputBuilder.contract.Call(opts, &out, "buildWithdrawalOutput", appContract, account)

	if err != nil {
		return *new([]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([]byte)).(*[]byte)

	return out0, err

}

// BuildWithdrawalOutput is a free data retrieval call binding the contract method 0x1d2675a3.
//
// Solidity: function buildWithdrawalOutput(address appContract, bytes account) view returns(bytes output)
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderSession) BuildWithdrawalOutput(appContract common.Address, account []byte) ([]byte, error) {
	return _IUsdWithdrawalOutputBuilder.Contract.BuildWithdrawalOutput(&_IUsdWithdrawalOutputBuilder.CallOpts, appContract, account)
}

// BuildWithdrawalOutput is a free data retrieval call binding the contract method 0x1d2675a3.
//
// Solidity: function buildWithdrawalOutput(address appContract, bytes account) view returns(bytes output)
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderCallerSession) BuildWithdrawalOutput(appContract common.Address, account []byte) ([]byte, error) {
	return _IUsdWithdrawalOutputBuilder.Contract.BuildWithdrawalOutput(&_IUsdWithdrawalOutputBuilder.CallOpts, appContract, account)
}

// Token is a free data retrieval call binding the contract method 0xfc0c546a.
//
// Solidity: function token() view returns(address)
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderCaller) Token(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IUsdWithdrawalOutputBuilder.contract.Call(opts, &out, "token")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Token is a free data retrieval call binding the contract method 0xfc0c546a.
//
// Solidity: function token() view returns(address)
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderSession) Token() (common.Address, error) {
	return _IUsdWithdrawalOutputBuilder.Contract.Token(&_IUsdWithdrawalOutputBuilder.CallOpts)
}

// Token is a free data retrieval call binding the contract method 0xfc0c546a.
//
// Solidity: function token() view returns(address)
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderCallerSession) Token() (common.Address, error) {
	return _IUsdWithdrawalOutputBuilder.Contract.Token(&_IUsdWithdrawalOutputBuilder.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderCaller) Version(opts *bind.CallOpts) (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	var out []interface{}
	err := _IUsdWithdrawalOutputBuilder.contract.Call(opts, &out, "version")

	outstruct := new(struct {
		Major         uint64
		Minor         uint64
		Patch         uint64
		PreRelease    string
		BuildMetadata string
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Major = *abi.ConvertType(out[0], new(uint64)).(*uint64)
	outstruct.Minor = *abi.ConvertType(out[1], new(uint64)).(*uint64)
	outstruct.Patch = *abi.ConvertType(out[2], new(uint64)).(*uint64)
	outstruct.PreRelease = *abi.ConvertType(out[3], new(string)).(*string)
	outstruct.BuildMetadata = *abi.ConvertType(out[4], new(string)).(*string)

	return *outstruct, err

}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderSession) Version() (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	return _IUsdWithdrawalOutputBuilder.Contract.Version(&_IUsdWithdrawalOutputBuilder.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IUsdWithdrawalOutputBuilder *IUsdWithdrawalOutputBuilderCallerSession) Version() (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	return _IUsdWithdrawalOutputBuilder.Contract.Version(&_IUsdWithdrawalOutputBuilder.CallOpts)
}
