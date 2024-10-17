// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package iselfhostedapplicationfactory

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

// ISelfHostedApplicationFactoryMetaData contains all meta data concerning the ISelfHostedApplicationFactory contract.
var ISelfHostedApplicationFactoryMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"authorityOwner\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"epochLength\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"appOwner\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"templateHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"salt\",\"type\":\"bytes32\"}],\"name\":\"calculateAddresses\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"authorityOwner\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"epochLength\",\"type\":\"uint256\"},{\"internalType\":\"address\",\"name\":\"appOwner\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"templateHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"salt\",\"type\":\"bytes32\"}],\"name\":\"deployContracts\",\"outputs\":[{\"internalType\":\"contractIApplication\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"contractIAuthority\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getApplicationFactory\",\"outputs\":[{\"internalType\":\"contractIApplicationFactory\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getAuthorityFactory\",\"outputs\":[{\"internalType\":\"contractIAuthorityFactory\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// ISelfHostedApplicationFactoryABI is the input ABI used to generate the binding from.
// Deprecated: Use ISelfHostedApplicationFactoryMetaData.ABI instead.
var ISelfHostedApplicationFactoryABI = ISelfHostedApplicationFactoryMetaData.ABI

// ISelfHostedApplicationFactory is an auto generated Go binding around an Ethereum contract.
type ISelfHostedApplicationFactory struct {
	ISelfHostedApplicationFactoryCaller     // Read-only binding to the contract
	ISelfHostedApplicationFactoryTransactor // Write-only binding to the contract
	ISelfHostedApplicationFactoryFilterer   // Log filterer for contract events
}

// ISelfHostedApplicationFactoryCaller is an auto generated read-only Go binding around an Ethereum contract.
type ISelfHostedApplicationFactoryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ISelfHostedApplicationFactoryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ISelfHostedApplicationFactoryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ISelfHostedApplicationFactoryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ISelfHostedApplicationFactoryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ISelfHostedApplicationFactorySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ISelfHostedApplicationFactorySession struct {
	Contract     *ISelfHostedApplicationFactory // Generic contract binding to set the session for
	CallOpts     bind.CallOpts                  // Call options to use throughout this session
	TransactOpts bind.TransactOpts              // Transaction auth options to use throughout this session
}

// ISelfHostedApplicationFactoryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ISelfHostedApplicationFactoryCallerSession struct {
	Contract *ISelfHostedApplicationFactoryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                        // Call options to use throughout this session
}

// ISelfHostedApplicationFactoryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ISelfHostedApplicationFactoryTransactorSession struct {
	Contract     *ISelfHostedApplicationFactoryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                        // Transaction auth options to use throughout this session
}

// ISelfHostedApplicationFactoryRaw is an auto generated low-level Go binding around an Ethereum contract.
type ISelfHostedApplicationFactoryRaw struct {
	Contract *ISelfHostedApplicationFactory // Generic contract binding to access the raw methods on
}

// ISelfHostedApplicationFactoryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ISelfHostedApplicationFactoryCallerRaw struct {
	Contract *ISelfHostedApplicationFactoryCaller // Generic read-only contract binding to access the raw methods on
}

// ISelfHostedApplicationFactoryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ISelfHostedApplicationFactoryTransactorRaw struct {
	Contract *ISelfHostedApplicationFactoryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewISelfHostedApplicationFactory creates a new instance of ISelfHostedApplicationFactory, bound to a specific deployed contract.
