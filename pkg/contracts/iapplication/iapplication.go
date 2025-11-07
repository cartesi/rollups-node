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
	ABI: "[{\"type\":\"function\",\"name\":\"executeOutput\",\"inputs\":[{\"name\":\"output\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"proof\",\"type\":\"tuple\",\"internalType\":\"structOutputValidityProof\",\"components\":[{\"name\":\"outputIndex\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"outputHashesSiblings\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getDataAvailability\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDeploymentBlockNumber\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNumberOfExecutedOutputs\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getOutputsMerkleRootValidator\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractIOutputsMerkleRootValidator\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getTemplateHash\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"migrateToOutputsMerkleRootValidator\",\"inputs\":[{\"name\":\"newOutputsMerkleRootValidator\",\"type\":\"address\",\"internalType\":\"contractIOutputsMerkleRootValidator\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"validateOutput\",\"inputs\":[{\"name\":\"output\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"proof\",\"type\":\"tuple\",\"internalType\":\"structOutputValidityProof\",\"components\":[{\"name\":\"outputIndex\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"outputHashesSiblings\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}]}],\"outputs\":[],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"validateOutputHash\",\"inputs\":[{\"name\":\"outputHash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"proof\",\"type\":\"tuple\",\"internalType\":\"structOutputValidityProof\",\"components\":[{\"name\":\"outputIndex\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"outputHashesSiblings\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}]}],\"outputs\":[],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"wasOutputExecuted\",\"inputs\":[{\"name\":\"outputIndex\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"OutputExecuted\",\"inputs\":[{\"name\":\"outputIndex\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"},{\"name\":\"output\",\"type\":\"bytes\",\"indexed\":false,\"internalType\":\"bytes\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OutputsMerkleRootValidatorChanged\",\"inputs\":[{\"name\":\"newOutputsMerkleRootValidator\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractIOutputsMerkleRootValidator\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"InsufficientFunds\",\"inputs\":[{\"name\":\"value\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"balance\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidOutputHashesSiblingsArrayLength\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidOutputsMerkleRoot\",\"inputs\":[{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"OutputNotExecutable\",\"inputs\":[{\"name\":\"output\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"OutputNotReexecutable\",\"inputs\":[{\"name\":\"output\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]}]",
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

// GetDataAvailability is a free data retrieval call binding the contract method 0xf02478de.
//
// Solidity: function getDataAvailability() view returns(bytes)
func (_IApplication *IApplicationCaller) GetDataAvailability(opts *bind.CallOpts) ([]byte, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "getDataAvailability")

	if err != nil {
		return *new([]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([]byte)).(*[]byte)

	return out0, err

}

// GetDataAvailability is a free data retrieval call binding the contract method 0xf02478de.
//
// Solidity: function getDataAvailability() view returns(bytes)
func (_IApplication *IApplicationSession) GetDataAvailability() ([]byte, error) {
	return _IApplication.Contract.GetDataAvailability(&_IApplication.CallOpts)
}

// GetDataAvailability is a free data retrieval call binding the contract method 0xf02478de.
//
// Solidity: function getDataAvailability() view returns(bytes)
func (_IApplication *IApplicationCallerSession) GetDataAvailability() ([]byte, error) {
	return _IApplication.Contract.GetDataAvailability(&_IApplication.CallOpts)
}

// GetDeploymentBlockNumber is a free data retrieval call binding the contract method 0xb3a1acd8.
//
// Solidity: function getDeploymentBlockNumber() view returns(uint256)
func (_IApplication *IApplicationCaller) GetDeploymentBlockNumber(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "getDeploymentBlockNumber")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetDeploymentBlockNumber is a free data retrieval call binding the contract method 0xb3a1acd8.
//
// Solidity: function getDeploymentBlockNumber() view returns(uint256)
func (_IApplication *IApplicationSession) GetDeploymentBlockNumber() (*big.Int, error) {
	return _IApplication.Contract.GetDeploymentBlockNumber(&_IApplication.CallOpts)
}

// GetDeploymentBlockNumber is a free data retrieval call binding the contract method 0xb3a1acd8.
//
// Solidity: function getDeploymentBlockNumber() view returns(uint256)
func (_IApplication *IApplicationCallerSession) GetDeploymentBlockNumber() (*big.Int, error) {
	return _IApplication.Contract.GetDeploymentBlockNumber(&_IApplication.CallOpts)
}

// GetNumberOfExecutedOutputs is a free data retrieval call binding the contract method 0xe64fab4d.
//
// Solidity: function getNumberOfExecutedOutputs() view returns(uint256)
func (_IApplication *IApplicationCaller) GetNumberOfExecutedOutputs(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "getNumberOfExecutedOutputs")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNumberOfExecutedOutputs is a free data retrieval call binding the contract method 0xe64fab4d.
//
// Solidity: function getNumberOfExecutedOutputs() view returns(uint256)
func (_IApplication *IApplicationSession) GetNumberOfExecutedOutputs() (*big.Int, error) {
	return _IApplication.Contract.GetNumberOfExecutedOutputs(&_IApplication.CallOpts)
}

// GetNumberOfExecutedOutputs is a free data retrieval call binding the contract method 0xe64fab4d.
//
// Solidity: function getNumberOfExecutedOutputs() view returns(uint256)
func (_IApplication *IApplicationCallerSession) GetNumberOfExecutedOutputs() (*big.Int, error) {
	return _IApplication.Contract.GetNumberOfExecutedOutputs(&_IApplication.CallOpts)
}

// GetOutputsMerkleRootValidator is a free data retrieval call binding the contract method 0xa94dfc5a.
//
// Solidity: function getOutputsMerkleRootValidator() view returns(address)
func (_IApplication *IApplicationCaller) GetOutputsMerkleRootValidator(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "getOutputsMerkleRootValidator")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetOutputsMerkleRootValidator is a free data retrieval call binding the contract method 0xa94dfc5a.
//
// Solidity: function getOutputsMerkleRootValidator() view returns(address)
func (_IApplication *IApplicationSession) GetOutputsMerkleRootValidator() (common.Address, error) {
	return _IApplication.Contract.GetOutputsMerkleRootValidator(&_IApplication.CallOpts)
}

// GetOutputsMerkleRootValidator is a free data retrieval call binding the contract method 0xa94dfc5a.
//
// Solidity: function getOutputsMerkleRootValidator() view returns(address)
func (_IApplication *IApplicationCallerSession) GetOutputsMerkleRootValidator() (common.Address, error) {
	return _IApplication.Contract.GetOutputsMerkleRootValidator(&_IApplication.CallOpts)
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

// MigrateToOutputsMerkleRootValidator is a paid mutator transaction binding the contract method 0xbf8abff8.
//
// Solidity: function migrateToOutputsMerkleRootValidator(address newOutputsMerkleRootValidator) returns()
func (_IApplication *IApplicationTransactor) MigrateToOutputsMerkleRootValidator(opts *bind.TransactOpts, newOutputsMerkleRootValidator common.Address) (*types.Transaction, error) {
	return _IApplication.contract.Transact(opts, "migrateToOutputsMerkleRootValidator", newOutputsMerkleRootValidator)
}

// MigrateToOutputsMerkleRootValidator is a paid mutator transaction binding the contract method 0xbf8abff8.
//
// Solidity: function migrateToOutputsMerkleRootValidator(address newOutputsMerkleRootValidator) returns()
func (_IApplication *IApplicationSession) MigrateToOutputsMerkleRootValidator(newOutputsMerkleRootValidator common.Address) (*types.Transaction, error) {
	return _IApplication.Contract.MigrateToOutputsMerkleRootValidator(&_IApplication.TransactOpts, newOutputsMerkleRootValidator)
}

// MigrateToOutputsMerkleRootValidator is a paid mutator transaction binding the contract method 0xbf8abff8.
//
// Solidity: function migrateToOutputsMerkleRootValidator(address newOutputsMerkleRootValidator) returns()
func (_IApplication *IApplicationTransactorSession) MigrateToOutputsMerkleRootValidator(newOutputsMerkleRootValidator common.Address) (*types.Transaction, error) {
	return _IApplication.Contract.MigrateToOutputsMerkleRootValidator(&_IApplication.TransactOpts, newOutputsMerkleRootValidator)
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

// IApplicationOutputsMerkleRootValidatorChangedIterator is returned from FilterOutputsMerkleRootValidatorChanged and is used to iterate over the raw logs and unpacked data for OutputsMerkleRootValidatorChanged events raised by the IApplication contract.
type IApplicationOutputsMerkleRootValidatorChangedIterator struct {
	Event *IApplicationOutputsMerkleRootValidatorChanged // Event containing the contract specifics and raw log

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
func (it *IApplicationOutputsMerkleRootValidatorChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IApplicationOutputsMerkleRootValidatorChanged)
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
		it.Event = new(IApplicationOutputsMerkleRootValidatorChanged)
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
func (it *IApplicationOutputsMerkleRootValidatorChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IApplicationOutputsMerkleRootValidatorChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IApplicationOutputsMerkleRootValidatorChanged represents a OutputsMerkleRootValidatorChanged event raised by the IApplication contract.
type IApplicationOutputsMerkleRootValidatorChanged struct {
	NewOutputsMerkleRootValidator common.Address
	Raw                           types.Log // Blockchain specific contextual infos
}

// FilterOutputsMerkleRootValidatorChanged is a free log retrieval operation binding the contract event 0x6ad3188ba8f430fba0656cb0a7e839ab2020d5586ba11a1477d18f7092f8bece.
//
// Solidity: event OutputsMerkleRootValidatorChanged(address newOutputsMerkleRootValidator)
func (_IApplication *IApplicationFilterer) FilterOutputsMerkleRootValidatorChanged(opts *bind.FilterOpts) (*IApplicationOutputsMerkleRootValidatorChangedIterator, error) {

	logs, sub, err := _IApplication.contract.FilterLogs(opts, "OutputsMerkleRootValidatorChanged")
	if err != nil {
		return nil, err
	}
	return &IApplicationOutputsMerkleRootValidatorChangedIterator{contract: _IApplication.contract, event: "OutputsMerkleRootValidatorChanged", logs: logs, sub: sub}, nil
}

// WatchOutputsMerkleRootValidatorChanged is a free log subscription operation binding the contract event 0x6ad3188ba8f430fba0656cb0a7e839ab2020d5586ba11a1477d18f7092f8bece.
//
// Solidity: event OutputsMerkleRootValidatorChanged(address newOutputsMerkleRootValidator)
func (_IApplication *IApplicationFilterer) WatchOutputsMerkleRootValidatorChanged(opts *bind.WatchOpts, sink chan<- *IApplicationOutputsMerkleRootValidatorChanged) (event.Subscription, error) {

	logs, sub, err := _IApplication.contract.WatchLogs(opts, "OutputsMerkleRootValidatorChanged")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IApplicationOutputsMerkleRootValidatorChanged)
				if err := _IApplication.contract.UnpackLog(event, "OutputsMerkleRootValidatorChanged", log); err != nil {
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

// ParseOutputsMerkleRootValidatorChanged is a log parse operation binding the contract event 0x6ad3188ba8f430fba0656cb0a7e839ab2020d5586ba11a1477d18f7092f8bece.
//
// Solidity: event OutputsMerkleRootValidatorChanged(address newOutputsMerkleRootValidator)
func (_IApplication *IApplicationFilterer) ParseOutputsMerkleRootValidatorChanged(log types.Log) (*IApplicationOutputsMerkleRootValidatorChanged, error) {
	event := new(IApplicationOutputsMerkleRootValidatorChanged)
	if err := _IApplication.contract.UnpackLog(event, "OutputsMerkleRootValidatorChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
