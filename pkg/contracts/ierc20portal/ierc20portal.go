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

// IErc20PortalMetaData contains all meta data concerning the IErc20Portal contract.
var IErc20PortalMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"depositErc20Tokens\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"contractIERC20\"},{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"value\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"execLayerData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"major\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"minor\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"patch\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"preRelease\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"buildMetadata\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"error\",\"name\":\"ApplicationForeclosed\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ApplicationNotDeployed\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ApplicationReverted\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"error\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"Erc20TransferDecreasedApplicationBalance\",\"inputs\":[{\"name\":\"balanceBefore\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"balanceAfter\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"Erc20TransferFailed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"Erc20TransferValueIsNotBalanceDelta\",\"inputs\":[{\"name\":\"value\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"balanceDelta\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"IllformedApplicationReturnData\",\"inputs\":[{\"name\":\"appContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"InputBoxNotDeployed\",\"inputs\":[{\"name\":\"inputBox\",\"type\":\"address\",\"internalType\":\"address\"}]}]",
}

// IErc20PortalABI is the input ABI used to generate the binding from.
// Deprecated: Use IErc20PortalMetaData.ABI instead.
var IErc20PortalABI = IErc20PortalMetaData.ABI

// IErc20Portal is an auto generated Go binding around an Ethereum contract.
type IErc20Portal struct {
	IErc20PortalCaller     // Read-only binding to the contract
	IErc20PortalTransactor // Write-only binding to the contract
	IErc20PortalFilterer   // Log filterer for contract events
}

// IErc20PortalCaller is an auto generated read-only Go binding around an Ethereum contract.
type IErc20PortalCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IErc20PortalTransactor is an auto generated write-only Go binding around an Ethereum contract.
type IErc20PortalTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IErc20PortalFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type IErc20PortalFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// IErc20PortalSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type IErc20PortalSession struct {
	Contract     *IErc20Portal     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// IErc20PortalCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type IErc20PortalCallerSession struct {
	Contract *IErc20PortalCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// IErc20PortalTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type IErc20PortalTransactorSession struct {
	Contract     *IErc20PortalTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// IErc20PortalRaw is an auto generated low-level Go binding around an Ethereum contract.
type IErc20PortalRaw struct {
	Contract *IErc20Portal // Generic contract binding to access the raw methods on
}

// IErc20PortalCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type IErc20PortalCallerRaw struct {
	Contract *IErc20PortalCaller // Generic read-only contract binding to access the raw methods on
}

// IErc20PortalTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type IErc20PortalTransactorRaw struct {
	Contract *IErc20PortalTransactor // Generic write-only contract binding to access the raw methods on
}

// NewIErc20Portal creates a new instance of IErc20Portal, bound to a specific deployed contract.
func NewIErc20Portal(address common.Address, backend bind.ContractBackend) (*IErc20Portal, error) {
	contract, err := bindIErc20Portal(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &IErc20Portal{IErc20PortalCaller: IErc20PortalCaller{contract: contract}, IErc20PortalTransactor: IErc20PortalTransactor{contract: contract}, IErc20PortalFilterer: IErc20PortalFilterer{contract: contract}}, nil
}

// NewIErc20PortalCaller creates a new read-only instance of IErc20Portal, bound to a specific deployed contract.
func NewIErc20PortalCaller(address common.Address, caller bind.ContractCaller) (*IErc20PortalCaller, error) {
	contract, err := bindIErc20Portal(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &IErc20PortalCaller{contract: contract}, nil
}

// NewIErc20PortalTransactor creates a new write-only instance of IErc20Portal, bound to a specific deployed contract.
func NewIErc20PortalTransactor(address common.Address, transactor bind.ContractTransactor) (*IErc20PortalTransactor, error) {
	contract, err := bindIErc20Portal(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &IErc20PortalTransactor{contract: contract}, nil
}

// NewIErc20PortalFilterer creates a new log filterer instance of IErc20Portal, bound to a specific deployed contract.
func NewIErc20PortalFilterer(address common.Address, filterer bind.ContractFilterer) (*IErc20PortalFilterer, error) {
	contract, err := bindIErc20Portal(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &IErc20PortalFilterer{contract: contract}, nil
}

// bindIErc20Portal binds a generic wrapper to an already deployed contract.
func bindIErc20Portal(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := IErc20PortalMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IErc20Portal *IErc20PortalRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IErc20Portal.Contract.IErc20PortalCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IErc20Portal *IErc20PortalRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IErc20Portal.Contract.IErc20PortalTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IErc20Portal *IErc20PortalRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IErc20Portal.Contract.IErc20PortalTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_IErc20Portal *IErc20PortalCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _IErc20Portal.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_IErc20Portal *IErc20PortalTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IErc20Portal.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_IErc20Portal *IErc20PortalTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _IErc20Portal.Contract.contract.Transact(opts, method, params...)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IErc20Portal *IErc20PortalCaller) Version(opts *bind.CallOpts) (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	var out []interface{}
	err := _IErc20Portal.contract.Call(opts, &out, "version")

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
func (_IErc20Portal *IErc20PortalSession) Version() (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	return _IErc20Portal.Contract.Version(&_IErc20Portal.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IErc20Portal *IErc20PortalCallerSession) Version() (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	return _IErc20Portal.Contract.Version(&_IErc20Portal.CallOpts)
}

// DepositErc20Tokens is a paid mutator transaction binding the contract method 0x766afa3a.
//
// Solidity: function depositErc20Tokens(address token, address appContract, uint256 value, bytes execLayerData) returns()
func (_IErc20Portal *IErc20PortalTransactor) DepositErc20Tokens(opts *bind.TransactOpts, token common.Address, appContract common.Address, value *big.Int, execLayerData []byte) (*types.Transaction, error) {
	return _IErc20Portal.contract.Transact(opts, "depositErc20Tokens", token, appContract, value, execLayerData)
}

// DepositErc20Tokens is a paid mutator transaction binding the contract method 0x766afa3a.
//
// Solidity: function depositErc20Tokens(address token, address appContract, uint256 value, bytes execLayerData) returns()
func (_IErc20Portal *IErc20PortalSession) DepositErc20Tokens(token common.Address, appContract common.Address, value *big.Int, execLayerData []byte) (*types.Transaction, error) {
	return _IErc20Portal.Contract.DepositErc20Tokens(&_IErc20Portal.TransactOpts, token, appContract, value, execLayerData)
}

// DepositErc20Tokens is a paid mutator transaction binding the contract method 0x766afa3a.
//
// Solidity: function depositErc20Tokens(address token, address appContract, uint256 value, bytes execLayerData) returns()
func (_IErc20Portal *IErc20PortalTransactorSession) DepositErc20Tokens(token common.Address, appContract common.Address, value *big.Int, execLayerData []byte) (*types.Transaction, error) {
	return _IErc20Portal.Contract.DepositErc20Tokens(&_IErc20Portal.TransactOpts, token, appContract, value, execLayerData)
}