func NewISelfHostedApplicationFactory(address common.Address, backend bind.ContractBackend) (*ISelfHostedApplicationFactory, error) {
	contract, err := bindISelfHostedApplicationFactory(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &ISelfHostedApplicationFactory{ISelfHostedApplicationFactoryCaller: ISelfHostedApplicationFactoryCaller{contract: contract}, ISelfHostedApplicationFactoryTransactor: ISelfHostedApplicationFactoryTransactor{contract: contract}, ISelfHostedApplicationFactoryFilterer: ISelfHostedApplicationFactoryFilterer{contract: contract}}, nil
}

// NewISelfHostedApplicationFactoryCaller creates a new read-only instance of ISelfHostedApplicationFactory, bound to a specific deployed contract.
func NewISelfHostedApplicationFactoryCaller(address common.Address, caller bind.ContractCaller) (*ISelfHostedApplicationFactoryCaller, error) {
	contract, err := bindISelfHostedApplicationFactory(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ISelfHostedApplicationFactoryCaller{contract: contract}, nil
}

// NewISelfHostedApplicationFactoryTransactor creates a new write-only instance of ISelfHostedApplicationFactory, bound to a specific deployed contract.
func NewISelfHostedApplicationFactoryTransactor(address common.Address, transactor bind.ContractTransactor) (*ISelfHostedApplicationFactoryTransactor, error) {
	contract, err := bindISelfHostedApplicationFactory(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ISelfHostedApplicationFactoryTransactor{contract: contract}, nil
}

// NewISelfHostedApplicationFactoryFilterer creates a new log filterer instance of ISelfHostedApplicationFactory, bound to a specific deployed contract.
func NewISelfHostedApplicationFactoryFilterer(address common.Address, filterer bind.ContractFilterer) (*ISelfHostedApplicationFactoryFilterer, error) {
	contract, err := bindISelfHostedApplicationFactory(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ISelfHostedApplicationFactoryFilterer{contract: contract}, nil
}

// bindISelfHostedApplicationFactory binds a generic wrapper to an already deployed contract.
func bindISelfHostedApplicationFactory(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := ISelfHostedApplicationFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactoryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ISelfHostedApplicationFactory.Contract.ISelfHostedApplicationFactoryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactoryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ISelfHostedApplicationFactory.Contract.ISelfHostedApplicationFactoryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactoryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ISelfHostedApplicationFactory.Contract.ISelfHostedApplicationFactoryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactoryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ISelfHostedApplicationFactory.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactoryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ISelfHostedApplicationFactory.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactoryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ISelfHostedApplicationFactory.Contract.contract.Transact(opts, method, params...)
}

// CalculateAddresses is a free data retrieval call binding the contract method 0xde4d53fd.
//
// Solidity: function calculateAddresses(address authorityOwner, uint256 epochLength, address appOwner, bytes32 templateHash, bytes32 salt) view returns(address, address)
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactoryCaller) CalculateAddresses(opts *bind.CallOpts, authorityOwner common.Address, epochLength *big.Int, appOwner common.Address, templateHash [32]byte, salt [32]byte) (common.Address, common.Address, error) {
	var out []interface{}
	err := _ISelfHostedApplicationFactory.contract.Call(opts, &out, "calculateAddresses", authorityOwner, epochLength, appOwner, templateHash, salt)

	if err != nil {
		return *new(common.Address), *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	out1 := *abi.ConvertType(out[1], new(common.Address)).(*common.Address)

	return out0, out1, err

}

// CalculateAddresses is a free data retrieval call binding the contract method 0xde4d53fd.
//
// Solidity: function calculateAddresses(address authorityOwner, uint256 epochLength, address appOwner, bytes32 templateHash, bytes32 salt) view returns(address, address)
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactorySession) CalculateAddresses(authorityOwner common.Address, epochLength *big.Int, appOwner common.Address, templateHash [32]byte, salt [32]byte) (common.Address, common.Address, error) {
	return _ISelfHostedApplicationFactory.Contract.CalculateAddresses(&_ISelfHostedApplicationFactory.CallOpts, authorityOwner, epochLength, appOwner, templateHash, salt)
}

// CalculateAddresses is a free data retrieval call binding the contract method 0xde4d53fd.
//
// Solidity: function calculateAddresses(address authorityOwner, uint256 epochLength, address appOwner, bytes32 templateHash, bytes32 salt) view returns(address, address)
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactoryCallerSession) CalculateAddresses(authorityOwner common.Address, epochLength *big.Int, appOwner common.Address, templateHash [32]byte, salt [32]byte) (common.Address, common.Address, error) {
	return _ISelfHostedApplicationFactory.Contract.CalculateAddresses(&_ISelfHostedApplicationFactory.CallOpts, authorityOwner, epochLength, appOwner, templateHash, salt)
}

// GetApplicationFactory is a free data retrieval call binding the contract method 0xe63d50ff.
//
// Solidity: function getApplicationFactory() view returns(address)
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactoryCaller) GetApplicationFactory(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _ISelfHostedApplicationFactory.contract.Call(opts, &out, "getApplicationFactory")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetApplicationFactory is a free data retrieval call binding the contract method 0xe63d50ff.
//
// Solidity: function getApplicationFactory() view returns(address)
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactorySession) GetApplicationFactory() (common.Address, error) {
	return _ISelfHostedApplicationFactory.Contract.GetApplicationFactory(&_ISelfHostedApplicationFactory.CallOpts)
}

// GetApplicationFactory is a free data retrieval call binding the contract method 0xe63d50ff.
//
// Solidity: function getApplicationFactory() view returns(address)
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactoryCallerSession) GetApplicationFactory() (common.Address, error) {
	return _ISelfHostedApplicationFactory.Contract.GetApplicationFactory(&_ISelfHostedApplicationFactory.CallOpts)
}

