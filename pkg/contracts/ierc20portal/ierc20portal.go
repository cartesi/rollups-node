// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package ierc20portal

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

// IERC20PortalMetaData contains all meta data concerning the IERC20Portal contract.
var IERC20PortalMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"depositERC20Tokens\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"contractIERC20\"},{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"value\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"execLayerData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getInputBox\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractIInputBox\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"major\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"minor\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"patch\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"preRelease\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"buildMetadata\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"error\",\"name\":\"ERC20TransferFailed\",\"inputs\":[]}]",
}

// IERC20PortalABI is the input ABI used to generate the binding from.
// Deprecated: Use IERC20PortalMetaData.ABI instead.
var IERC20PortalABI = IERC20PortalMetaData.ABI

// IERC20Portal is an auto generated Go binding around an Ethereum contract.
type IERC20Portal struct {
	IERC20PortalCaller     // Read-only binding to the contract
	IERC20PortalTransactor // Write-only binding to the contract
	IERC20PortalFilterer   // Log filterer for contract events
}

// IERC20PortalCaller is an auto generated read-only Go binding around an Ethereum contract.
type IERC20PortalCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IERC20PortalTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IERC20PortalTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IERC20PortalFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IERC20PortalFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IERC20PortalSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IERC20PortalSession struct {
	Contract     *IERC20Portal     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IERC20PortalCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IERC20PortalCallerSession struct {
	Contract *IERC20PortalCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// IERC20PortalTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IERC20PortalTransactorSession struct {
	Contract     *IERC20PortalTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// IERC20PortalRaw is an auto generated low-level Go binding around an Ethereum contract.
type IERC20PortalRaw struct {
	Contract *IERC20Portal // Generic contract binding to access the raw methods on
}

// IERC20PortalCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IERC20PortalCallerRaw struct {
	Contract *IERC20PortalCaller // Generic read-only contract binding to access the raw methods on
}

// IERC20PortalTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IERC20PortalTransactorRaw struct {
	Contract *IERC20PortalTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIERC20Portal creates a new instance of IERC20Portal, bound to a specific deployed contract.
func NewIERC20Portal(address common.Address, backend bind.ContractBackend) (*IERC20Portal, error) {
	contract, err := bindIERC20Portal(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IERC20Portal{IERC20PortalCaller: IERC20PortalCaller{contract: contract}, IERC20PortalTransactor: IERC20PortalTransactor{contract: contract}, IERC20PortalFilterer: IERC20PortalFilterer{contract: contract}}, nil
}

// NewIERC20PortalCaller creates a new read-only instance of IERC20Portal, bound to a specific deployed contract.
func NewIERC20PortalCaller(address common.Address, caller bind.ContractCaller) (*IERC20PortalCaller, error) {
	contract, err := bindIERC20Portal(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IERC20PortalCaller{contract: contract}, nil
}

// NewIERC20PortalTransactor creates a new write-only instance of IERC20Portal, bound to a specific deployed contract.
func NewIERC20PortalTransactor(address common.Address, transactor bind.ContractTransactor) (*IERC20PortalTransactor, error) {
	contract, err := bindIERC20Portal(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IERC20PortalTransactor{contract: contract}, nil
}

// NewIERC20PortalFilterer creates a new log filterer instance of IERC20Portal, bound to a specific deployed contract.
func NewIERC20PortalFilterer(address common.Address, filterer bind.ContractFilterer) (*IERC20PortalFilterer, error) {
	contract, err := bindIERC20Portal(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IERC20PortalFilterer{contract: contract}, nil
}

// bindIERC20Portal binds a generic wrapper to an already deployed contract.
func bindIERC20Portal(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IERC20PortalMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IERC20Portal *IERC20PortalRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IERC20Portal.Contract.IERC20PortalCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IERC20Portal *IERC20PortalRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IERC20Portal.Contract.IERC20PortalTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IERC20Portal *IERC20PortalRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IERC20Portal.Contract.IERC20PortalTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IERC20Portal *IERC20PortalCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IERC20Portal.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IERC20Portal *IERC20PortalTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IERC20Portal.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IERC20Portal *IERC20PortalTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IERC20Portal.Contract.contract.Transact(opts, method, params...)
}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_IERC20Portal *IERC20PortalCaller) GetInputBox(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IERC20Portal.contract.Call(opts, &out, "getInputBox")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_IERC20Portal *IERC20PortalSession) GetInputBox() (common.Address, error) {
	return _IERC20Portal.Contract.GetInputBox(&_IERC20Portal.CallOpts)
}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_IERC20Portal *IERC20PortalCallerSession) GetInputBox() (common.Address, error) {
	return _IERC20Portal.Contract.GetInputBox(&_IERC20Portal.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IERC20Portal *IERC20PortalCaller) Version(opts *bind.CallOpts) (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	var out []interface{}
	err := _IERC20Portal.contract.Call(opts, &out, "version")

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
func (_IERC20Portal *IERC20PortalSession) Version() (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	return _IERC20Portal.Contract.Version(&_IERC20Portal.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IERC20Portal *IERC20PortalCallerSession) Version() (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	return _IERC20Portal.Contract.Version(&_IERC20Portal.CallOpts)
}

// DepositERC20Tokens is a paid mutator transaction binding the contract method 0x95854b81.
//
// Solidity: function depositERC20Tokens(address token, address appContract, uint256 value, bytes execLayerData) returns()
func (_IERC20Portal *IERC20PortalTransactor) DepositERC20Tokens(opts *bind.TransactOpts, token common.Address, appContract common.Address, value *big.Int, execLayerData []byte) (*types.Transaction, error) {
	return _IERC20Portal.contract.Transact(opts, "depositERC20Tokens", token, appContract, value, execLayerData)
}

// DepositERC20Tokens is a paid mutator transaction binding the contract method 0x95854b81.
//
// Solidity: function depositERC20Tokens(address token, address appContract, uint256 value, bytes execLayerData) returns()
func (_IERC20Portal *IERC20PortalSession) DepositERC20Tokens(token common.Address, appContract common.Address, value *big.Int, execLayerData []byte) (*types.Transaction, error) {
	return _IERC20Portal.Contract.DepositERC20Tokens(&_IERC20Portal.TransactOpts, token, appContract, value, execLayerData)
}

// DepositERC20Tokens is a paid mutator transaction binding the contract method 0x95854b81.
//
// Solidity: function depositERC20Tokens(address token, address appContract, uint256 value, bytes execLayerData) returns()
func (_IERC20Portal *IERC20PortalTransactorSession) DepositERC20Tokens(token common.Address, appContract common.Address, value *big.Int, execLayerData []byte) (*types.Transaction, error) {
	return _IERC20Portal.Contract.DepositERC20Tokens(&_IERC20Portal.TransactOpts, token, appContract, value, execLayerData)
}
