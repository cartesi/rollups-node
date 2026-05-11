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

// AccountValidityProof is an auto generated low-level Go binding around an user-defined struct.
type AccountValidityProof struct {
	AccountIndex        uint64
	AccountRootSiblings [][32]byte
}

// OutputValidityProof is an auto generated low-level Go binding around an user-defined struct.
type OutputValidityProof struct {
	OutputIndex          uint64
	OutputHashesSiblings [][32]byte
}

// WithdrawalConfig is an auto generated low-level Go binding around an user-defined struct.
type WithdrawalConfig struct {
	Guardian                common.Address
	Log2LeavesPerAccount    uint8
	Log2MaxNumOfAccounts    uint8
	AccountsDriveStartIndex uint64
	WithdrawalOutputBuilder common.Address
}

// IApplicationMetaData contains all meta data concerning the IApplication contract.
var IApplicationMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"executeOutput\",\"inputs\":[{\"name\":\"output\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"proof\",\"type\":\"tuple\",\"internalType\":\"structOutputValidityProof\",\"components\":[{\"name\":\"outputIndex\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"outputHashesSiblings\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"foreclose\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getAccountsDriveMerkleRoot\",\"inputs\":[],\"outputs\":[{\"name\":\"wasAccountsDriveMerkleRootProved\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"accountsDriveMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAccountsDriveStartIndex\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDataAvailability\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDeploymentBlockNumber\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getGuardian\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getLog2LeavesPerAccount\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint8\",\"internalType\":\"uint8\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getLog2MaxNumOfAccounts\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint8\",\"internalType\":\"uint8\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNumberOfExecutedOutputs\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNumberOfWithdrawals\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getOutputsMerkleRootValidator\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractIOutputsMerkleRootValidator\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getTemplateHash\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getWithdrawalConfig\",\"inputs\":[],\"outputs\":[{\"name\":\"withdrawalConfig\",\"type\":\"tuple\",\"internalType\":\"structWithdrawalConfig\",\"components\":[{\"name\":\"guardian\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"log2LeavesPerAccount\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"log2MaxNumOfAccounts\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"accountsDriveStartIndex\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"withdrawalOutputBuilder\",\"type\":\"address\",\"internalType\":\"contractIWithdrawalOutputBuilder\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getWithdrawalOutputBuilder\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractIWithdrawalOutputBuilder\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isForeclosed\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"migrateToOutputsMerkleRootValidator\",\"inputs\":[{\"name\":\"newOutputsMerkleRootValidator\",\"type\":\"address\",\"internalType\":\"contractIOutputsMerkleRootValidator\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"proveAccountsDriveMerkleRoot\",\"inputs\":[{\"name\":\"accountsDriveMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"proof\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"validateAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"proof\",\"type\":\"tuple\",\"internalType\":\"structAccountValidityProof\",\"components\":[{\"name\":\"accountIndex\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"accountRootSiblings\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}]}],\"outputs\":[],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"validateAccountMerkleRoot\",\"inputs\":[{\"name\":\"accountMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"proof\",\"type\":\"tuple\",\"internalType\":\"structAccountValidityProof\",\"components\":[{\"name\":\"accountIndex\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"accountRootSiblings\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}]}],\"outputs\":[],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"validateOutput\",\"inputs\":[{\"name\":\"output\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"proof\",\"type\":\"tuple\",\"internalType\":\"structOutputValidityProof\",\"components\":[{\"name\":\"outputIndex\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"outputHashesSiblings\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}]}],\"outputs\":[],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"validateOutputHash\",\"inputs\":[{\"name\":\"outputHash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"proof\",\"type\":\"tuple\",\"internalType\":\"structOutputValidityProof\",\"components\":[{\"name\":\"outputIndex\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"outputHashesSiblings\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}]}],\"outputs\":[],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"version\",\"inputs\":[],\"outputs\":[{\"name\":\"major\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"minor\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"patch\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"preRelease\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"buildMetadata\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"wasOutputExecuted\",\"inputs\":[{\"name\":\"outputIndex\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"wereAccountFundsWithdrawn\",\"inputs\":[{\"name\":\"accountIndex\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"withdraw\",\"inputs\":[{\"name\":\"account\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"proof\",\"type\":\"tuple\",\"internalType\":\"structAccountValidityProof\",\"components\":[{\"name\":\"accountIndex\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"accountRootSiblings\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"AccountsDriveMerkleRootProved\",\"inputs\":[{\"name\":\"accountsDriveMerkleRoot\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Foreclosure\",\"inputs\":[],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OutputExecuted\",\"inputs\":[{\"name\":\"outputIndex\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"},{\"name\":\"output\",\"type\":\"bytes\",\"indexed\":false,\"internalType\":\"bytes\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OutputsMerkleRootValidatorChanged\",\"inputs\":[{\"name\":\"newOutputsMerkleRootValidator\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"contractIOutputsMerkleRootValidator\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Withdrawal\",\"inputs\":[{\"name\":\"accountIndex\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"},{\"name\":\"account\",\"type\":\"bytes\",\"indexed\":false,\"internalType\":\"bytes\"},{\"name\":\"output\",\"type\":\"bytes\",\"indexed\":false,\"internalType\":\"bytes\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AccountFundsAlreadyWithdrawn\",\"inputs\":[{\"name\":\"accountIndex\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]},{\"type\":\"error\",\"name\":\"AccountTooShort\",\"inputs\":[{\"name\":\"attemptedAccountSize\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"minAccountSize\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]},{\"type\":\"error\",\"name\":\"AccountsDriveMerkleRootAlreadyProved\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AccountsDriveMerkleRootNotProved\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"DataBlockTooLarge\",\"inputs\":[{\"name\":\"log2DataBlockSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxLog2DataBlockSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DriveSmallerThanData\",\"inputs\":[{\"name\":\"driveSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"dataSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DriveSmallerThanDataBlock\",\"inputs\":[{\"name\":\"log2DriveSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"log2DataBlockSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DriveTooLarge\",\"inputs\":[{\"name\":\"log2DriveSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxLog2DriveSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"Foreclosed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientFunds\",\"inputs\":[{\"name\":\"value\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"balance\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidAccountRootSiblingsArrayLength\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidAccountsDriveMerkleRoot\",\"inputs\":[{\"name\":\"accountsDriveMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"InvalidAccountsDriveMerkleRootProofSize\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidMachineMerkleRoot\",\"inputs\":[{\"name\":\"machineMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"InvalidNodeIndex\",\"inputs\":[{\"name\":\"nodeIndex\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"height\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidOutputHashesSiblingsArrayLength\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidOutputsMerkleRoot\",\"inputs\":[{\"name\":\"outputsMerkleRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"NotForeclosed\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotGuardian\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OutputNotExecutable\",\"inputs\":[{\"name\":\"output\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"OutputNotReexecutable\",\"inputs\":[{\"name\":\"output\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"UnexpectedFinalStackDepth\",\"inputs\":[{\"name\":\"stackDepth\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]",
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

// GetAccountsDriveMerkleRoot is a free data retrieval call binding the contract method 0xf04ba871.
//
// Solidity: function getAccountsDriveMerkleRoot() view returns(bool wasAccountsDriveMerkleRootProved, bytes32 accountsDriveMerkleRoot)
func (_IApplication *IApplicationCaller) GetAccountsDriveMerkleRoot(opts *bind.CallOpts) (struct {
	WasAccountsDriveMerkleRootProved bool
	AccountsDriveMerkleRoot          [32]byte
}, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "getAccountsDriveMerkleRoot")

	outstruct := new(struct {
		WasAccountsDriveMerkleRootProved bool
		AccountsDriveMerkleRoot          [32]byte
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.WasAccountsDriveMerkleRootProved = *abi.ConvertType(out[0], new(bool)).(*bool)
	outstruct.AccountsDriveMerkleRoot = *abi.ConvertType(out[1], new([32]byte)).(*[32]byte)

	return *outstruct, err

}

// GetAccountsDriveMerkleRoot is a free data retrieval call binding the contract method 0xf04ba871.
//
// Solidity: function getAccountsDriveMerkleRoot() view returns(bool wasAccountsDriveMerkleRootProved, bytes32 accountsDriveMerkleRoot)
func (_IApplication *IApplicationSession) GetAccountsDriveMerkleRoot() (struct {
	WasAccountsDriveMerkleRootProved bool
	AccountsDriveMerkleRoot          [32]byte
}, error) {
	return _IApplication.Contract.GetAccountsDriveMerkleRoot(&_IApplication.CallOpts)
}

// GetAccountsDriveMerkleRoot is a free data retrieval call binding the contract method 0xf04ba871.
//
// Solidity: function getAccountsDriveMerkleRoot() view returns(bool wasAccountsDriveMerkleRootProved, bytes32 accountsDriveMerkleRoot)
func (_IApplication *IApplicationCallerSession) GetAccountsDriveMerkleRoot() (struct {
	WasAccountsDriveMerkleRootProved bool
	AccountsDriveMerkleRoot          [32]byte
}, error) {
	return _IApplication.Contract.GetAccountsDriveMerkleRoot(&_IApplication.CallOpts)
}

// GetAccountsDriveStartIndex is a free data retrieval call binding the contract method 0xab2423ad.
//
// Solidity: function getAccountsDriveStartIndex() view returns(uint64)
func (_IApplication *IApplicationCaller) GetAccountsDriveStartIndex(opts *bind.CallOpts) (uint64, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "getAccountsDriveStartIndex")

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// GetAccountsDriveStartIndex is a free data retrieval call binding the contract method 0xab2423ad.
//
// Solidity: function getAccountsDriveStartIndex() view returns(uint64)
func (_IApplication *IApplicationSession) GetAccountsDriveStartIndex() (uint64, error) {
	return _IApplication.Contract.GetAccountsDriveStartIndex(&_IApplication.CallOpts)
}

// GetAccountsDriveStartIndex is a free data retrieval call binding the contract method 0xab2423ad.
//
// Solidity: function getAccountsDriveStartIndex() view returns(uint64)
func (_IApplication *IApplicationCallerSession) GetAccountsDriveStartIndex() (uint64, error) {
	return _IApplication.Contract.GetAccountsDriveStartIndex(&_IApplication.CallOpts)
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

// GetGuardian is a free data retrieval call binding the contract method 0xa75b87d2.
//
// Solidity: function getGuardian() view returns(address)
func (_IApplication *IApplicationCaller) GetGuardian(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "getGuardian")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetGuardian is a free data retrieval call binding the contract method 0xa75b87d2.
//
// Solidity: function getGuardian() view returns(address)
func (_IApplication *IApplicationSession) GetGuardian() (common.Address, error) {
	return _IApplication.Contract.GetGuardian(&_IApplication.CallOpts)
}

// GetGuardian is a free data retrieval call binding the contract method 0xa75b87d2.
//
// Solidity: function getGuardian() view returns(address)
func (_IApplication *IApplicationCallerSession) GetGuardian() (common.Address, error) {
	return _IApplication.Contract.GetGuardian(&_IApplication.CallOpts)
}

// GetLog2LeavesPerAccount is a free data retrieval call binding the contract method 0x28a0e3c5.
//
// Solidity: function getLog2LeavesPerAccount() view returns(uint8)
func (_IApplication *IApplicationCaller) GetLog2LeavesPerAccount(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "getLog2LeavesPerAccount")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// GetLog2LeavesPerAccount is a free data retrieval call binding the contract method 0x28a0e3c5.
//
// Solidity: function getLog2LeavesPerAccount() view returns(uint8)
func (_IApplication *IApplicationSession) GetLog2LeavesPerAccount() (uint8, error) {
	return _IApplication.Contract.GetLog2LeavesPerAccount(&_IApplication.CallOpts)
}

// GetLog2LeavesPerAccount is a free data retrieval call binding the contract method 0x28a0e3c5.
//
// Solidity: function getLog2LeavesPerAccount() view returns(uint8)
func (_IApplication *IApplicationCallerSession) GetLog2LeavesPerAccount() (uint8, error) {
	return _IApplication.Contract.GetLog2LeavesPerAccount(&_IApplication.CallOpts)
}

// GetLog2MaxNumOfAccounts is a free data retrieval call binding the contract method 0xfc39b736.
//
// Solidity: function getLog2MaxNumOfAccounts() view returns(uint8)
func (_IApplication *IApplicationCaller) GetLog2MaxNumOfAccounts(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "getLog2MaxNumOfAccounts")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// GetLog2MaxNumOfAccounts is a free data retrieval call binding the contract method 0xfc39b736.
//
// Solidity: function getLog2MaxNumOfAccounts() view returns(uint8)
func (_IApplication *IApplicationSession) GetLog2MaxNumOfAccounts() (uint8, error) {
	return _IApplication.Contract.GetLog2MaxNumOfAccounts(&_IApplication.CallOpts)
}

// GetLog2MaxNumOfAccounts is a free data retrieval call binding the contract method 0xfc39b736.
//
// Solidity: function getLog2MaxNumOfAccounts() view returns(uint8)
func (_IApplication *IApplicationCallerSession) GetLog2MaxNumOfAccounts() (uint8, error) {
	return _IApplication.Contract.GetLog2MaxNumOfAccounts(&_IApplication.CallOpts)
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

// GetNumberOfWithdrawals is a free data retrieval call binding the contract method 0x0e70381b.
//
// Solidity: function getNumberOfWithdrawals() view returns(uint256)
func (_IApplication *IApplicationCaller) GetNumberOfWithdrawals(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "getNumberOfWithdrawals")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNumberOfWithdrawals is a free data retrieval call binding the contract method 0x0e70381b.
//
// Solidity: function getNumberOfWithdrawals() view returns(uint256)
func (_IApplication *IApplicationSession) GetNumberOfWithdrawals() (*big.Int, error) {
	return _IApplication.Contract.GetNumberOfWithdrawals(&_IApplication.CallOpts)
}

// GetNumberOfWithdrawals is a free data retrieval call binding the contract method 0x0e70381b.
//
// Solidity: function getNumberOfWithdrawals() view returns(uint256)
func (_IApplication *IApplicationCallerSession) GetNumberOfWithdrawals() (*big.Int, error) {
	return _IApplication.Contract.GetNumberOfWithdrawals(&_IApplication.CallOpts)
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

// GetWithdrawalConfig is a free data retrieval call binding the contract method 0x65d0c9ce.
//
// Solidity: function getWithdrawalConfig() view returns((address,uint8,uint8,uint64,address) withdrawalConfig)
func (_IApplication *IApplicationCaller) GetWithdrawalConfig(opts *bind.CallOpts) (WithdrawalConfig, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "getWithdrawalConfig")

	if err != nil {
		return *new(WithdrawalConfig), err
	}

	out0 := *abi.ConvertType(out[0], new(WithdrawalConfig)).(*WithdrawalConfig)

	return out0, err

}

// GetWithdrawalConfig is a free data retrieval call binding the contract method 0x65d0c9ce.
//
// Solidity: function getWithdrawalConfig() view returns((address,uint8,uint8,uint64,address) withdrawalConfig)
func (_IApplication *IApplicationSession) GetWithdrawalConfig() (WithdrawalConfig, error) {
	return _IApplication.Contract.GetWithdrawalConfig(&_IApplication.CallOpts)
}

// GetWithdrawalConfig is a free data retrieval call binding the contract method 0x65d0c9ce.
//
// Solidity: function getWithdrawalConfig() view returns((address,uint8,uint8,uint64,address) withdrawalConfig)
func (_IApplication *IApplicationCallerSession) GetWithdrawalConfig() (WithdrawalConfig, error) {
	return _IApplication.Contract.GetWithdrawalConfig(&_IApplication.CallOpts)
}

// GetWithdrawalOutputBuilder is a free data retrieval call binding the contract method 0x92ab68d0.
//
// Solidity: function getWithdrawalOutputBuilder() view returns(address)
func (_IApplication *IApplicationCaller) GetWithdrawalOutputBuilder(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "getWithdrawalOutputBuilder")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetWithdrawalOutputBuilder is a free data retrieval call binding the contract method 0x92ab68d0.
//
// Solidity: function getWithdrawalOutputBuilder() view returns(address)
func (_IApplication *IApplicationSession) GetWithdrawalOutputBuilder() (common.Address, error) {
	return _IApplication.Contract.GetWithdrawalOutputBuilder(&_IApplication.CallOpts)
}

// GetWithdrawalOutputBuilder is a free data retrieval call binding the contract method 0x92ab68d0.
//
// Solidity: function getWithdrawalOutputBuilder() view returns(address)
func (_IApplication *IApplicationCallerSession) GetWithdrawalOutputBuilder() (common.Address, error) {
	return _IApplication.Contract.GetWithdrawalOutputBuilder(&_IApplication.CallOpts)
}

// IsForeclosed is a free data retrieval call binding the contract method 0x83e4fbcd.
//
// Solidity: function isForeclosed() view returns(bool)
func (_IApplication *IApplicationCaller) IsForeclosed(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "isForeclosed")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsForeclosed is a free data retrieval call binding the contract method 0x83e4fbcd.
//
// Solidity: function isForeclosed() view returns(bool)
func (_IApplication *IApplicationSession) IsForeclosed() (bool, error) {
	return _IApplication.Contract.IsForeclosed(&_IApplication.CallOpts)
}

// IsForeclosed is a free data retrieval call binding the contract method 0x83e4fbcd.
//
// Solidity: function isForeclosed() view returns(bool)
func (_IApplication *IApplicationCallerSession) IsForeclosed() (bool, error) {
	return _IApplication.Contract.IsForeclosed(&_IApplication.CallOpts)
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

// ValidateAccount is a free data retrieval call binding the contract method 0x2b639720.
//
// Solidity: function validateAccount(bytes account, (uint64,bytes32[]) proof) view returns()
func (_IApplication *IApplicationCaller) ValidateAccount(opts *bind.CallOpts, account []byte, proof AccountValidityProof) error {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "validateAccount", account, proof)

	if err != nil {
		return err
	}

	return err

}

// ValidateAccount is a free data retrieval call binding the contract method 0x2b639720.
//
// Solidity: function validateAccount(bytes account, (uint64,bytes32[]) proof) view returns()
func (_IApplication *IApplicationSession) ValidateAccount(account []byte, proof AccountValidityProof) error {
	return _IApplication.Contract.ValidateAccount(&_IApplication.CallOpts, account, proof)
}

// ValidateAccount is a free data retrieval call binding the contract method 0x2b639720.
//
// Solidity: function validateAccount(bytes account, (uint64,bytes32[]) proof) view returns()
func (_IApplication *IApplicationCallerSession) ValidateAccount(account []byte, proof AccountValidityProof) error {
	return _IApplication.Contract.ValidateAccount(&_IApplication.CallOpts, account, proof)
}

// ValidateAccountMerkleRoot is a free data retrieval call binding the contract method 0x63b9c3b2.
//
// Solidity: function validateAccountMerkleRoot(bytes32 accountMerkleRoot, (uint64,bytes32[]) proof) view returns()
func (_IApplication *IApplicationCaller) ValidateAccountMerkleRoot(opts *bind.CallOpts, accountMerkleRoot [32]byte, proof AccountValidityProof) error {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "validateAccountMerkleRoot", accountMerkleRoot, proof)

	if err != nil {
		return err
	}

	return err

}

// ValidateAccountMerkleRoot is a free data retrieval call binding the contract method 0x63b9c3b2.
//
// Solidity: function validateAccountMerkleRoot(bytes32 accountMerkleRoot, (uint64,bytes32[]) proof) view returns()
func (_IApplication *IApplicationSession) ValidateAccountMerkleRoot(accountMerkleRoot [32]byte, proof AccountValidityProof) error {
	return _IApplication.Contract.ValidateAccountMerkleRoot(&_IApplication.CallOpts, accountMerkleRoot, proof)
}

// ValidateAccountMerkleRoot is a free data retrieval call binding the contract method 0x63b9c3b2.
//
// Solidity: function validateAccountMerkleRoot(bytes32 accountMerkleRoot, (uint64,bytes32[]) proof) view returns()
func (_IApplication *IApplicationCallerSession) ValidateAccountMerkleRoot(accountMerkleRoot [32]byte, proof AccountValidityProof) error {
	return _IApplication.Contract.ValidateAccountMerkleRoot(&_IApplication.CallOpts, accountMerkleRoot, proof)
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

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IApplication *IApplicationCaller) Version(opts *bind.CallOpts) (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "version")

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
func (_IApplication *IApplicationSession) Version() (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	return _IApplication.Contract.Version(&_IApplication.CallOpts)
}

// Version is a free data retrieval call binding the contract method 0x54fd4d50.
//
// Solidity: function version() view returns(uint64 major, uint64 minor, uint64 patch, string preRelease, string buildMetadata)
func (_IApplication *IApplicationCallerSession) Version() (struct {
	Major         uint64
	Minor         uint64
	Patch         uint64
	PreRelease    string
	BuildMetadata string
}, error) {
	return _IApplication.Contract.Version(&_IApplication.CallOpts)
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

// WereAccountFundsWithdrawn is a free data retrieval call binding the contract method 0x8272a6aa.
//
// Solidity: function wereAccountFundsWithdrawn(uint256 accountIndex) view returns(bool)
func (_IApplication *IApplicationCaller) WereAccountFundsWithdrawn(opts *bind.CallOpts, accountIndex *big.Int) (bool, error) {
	var out []interface{}
	err := _IApplication.contract.Call(opts, &out, "wereAccountFundsWithdrawn", accountIndex)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// WereAccountFundsWithdrawn is a free data retrieval call binding the contract method 0x8272a6aa.
//
// Solidity: function wereAccountFundsWithdrawn(uint256 accountIndex) view returns(bool)
func (_IApplication *IApplicationSession) WereAccountFundsWithdrawn(accountIndex *big.Int) (bool, error) {
	return _IApplication.Contract.WereAccountFundsWithdrawn(&_IApplication.CallOpts, accountIndex)
}

// WereAccountFundsWithdrawn is a free data retrieval call binding the contract method 0x8272a6aa.
//
// Solidity: function wereAccountFundsWithdrawn(uint256 accountIndex) view returns(bool)
func (_IApplication *IApplicationCallerSession) WereAccountFundsWithdrawn(accountIndex *big.Int) (bool, error) {
	return _IApplication.Contract.WereAccountFundsWithdrawn(&_IApplication.CallOpts, accountIndex)
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

// Foreclose is a paid mutator transaction binding the contract method 0xeb6266e2.
//
// Solidity: function foreclose() returns()
func (_IApplication *IApplicationTransactor) Foreclose(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _IApplication.contract.Transact(opts, "foreclose")
}

// Foreclose is a paid mutator transaction binding the contract method 0xeb6266e2.
//
// Solidity: function foreclose() returns()
func (_IApplication *IApplicationSession) Foreclose() (*types.Transaction, error) {
	return _IApplication.Contract.Foreclose(&_IApplication.TransactOpts)
}

// Foreclose is a paid mutator transaction binding the contract method 0xeb6266e2.
//
// Solidity: function foreclose() returns()
func (_IApplication *IApplicationTransactorSession) Foreclose() (*types.Transaction, error) {
	return _IApplication.Contract.Foreclose(&_IApplication.TransactOpts)
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

// ProveAccountsDriveMerkleRoot is a paid mutator transaction binding the contract method 0xbe77e2c4.
//
// Solidity: function proveAccountsDriveMerkleRoot(bytes32 accountsDriveMerkleRoot, bytes32[] proof) returns()
func (_IApplication *IApplicationTransactor) ProveAccountsDriveMerkleRoot(opts *bind.TransactOpts, accountsDriveMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _IApplication.contract.Transact(opts, "proveAccountsDriveMerkleRoot", accountsDriveMerkleRoot, proof)
}

// ProveAccountsDriveMerkleRoot is a paid mutator transaction binding the contract method 0xbe77e2c4.
//
// Solidity: function proveAccountsDriveMerkleRoot(bytes32 accountsDriveMerkleRoot, bytes32[] proof) returns()
func (_IApplication *IApplicationSession) ProveAccountsDriveMerkleRoot(accountsDriveMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _IApplication.Contract.ProveAccountsDriveMerkleRoot(&_IApplication.TransactOpts, accountsDriveMerkleRoot, proof)
}

// ProveAccountsDriveMerkleRoot is a paid mutator transaction binding the contract method 0xbe77e2c4.
//
// Solidity: function proveAccountsDriveMerkleRoot(bytes32 accountsDriveMerkleRoot, bytes32[] proof) returns()
func (_IApplication *IApplicationTransactorSession) ProveAccountsDriveMerkleRoot(accountsDriveMerkleRoot [32]byte, proof [][32]byte) (*types.Transaction, error) {
	return _IApplication.Contract.ProveAccountsDriveMerkleRoot(&_IApplication.TransactOpts, accountsDriveMerkleRoot, proof)
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

// Withdraw is a paid mutator transaction binding the contract method 0xbcf8023f.
//
// Solidity: function withdraw(bytes account, (uint64,bytes32[]) proof) returns()
func (_IApplication *IApplicationTransactor) Withdraw(opts *bind.TransactOpts, account []byte, proof AccountValidityProof) (*types.Transaction, error) {
	return _IApplication.contract.Transact(opts, "withdraw", account, proof)
}

// Withdraw is a paid mutator transaction binding the contract method 0xbcf8023f.
//
// Solidity: function withdraw(bytes account, (uint64,bytes32[]) proof) returns()
func (_IApplication *IApplicationSession) Withdraw(account []byte, proof AccountValidityProof) (*types.Transaction, error) {
	return _IApplication.Contract.Withdraw(&_IApplication.TransactOpts, account, proof)
}

// Withdraw is a paid mutator transaction binding the contract method 0xbcf8023f.
//
// Solidity: function withdraw(bytes account, (uint64,bytes32[]) proof) returns()
func (_IApplication *IApplicationTransactorSession) Withdraw(account []byte, proof AccountValidityProof) (*types.Transaction, error) {
	return _IApplication.Contract.Withdraw(&_IApplication.TransactOpts, account, proof)
}

// IApplicationAccountsDriveMerkleRootProvedIterator is returned from FilterAccountsDriveMerkleRootProved and is used to iterate over the raw logs and unpacked data for AccountsDriveMerkleRootProved events raised by the IApplication contract.
type IApplicationAccountsDriveMerkleRootProvedIterator struct {
	Event *IApplicationAccountsDriveMerkleRootProved // Event containing the contract specifics and raw log

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
func (it *IApplicationAccountsDriveMerkleRootProvedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IApplicationAccountsDriveMerkleRootProved)
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
		it.Event = new(IApplicationAccountsDriveMerkleRootProved)
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
func (it *IApplicationAccountsDriveMerkleRootProvedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IApplicationAccountsDriveMerkleRootProvedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IApplicationAccountsDriveMerkleRootProved represents a AccountsDriveMerkleRootProved event raised by the IApplication contract.
type IApplicationAccountsDriveMerkleRootProved struct {
	AccountsDriveMerkleRoot [32]byte
	Raw                     types.Log // Blockchain specific contextual infos
}

// FilterAccountsDriveMerkleRootProved is a free log retrieval operation binding the contract event 0x421863fbad9f3586640ffad00109861693263a28f6e97c679f45c3cbf5263594.
//
// Solidity: event AccountsDriveMerkleRootProved(bytes32 accountsDriveMerkleRoot)
func (_IApplication *IApplicationFilterer) FilterAccountsDriveMerkleRootProved(opts *bind.FilterOpts) (*IApplicationAccountsDriveMerkleRootProvedIterator, error) {

	logs, sub, err := _IApplication.contract.FilterLogs(opts, "AccountsDriveMerkleRootProved")
	if err != nil {
		return nil, err
	}
	return &IApplicationAccountsDriveMerkleRootProvedIterator{contract: _IApplication.contract, event: "AccountsDriveMerkleRootProved", logs: logs, sub: sub}, nil
}

// WatchAccountsDriveMerkleRootProved is a free log subscription operation binding the contract event 0x421863fbad9f3586640ffad00109861693263a28f6e97c679f45c3cbf5263594.
//
// Solidity: event AccountsDriveMerkleRootProved(bytes32 accountsDriveMerkleRoot)
func (_IApplication *IApplicationFilterer) WatchAccountsDriveMerkleRootProved(opts *bind.WatchOpts, sink chan<- *IApplicationAccountsDriveMerkleRootProved) (event.Subscription, error) {

	logs, sub, err := _IApplication.contract.WatchLogs(opts, "AccountsDriveMerkleRootProved")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IApplicationAccountsDriveMerkleRootProved)
				if err := _IApplication.contract.UnpackLog(event, "AccountsDriveMerkleRootProved", log); err != nil {
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

// ParseAccountsDriveMerkleRootProved is a log parse operation binding the contract event 0x421863fbad9f3586640ffad00109861693263a28f6e97c679f45c3cbf5263594.
//
// Solidity: event AccountsDriveMerkleRootProved(bytes32 accountsDriveMerkleRoot)
func (_IApplication *IApplicationFilterer) ParseAccountsDriveMerkleRootProved(log types.Log) (*IApplicationAccountsDriveMerkleRootProved, error) {
	event := new(IApplicationAccountsDriveMerkleRootProved)
	if err := _IApplication.contract.UnpackLog(event, "AccountsDriveMerkleRootProved", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// IApplicationForeclosureIterator is returned from FilterForeclosure and is used to iterate over the raw logs and unpacked data for Foreclosure events raised by the IApplication contract.
type IApplicationForeclosureIterator struct {
	Event *IApplicationForeclosure // Event containing the contract specifics and raw log

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
func (it *IApplicationForeclosureIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IApplicationForeclosure)
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
		it.Event = new(IApplicationForeclosure)
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
func (it *IApplicationForeclosureIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IApplicationForeclosureIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IApplicationForeclosure represents a Foreclosure event raised by the IApplication contract.
type IApplicationForeclosure struct {
	Raw types.Log // Blockchain specific contextual infos
}

// FilterForeclosure is a free log retrieval operation binding the contract event 0xd10ac0ca4adfcec0fa841963d75fefd049b7cd20555173c0673d9b2bfdb9d3ac.
//
// Solidity: event Foreclosure()
func (_IApplication *IApplicationFilterer) FilterForeclosure(opts *bind.FilterOpts) (*IApplicationForeclosureIterator, error) {

	logs, sub, err := _IApplication.contract.FilterLogs(opts, "Foreclosure")
	if err != nil {
		return nil, err
	}
	return &IApplicationForeclosureIterator{contract: _IApplication.contract, event: "Foreclosure", logs: logs, sub: sub}, nil
}

// WatchForeclosure is a free log subscription operation binding the contract event 0xd10ac0ca4adfcec0fa841963d75fefd049b7cd20555173c0673d9b2bfdb9d3ac.
//
// Solidity: event Foreclosure()
func (_IApplication *IApplicationFilterer) WatchForeclosure(opts *bind.WatchOpts, sink chan<- *IApplicationForeclosure) (event.Subscription, error) {

	logs, sub, err := _IApplication.contract.WatchLogs(opts, "Foreclosure")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IApplicationForeclosure)
				if err := _IApplication.contract.UnpackLog(event, "Foreclosure", log); err != nil {
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

// ParseForeclosure is a log parse operation binding the contract event 0xd10ac0ca4adfcec0fa841963d75fefd049b7cd20555173c0673d9b2bfdb9d3ac.
//
// Solidity: event Foreclosure()
func (_IApplication *IApplicationFilterer) ParseForeclosure(log types.Log) (*IApplicationForeclosure, error) {
	event := new(IApplicationForeclosure)
	if err := _IApplication.contract.UnpackLog(event, "Foreclosure", log); err != nil {
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

// IApplicationWithdrawalIterator is returned from FilterWithdrawal and is used to iterate over the raw logs and unpacked data for Withdrawal events raised by the IApplication contract.
type IApplicationWithdrawalIterator struct {
	Event *IApplicationWithdrawal // Event containing the contract specifics and raw log

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
func (it *IApplicationWithdrawalIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(IApplicationWithdrawal)
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
		it.Event = new(IApplicationWithdrawal)
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
func (it *IApplicationWithdrawalIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *IApplicationWithdrawalIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// IApplicationWithdrawal represents a Withdrawal event raised by the IApplication contract.
type IApplicationWithdrawal struct {
	AccountIndex uint64
	Account      []byte
	Output       []byte
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterWithdrawal is a free log retrieval operation binding the contract event 0xde17c4fe795586e35da70cf61f10d8b19542b1eaf30daf7670e8ae438908ba59.
//
// Solidity: event Withdrawal(uint64 accountIndex, bytes account, bytes output)
func (_IApplication *IApplicationFilterer) FilterWithdrawal(opts *bind.FilterOpts) (*IApplicationWithdrawalIterator, error) {

	logs, sub, err := _IApplication.contract.FilterLogs(opts, "Withdrawal")
	if err != nil {
		return nil, err
	}
	return &IApplicationWithdrawalIterator{contract: _IApplication.contract, event: "Withdrawal", logs: logs, sub: sub}, nil
}

// WatchWithdrawal is a free log subscription operation binding the contract event 0xde17c4fe795586e35da70cf61f10d8b19542b1eaf30daf7670e8ae438908ba59.
//
// Solidity: event Withdrawal(uint64 accountIndex, bytes account, bytes output)
func (_IApplication *IApplicationFilterer) WatchWithdrawal(opts *bind.WatchOpts, sink chan<- *IApplicationWithdrawal) (event.Subscription, error) {

	logs, sub, err := _IApplication.contract.WatchLogs(opts, "Withdrawal")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(IApplicationWithdrawal)
				if err := _IApplication.contract.UnpackLog(event, "Withdrawal", log); err != nil {
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

// ParseWithdrawal is a log parse operation binding the contract event 0xde17c4fe795586e35da70cf61f10d8b19542b1eaf30daf7670e8ae438908ba59.
//
// Solidity: event Withdrawal(uint64 accountIndex, bytes account, bytes output)
func (_IApplication *IApplicationFilterer) ParseWithdrawal(log types.Log) (*IApplicationWithdrawal, error) {
	event := new(IApplicationWithdrawal)
	if err := _IApplication.contract.UnpackLog(event, "Withdrawal", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