// GetAuthorityFactory is a free data retrieval call binding the contract method 0x75689f83.
//
// Solidity: function getAuthorityFactory() view returns(address)
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactoryCaller) GetAuthorityFactory(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _ISelfHostedApplicationFactory.contract.Call(opts, &out, "getAuthorityFactory")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetAuthorityFactory is a free data retrieval call binding the contract method 0x75689f83.
//
// Solidity: function getAuthorityFactory() view returns(address)
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactorySession) GetAuthorityFactory() (common.Address, error) {
	return _ISelfHostedApplicationFactory.Contract.GetAuthorityFactory(&_ISelfHostedApplicationFactory.CallOpts)
}

// GetAuthorityFactory is a free data retrieval call binding the contract method 0x75689f83.
//
// Solidity: function getAuthorityFactory() view returns(address)
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactoryCallerSession) GetAuthorityFactory() (common.Address, error) {
	return _ISelfHostedApplicationFactory.Contract.GetAuthorityFactory(&_ISelfHostedApplicationFactory.CallOpts)
}

// DeployContracts is a paid mutator transaction binding the contract method 0xffc643ca.
//
// Solidity: function deployContracts(address authorityOwner, uint256 epochLength, address appOwner, bytes32 templateHash, bytes32 salt) returns(address, address)
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactoryTransactor) DeployContracts(opts *bind.TransactOpts, authorityOwner common.Address, epochLength *big.Int, appOwner common.Address, templateHash [32]byte, salt [32]byte) (*types.Transaction, error) {
	return _ISelfHostedApplicationFactory.contract.Transact(opts, "deployContracts", authorityOwner, epochLength, appOwner, templateHash, salt)
}

// DeployContracts is a paid mutator transaction binding the contract method 0xffc643ca.
//
// Solidity: function deployContracts(address authorityOwner, uint256 epochLength, address appOwner, bytes32 templateHash, bytes32 salt) returns(address, address)
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactorySession) DeployContracts(authorityOwner common.Address, epochLength *big.Int, appOwner common.Address, templateHash [32]byte, salt [32]byte) (*types.Transaction, error) {
	return _ISelfHostedApplicationFactory.Contract.DeployContracts(&_ISelfHostedApplicationFactory.TransactOpts, authorityOwner, epochLength, appOwner, templateHash, salt)
}

// DeployContracts is a paid mutator transaction binding the contract method 0xffc643ca.
//
// Solidity: function deployContracts(address authorityOwner, uint256 epochLength, address appOwner, bytes32 templateHash, bytes32 salt) returns(address, address)
func (_ISelfHostedApplicationFactory *ISelfHostedApplicationFactoryTransactorSession) DeployContracts(authorityOwner common.Address, epochLength *big.Int, appOwner common.Address, templateHash [32]byte, salt [32]byte) (*types.Transaction, error) {
	return _ISelfHostedApplicationFactory.Contract.DeployContracts(&_ISelfHostedApplicationFactory.TransactOpts, authorityOwner, epochLength, appOwner, templateHash, salt)
}
