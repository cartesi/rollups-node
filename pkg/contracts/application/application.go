// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package application

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

// InputRange is an auto generated low-level Go binding around an user-defined struct.
type InputRange struct {
	FirstIndex uint64
	LastIndex  uint64
}

// OutputValidityProof is an auto generated low-level Go binding around an user-defined struct.
type OutputValidityProof struct {
	InputRange                       InputRange
	InputIndexWithinEpoch            uint64
	OutputIndexWithinInput           uint64
	OutputHashesRootHash             [32]byte
	OutputsEpochRootHash             [32]byte
	MachineStateHash                 [32]byte
	OutputHashInOutputHashesSiblings [][32]byte
	OutputHashesInEpochSiblings      [][32]byte
}

// ApplicationMetaData contains all meta data concerning the Application contract.
var ApplicationMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"contractIConsensus\",\"name\":\"consensus\",\"type\":\"address\"},{\"internalType\":\"contractIInputBox\",\"name\":\"inputBox\",\"type\":\"address\"},{\"internalType\":\"contractIPortal[]\",\"name\":\"portals\",\"type\":\"address[]\"},{\"internalType\":\"address\",\"name\":\"initialOwner\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"templateHash\",\"type\":\"bytes32\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"IncorrectEpochHash\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"IncorrectOutputHashesRootHash\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"IncorrectOutputsEpochRootHash\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"inputIndex\",\"type\":\"uint256\"},{\"components\":[{\"internalType\":\"uint64\",\"name\":\"firstIndex\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"lastIndex\",\"type\":\"uint64\"}],\"internalType\":\"structInputRange\",\"name\":\"inputRange\",\"type\":\"tuple\"}],\"name\":\"InputIndexOutOfRange\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"output\",\"type\":\"bytes\"}],\"name\":\"OutputNotExecutable\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"output\",\"type\":\"bytes\"}],\"name\":\"OutputNotReexecutable\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"owner\",\"type\":\"address\"}],\"name\":\"OwnableInvalidOwner\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"OwnableUnauthorizedAccount\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"ReentrancyGuardReentrantCall\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"contractIConsensus\",\"name\":\"newConsensus\",\"type\":\"address\"}],\"name\":\"NewConsensus\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"inputIndex\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"uint64\",\"name\":\"outputIndexWithinInput\",\"type\":\"uint64\"},{\"indexed\":false,\"internalType\":\"bytes\",\"name\":\"output\",\"type\":\"bytes\"}],\"name\":\"OutputExecuted\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"previousOwner\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"OwnershipTransferred\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"output\",\"type\":\"bytes\"},{\"components\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"firstIndex\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"lastIndex\",\"type\":\"uint64\"}],\"internalType\":\"structInputRange\",\"name\":\"inputRange\",\"type\":\"tuple\"},{\"internalType\":\"uint64\",\"name\":\"inputIndexWithinEpoch\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"outputIndexWithinInput\",\"type\":\"uint64\"},{\"internalType\":\"bytes32\",\"name\":\"outputHashesRootHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"outputsEpochRootHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"machineStateHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32[]\",\"name\":\"outputHashInOutputHashesSiblings\",\"type\":\"bytes32[]\"},{\"internalType\":\"bytes32[]\",\"name\":\"outputHashesInEpochSiblings\",\"type\":\"bytes32[]\"}],\"internalType\":\"structOutputValidityProof\",\"name\":\"proof\",\"type\":\"tuple\"}],\"name\":\"executeOutput\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getConsensus\",\"outputs\":[{\"internalType\":\"contractIConsensus\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getInputBox\",\"outputs\":[{\"internalType\":\"contractIInputBox\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getPortals\",\"outputs\":[{\"internalType\":\"contractIPortal[]\",\"name\":\"\",\"type\":\"address[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getTemplateHash\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"contractIConsensus\",\"name\":\"newConsensus\",\"type\":\"address\"}],\"name\":\"migrateToConsensus\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"uint256[]\",\"name\":\"\",\"type\":\"uint256[]\"},{\"internalType\":\"uint256[]\",\"name\":\"\",\"type\":\"uint256[]\"},{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"}],\"name\":\"onERC1155BatchReceived\",\"outputs\":[{\"internalType\":\"bytes4\",\"name\":\"\",\"type\":\"bytes4\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"}],\"name\":\"onERC1155Received\",\"outputs\":[{\"internalType\":\"bytes4\",\"name\":\"\",\"type\":\"bytes4\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"\",\"type\":\"bytes\"}],\"name\":\"onERC721Received\",\"outputs\":[{\"internalType\":\"bytes4\",\"name\":\"\",\"type\":\"bytes4\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"renounceOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes4\",\"name\":\"interfaceId\",\"type\":\"bytes4\"}],\"name\":\"supportsInterface\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"transferOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"output\",\"type\":\"bytes\"},{\"components\":[{\"components\":[{\"internalType\":\"uint64\",\"name\":\"firstIndex\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"lastIndex\",\"type\":\"uint64\"}],\"internalType\":\"structInputRange\",\"name\":\"inputRange\",\"type\":\"tuple\"},{\"internalType\":\"uint64\",\"name\":\"inputIndexWithinEpoch\",\"type\":\"uint64\"},{\"internalType\":\"uint64\",\"name\":\"outputIndexWithinInput\",\"type\":\"uint64\"},{\"internalType\":\"bytes32\",\"name\":\"outputHashesRootHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"outputsEpochRootHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"machineStateHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32[]\",\"name\":\"outputHashInOutputHashesSiblings\",\"type\":\"bytes32[]\"},{\"internalType\":\"bytes32[]\",\"name\":\"outputHashesInEpochSiblings\",\"type\":\"bytes32[]\"}],\"internalType\":\"structOutputValidityProof\",\"name\":\"proof\",\"type\":\"tuple\"}],\"name\":\"validateOutput\",\"outputs\":[],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"inputIndex\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"outputIndexWithinInput\",\"type\":\"uint256\"}],\"name\":\"wasOutputExecuted\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"stateMutability\":\"payable\",\"type\":\"receive\"}]",
}

// ApplicationABI is the input ABI used to generate the binding from.
// Deprecated: Use ApplicationMetaData.ABI instead.
var ApplicationABI = ApplicationMetaData.ABI

// Application is an auto generated Go binding around an Ethereum contract.
type Application struct {
	ApplicationCaller     // Read-only binding to the contract
	ApplicationTransactor // Write-only binding to the contract
	ApplicationFilterer   // Log filterer for contract events
}

// ApplicationCaller is an auto generated read-only Go binding around an Ethereum contract.
type ApplicationCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ApplicationTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ApplicationTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ApplicationFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ApplicationFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ApplicationSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ApplicationSession struct {
	Contract     *Application      // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// ApplicationCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ApplicationCallerSession struct {
	Contract *ApplicationCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts      // Call options to use throughout this session
}

// ApplicationTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ApplicationTransactorSession struct {
	Contract     *ApplicationTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts      // Transaction auth options to use throughout this session
}

// ApplicationRaw is an auto generated low-level Go binding around an Ethereum contract.
type ApplicationRaw struct {
	Contract *Application // Generic contract binding to access the raw methods on
}

// ApplicationCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ApplicationCallerRaw struct {
	Contract *ApplicationCaller // Generic read-only contract binding to access the raw methods on
}

// ApplicationTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ApplicationTransactorRaw struct {
	Contract *ApplicationTransactor // Generic write-only contract binding to access the raw methods on
}

// NewApplication creates a new instance of Application, bound to a specific deployed contract.
func NewApplication(address common.Address, backend bind.ContractBackend) (*Application, error) {
	contract, err := bindApplication(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Application{ApplicationCaller: ApplicationCaller{contract: contract}, ApplicationTransactor: ApplicationTransactor{contract: contract}, ApplicationFilterer: ApplicationFilterer{contract: contract}}, nil
}

// NewApplicationCaller creates a new read-only instance of Application, bound to a specific deployed contract.
func NewApplicationCaller(address common.Address, caller bind.ContractCaller) (*ApplicationCaller, error) {
	contract, err := bindApplication(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ApplicationCaller{contract: contract}, nil
}

// NewApplicationTransactor creates a new write-only instance of Application, bound to a specific deployed contract.
func NewApplicationTransactor(address common.Address, transactor bind.ContractTransactor) (*ApplicationTransactor, error) {
	contract, err := bindApplication(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ApplicationTransactor{contract: contract}, nil
}

// NewApplicationFilterer creates a new log filterer instance of Application, bound to a specific deployed contract.
func NewApplicationFilterer(address common.Address, filterer bind.ContractFilterer) (*ApplicationFilterer, error) {
	contract, err := bindApplication(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ApplicationFilterer{contract: contract}, nil
}

// bindApplication binds a generic wrapper to an already deployed contract.
func bindApplication(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := ApplicationMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Application *ApplicationRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Application.Contract.ApplicationCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Application *ApplicationRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Application.Contract.ApplicationTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Application *ApplicationRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Application.Contract.ApplicationTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Application *ApplicationCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Application.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Application *ApplicationTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Application.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Application *ApplicationTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Application.Contract.contract.Transact(opts, method, params...)
}

// GetConsensus is a free data retrieval call binding the contract method 0x179e740b.
//
// Solidity: function getConsensus() view returns(address)
func (_Application *ApplicationCaller) GetConsensus(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Application.contract.Call(opts, &out, "getConsensus")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetConsensus is a free data retrieval call binding the contract method 0x179e740b.
//
// Solidity: function getConsensus() view returns(address)
func (_Application *ApplicationSession) GetConsensus() (common.Address, error) {
	return _Application.Contract.GetConsensus(&_Application.CallOpts)
}

// GetConsensus is a free data retrieval call binding the contract method 0x179e740b.
//
// Solidity: function getConsensus() view returns(address)
func (_Application *ApplicationCallerSession) GetConsensus() (common.Address, error) {
	return _Application.Contract.GetConsensus(&_Application.CallOpts)
}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_Application *ApplicationCaller) GetInputBox(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Application.contract.Call(opts, &out, "getInputBox")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_Application *ApplicationSession) GetInputBox() (common.Address, error) {
	return _Application.Contract.GetInputBox(&_Application.CallOpts)
}

// GetInputBox is a free data retrieval call binding the contract method 0x00aace9a.
//
// Solidity: function getInputBox() view returns(address)
func (_Application *ApplicationCallerSession) GetInputBox() (common.Address, error) {
	return _Application.Contract.GetInputBox(&_Application.CallOpts)
}

// GetPortals is a free data retrieval call binding the contract method 0x108e8c1d.
//
// Solidity: function getPortals() view returns(address[])
func (_Application *ApplicationCaller) GetPortals(opts *bind.CallOpts) ([]common.Address, error) {
	var out []interface{}
	err := _Application.contract.Call(opts, &out, "getPortals")

	if err != nil {
		return *new([]common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new([]common.Address)).(*[]common.Address)

	return out0, err

}

// GetPortals is a free data retrieval call binding the contract method 0x108e8c1d.
//
// Solidity: function getPortals() view returns(address[])
func (_Application *ApplicationSession) GetPortals() ([]common.Address, error) {
	return _Application.Contract.GetPortals(&_Application.CallOpts)
}

// GetPortals is a free data retrieval call binding the contract method 0x108e8c1d.
//
// Solidity: function getPortals() view returns(address[])
func (_Application *ApplicationCallerSession) GetPortals() ([]common.Address, error) {
	return _Application.Contract.GetPortals(&_Application.CallOpts)
}

// GetTemplateHash is a free data retrieval call binding the contract method 0x61b12c66.
//
// Solidity: function getTemplateHash() view returns(bytes32)
func (_Application *ApplicationCaller) GetTemplateHash(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _Application.contract.Call(opts, &out, "getTemplateHash")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetTemplateHash is a free data retrieval call binding the contract method 0x61b12c66.
//
// Solidity: function getTemplateHash() view returns(bytes32)
func (_Application *ApplicationSession) GetTemplateHash() ([32]byte, error) {
	return _Application.Contract.GetTemplateHash(&_Application.CallOpts)
}

// GetTemplateHash is a free data retrieval call binding the contract method 0x61b12c66.
//
// Solidity: function getTemplateHash() view returns(bytes32)
func (_Application *ApplicationCallerSession) GetTemplateHash() ([32]byte, error) {
	return _Application.Contract.GetTemplateHash(&_Application.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Application *ApplicationCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Application.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Application *ApplicationSession) Owner() (common.Address, error) {
	return _Application.Contract.Owner(&_Application.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Application *ApplicationCallerSession) Owner() (common.Address, error) {
	return _Application.Contract.Owner(&_Application.CallOpts)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_Application *ApplicationCaller) SupportsInterface(opts *bind.CallOpts, interfaceId [4]byte) (bool, error) {
	var out []interface{}
	err := _Application.contract.Call(opts, &out, "supportsInterface", interfaceId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_Application *ApplicationSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _Application.Contract.SupportsInterface(&_Application.CallOpts, interfaceId)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_Application *ApplicationCallerSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _Application.Contract.SupportsInterface(&_Application.CallOpts, interfaceId)
}

// ValidateOutput is a free data retrieval call binding the contract method 0x4dcea155.
//
// Solidity: function validateOutput(bytes output, ((uint64,uint64),uint64,uint64,bytes32,bytes32,bytes32,bytes32[],bytes32[]) proof) view returns()
func (_Application *ApplicationCaller) ValidateOutput(opts *bind.CallOpts, output []byte, proof OutputValidityProof) error {
	var out []interface{}
	err := _Application.contract.Call(opts, &out, "validateOutput", output, proof)

	if err != nil {
		return err
	}

	return err

}

// ValidateOutput is a free data retrieval call binding the contract method 0x4dcea155.
//
// Solidity: function validateOutput(bytes output, ((uint64,uint64),uint64,uint64,bytes32,bytes32,bytes32,bytes32[],bytes32[]) proof) view returns()
func (_Application *ApplicationSession) ValidateOutput(output []byte, proof OutputValidityProof) error {
	return _Application.Contract.ValidateOutput(&_Application.CallOpts, output, proof)
}

// ValidateOutput is a free data retrieval call binding the contract method 0x4dcea155.
//
// Solidity: function validateOutput(bytes output, ((uint64,uint64),uint64,uint64,bytes32,bytes32,bytes32,bytes32[],bytes32[]) proof) view returns()
func (_Application *ApplicationCallerSession) ValidateOutput(output []byte, proof OutputValidityProof) error {
	return _Application.Contract.ValidateOutput(&_Application.CallOpts, output, proof)
}

// WasOutputExecuted is a free data retrieval call binding the contract method 0x24523192.
//
// Solidity: function wasOutputExecuted(uint256 inputIndex, uint256 outputIndexWithinInput) view returns(bool)
func (_Application *ApplicationCaller) WasOutputExecuted(opts *bind.CallOpts, inputIndex *big.Int, outputIndexWithinInput *big.Int) (bool, error) {
	var out []interface{}
	err := _Application.contract.Call(opts, &out, "wasOutputExecuted", inputIndex, outputIndexWithinInput)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// WasOutputExecuted is a free data retrieval call binding the contract method 0x24523192.
//
// Solidity: function wasOutputExecuted(uint256 inputIndex, uint256 outputIndexWithinInput) view returns(bool)
func (_Application *ApplicationSession) WasOutputExecuted(inputIndex *big.Int, outputIndexWithinInput *big.Int) (bool, error) {
	return _Application.Contract.WasOutputExecuted(&_Application.CallOpts, inputIndex, outputIndexWithinInput)
}

// WasOutputExecuted is a free data retrieval call binding the contract method 0x24523192.
//
// Solidity: function wasOutputExecuted(uint256 inputIndex, uint256 outputIndexWithinInput) view returns(bool)
func (_Application *ApplicationCallerSession) WasOutputExecuted(inputIndex *big.Int, outputIndexWithinInput *big.Int) (bool, error) {
	return _Application.Contract.WasOutputExecuted(&_Application.CallOpts, inputIndex, outputIndexWithinInput)
}

// ExecuteOutput is a paid mutator transaction binding the contract method 0xdbe1a6eb.
//
// Solidity: function executeOutput(bytes output, ((uint64,uint64),uint64,uint64,bytes32,bytes32,bytes32,bytes32[],bytes32[]) proof) returns()
func (_Application *ApplicationTransactor) ExecuteOutput(opts *bind.TransactOpts, output []byte, proof OutputValidityProof) (*types.Transaction, error) {
	return _Application.contract.Transact(opts, "executeOutput", output, proof)
}

// ExecuteOutput is a paid mutator transaction binding the contract method 0xdbe1a6eb.
//
// Solidity: function executeOutput(bytes output, ((uint64,uint64),uint64,uint64,bytes32,bytes32,bytes32,bytes32[],bytes32[]) proof) returns()
func (_Application *ApplicationSession) ExecuteOutput(output []byte, proof OutputValidityProof) (*types.Transaction, error) {
	return _Application.Contract.ExecuteOutput(&_Application.TransactOpts, output, proof)
}

// ExecuteOutput is a paid mutator transaction binding the contract method 0xdbe1a6eb.
//
// Solidity: function executeOutput(bytes output, ((uint64,uint64),uint64,uint64,bytes32,bytes32,bytes32,bytes32[],bytes32[]) proof) returns()
func (_Application *ApplicationTransactorSession) ExecuteOutput(output []byte, proof OutputValidityProof) (*types.Transaction, error) {
	return _Application.Contract.ExecuteOutput(&_Application.TransactOpts, output, proof)
}

// MigrateToConsensus is a paid mutator transaction binding the contract method 0xfc411683.
//
// Solidity: function migrateToConsensus(address newConsensus) returns()
func (_Application *ApplicationTransactor) MigrateToConsensus(opts *bind.TransactOpts, newConsensus common.Address) (*types.Transaction, error) {
	return _Application.contract.Transact(opts, "migrateToConsensus", newConsensus)
}

// MigrateToConsensus is a paid mutator transaction binding the contract method 0xfc411683.
//
// Solidity: function migrateToConsensus(address newConsensus) returns()
func (_Application *ApplicationSession) MigrateToConsensus(newConsensus common.Address) (*types.Transaction, error) {
	return _Application.Contract.MigrateToConsensus(&_Application.TransactOpts, newConsensus)
}

// MigrateToConsensus is a paid mutator transaction binding the contract method 0xfc411683.
//
// Solidity: function migrateToConsensus(address newConsensus) returns()
func (_Application *ApplicationTransactorSession) MigrateToConsensus(newConsensus common.Address) (*types.Transaction, error) {
	return _Application.Contract.MigrateToConsensus(&_Application.TransactOpts, newConsensus)
}

// OnERC1155BatchReceived is a paid mutator transaction binding the contract method 0xbc197c81.
//
// Solidity: function onERC1155BatchReceived(address , address , uint256[] , uint256[] , bytes ) returns(bytes4)
func (_Application *ApplicationTransactor) OnERC1155BatchReceived(opts *bind.TransactOpts, arg0 common.Address, arg1 common.Address, arg2 []*big.Int, arg3 []*big.Int, arg4 []byte) (*types.Transaction, error) {
	return _Application.contract.Transact(opts, "onERC1155BatchReceived", arg0, arg1, arg2, arg3, arg4)
}

// OnERC1155BatchReceived is a paid mutator transaction binding the contract method 0xbc197c81.
//
// Solidity: function onERC1155BatchReceived(address , address , uint256[] , uint256[] , bytes ) returns(bytes4)
func (_Application *ApplicationSession) OnERC1155BatchReceived(arg0 common.Address, arg1 common.Address, arg2 []*big.Int, arg3 []*big.Int, arg4 []byte) (*types.Transaction, error) {
	return _Application.Contract.OnERC1155BatchReceived(&_Application.TransactOpts, arg0, arg1, arg2, arg3, arg4)
}

// OnERC1155BatchReceived is a paid mutator transaction binding the contract method 0xbc197c81.
//
// Solidity: function onERC1155BatchReceived(address , address , uint256[] , uint256[] , bytes ) returns(bytes4)
func (_Application *ApplicationTransactorSession) OnERC1155BatchReceived(arg0 common.Address, arg1 common.Address, arg2 []*big.Int, arg3 []*big.Int, arg4 []byte) (*types.Transaction, error) {
	return _Application.Contract.OnERC1155BatchReceived(&_Application.TransactOpts, arg0, arg1, arg2, arg3, arg4)
}

// OnERC1155Received is a paid mutator transaction binding the contract method 0xf23a6e61.
//
// Solidity: function onERC1155Received(address , address , uint256 , uint256 , bytes ) returns(bytes4)
func (_Application *ApplicationTransactor) OnERC1155Received(opts *bind.TransactOpts, arg0 common.Address, arg1 common.Address, arg2 *big.Int, arg3 *big.Int, arg4 []byte) (*types.Transaction, error) {
	return _Application.contract.Transact(opts, "onERC1155Received", arg0, arg1, arg2, arg3, arg4)
}

// OnERC1155Received is a paid mutator transaction binding the contract method 0xf23a6e61.
//
// Solidity: function onERC1155Received(address , address , uint256 , uint256 , bytes ) returns(bytes4)
func (_Application *ApplicationSession) OnERC1155Received(arg0 common.Address, arg1 common.Address, arg2 *big.Int, arg3 *big.Int, arg4 []byte) (*types.Transaction, error) {
	return _Application.Contract.OnERC1155Received(&_Application.TransactOpts, arg0, arg1, arg2, arg3, arg4)
}

// OnERC1155Received is a paid mutator transaction binding the contract method 0xf23a6e61.
//
// Solidity: function onERC1155Received(address , address , uint256 , uint256 , bytes ) returns(bytes4)
func (_Application *ApplicationTransactorSession) OnERC1155Received(arg0 common.Address, arg1 common.Address, arg2 *big.Int, arg3 *big.Int, arg4 []byte) (*types.Transaction, error) {
	return _Application.Contract.OnERC1155Received(&_Application.TransactOpts, arg0, arg1, arg2, arg3, arg4)
}

// OnERC721Received is a paid mutator transaction binding the contract method 0x150b7a02.
//
// Solidity: function onERC721Received(address , address , uint256 , bytes ) returns(bytes4)
func (_Application *ApplicationTransactor) OnERC721Received(opts *bind.TransactOpts, arg0 common.Address, arg1 common.Address, arg2 *big.Int, arg3 []byte) (*types.Transaction, error) {
	return _Application.contract.Transact(opts, "onERC721Received", arg0, arg1, arg2, arg3)
}

// OnERC721Received is a paid mutator transaction binding the contract method 0x150b7a02.
//
// Solidity: function onERC721Received(address , address , uint256 , bytes ) returns(bytes4)
func (_Application *ApplicationSession) OnERC721Received(arg0 common.Address, arg1 common.Address, arg2 *big.Int, arg3 []byte) (*types.Transaction, error) {
	return _Application.Contract.OnERC721Received(&_Application.TransactOpts, arg0, arg1, arg2, arg3)
}

// OnERC721Received is a paid mutator transaction binding the contract method 0x150b7a02.
//
// Solidity: function onERC721Received(address , address , uint256 , bytes ) returns(bytes4)
func (_Application *ApplicationTransactorSession) OnERC721Received(arg0 common.Address, arg1 common.Address, arg2 *big.Int, arg3 []byte) (*types.Transaction, error) {
	return _Application.Contract.OnERC721Received(&_Application.TransactOpts, arg0, arg1, arg2, arg3)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_Application *ApplicationTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Application.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_Application *ApplicationSession) RenounceOwnership() (*types.Transaction, error) {
	return _Application.Contract.RenounceOwnership(&_Application.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_Application *ApplicationTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _Application.Contract.RenounceOwnership(&_Application.TransactOpts)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_Application *ApplicationTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _Application.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_Application *ApplicationSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _Application.Contract.TransferOwnership(&_Application.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_Application *ApplicationTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _Application.Contract.TransferOwnership(&_Application.TransactOpts, newOwner)
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_Application *ApplicationTransactor) Receive(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Application.contract.RawTransact(opts, nil) // calldata is disallowed for receive function
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_Application *ApplicationSession) Receive() (*types.Transaction, error) {
	return _Application.Contract.Receive(&_Application.TransactOpts)
}

// Receive is a paid mutator transaction binding the contract receive function.
//
// Solidity: receive() payable returns()
func (_Application *ApplicationTransactorSession) Receive() (*types.Transaction, error) {
	return _Application.Contract.Receive(&_Application.TransactOpts)
}

// ApplicationNewConsensusIterator is returned from FilterNewConsensus and is used to iterate over the raw logs and unpacked data for NewConsensus events raised by the Application contract.
type ApplicationNewConsensusIterator struct {
	Event *ApplicationNewConsensus // Event containing the contract specifics and raw log

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
func (it *ApplicationNewConsensusIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ApplicationNewConsensus)
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
		it.Event = new(ApplicationNewConsensus)
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
func (it *ApplicationNewConsensusIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ApplicationNewConsensusIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ApplicationNewConsensus represents a NewConsensus event raised by the Application contract.
type ApplicationNewConsensus struct {
	NewConsensus common.Address
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterNewConsensus is a free log retrieval operation binding the contract event 0x4991c6f37185659e276ff918a96f3e20e6c5abcd8c9aab450dc19c2f7ad35cb5.
//
// Solidity: event NewConsensus(address newConsensus)
func (_Application *ApplicationFilterer) FilterNewConsensus(opts *bind.FilterOpts) (*ApplicationNewConsensusIterator, error) {

	logs, sub, err := _Application.contract.FilterLogs(opts, "NewConsensus")
	if err != nil {
		return nil, err
	}
	return &ApplicationNewConsensusIterator{contract: _Application.contract, event: "NewConsensus", logs: logs, sub: sub}, nil
}

// WatchNewConsensus is a free log subscription operation binding the contract event 0x4991c6f37185659e276ff918a96f3e20e6c5abcd8c9aab450dc19c2f7ad35cb5.
//
// Solidity: event NewConsensus(address newConsensus)
func (_Application *ApplicationFilterer) WatchNewConsensus(opts *bind.WatchOpts, sink chan<- *ApplicationNewConsensus) (event.Subscription, error) {

	logs, sub, err := _Application.contract.WatchLogs(opts, "NewConsensus")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ApplicationNewConsensus)
				if err := _Application.contract.UnpackLog(event, "NewConsensus", log); err != nil {
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
func (_Application *ApplicationFilterer) ParseNewConsensus(log types.Log) (*ApplicationNewConsensus, error) {
	event := new(ApplicationNewConsensus)
	if err := _Application.contract.UnpackLog(event, "NewConsensus", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ApplicationOutputExecutedIterator is returned from FilterOutputExecuted and is used to iterate over the raw logs and unpacked data for OutputExecuted events raised by the Application contract.
type ApplicationOutputExecutedIterator struct {
	Event *ApplicationOutputExecuted // Event containing the contract specifics and raw log

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
func (it *ApplicationOutputExecutedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ApplicationOutputExecuted)
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
		it.Event = new(ApplicationOutputExecuted)
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
func (it *ApplicationOutputExecutedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ApplicationOutputExecutedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ApplicationOutputExecuted represents a OutputExecuted event raised by the Application contract.
type ApplicationOutputExecuted struct {
	InputIndex             uint64
	OutputIndexWithinInput uint64
	Output                 []byte
	Raw                    types.Log // Blockchain specific contextual infos
}

// FilterOutputExecuted is a free log retrieval operation binding the contract event 0xd39d8e3e610251d36b5464d9cabbd8fa8319fe6cff76941ce041ecf04669726f.
//
// Solidity: event OutputExecuted(uint64 inputIndex, uint64 outputIndexWithinInput, bytes output)
func (_Application *ApplicationFilterer) FilterOutputExecuted(opts *bind.FilterOpts) (*ApplicationOutputExecutedIterator, error) {

	logs, sub, err := _Application.contract.FilterLogs(opts, "OutputExecuted")
	if err != nil {
		return nil, err
	}
	return &ApplicationOutputExecutedIterator{contract: _Application.contract, event: "OutputExecuted", logs: logs, sub: sub}, nil
}

// WatchOutputExecuted is a free log subscription operation binding the contract event 0xd39d8e3e610251d36b5464d9cabbd8fa8319fe6cff76941ce041ecf04669726f.
//
// Solidity: event OutputExecuted(uint64 inputIndex, uint64 outputIndexWithinInput, bytes output)
func (_Application *ApplicationFilterer) WatchOutputExecuted(opts *bind.WatchOpts, sink chan<- *ApplicationOutputExecuted) (event.Subscription, error) {

	logs, sub, err := _Application.contract.WatchLogs(opts, "OutputExecuted")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ApplicationOutputExecuted)
				if err := _Application.contract.UnpackLog(event, "OutputExecuted", log); err != nil {
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

// ParseOutputExecuted is a log parse operation binding the contract event 0xd39d8e3e610251d36b5464d9cabbd8fa8319fe6cff76941ce041ecf04669726f.
//
// Solidity: event OutputExecuted(uint64 inputIndex, uint64 outputIndexWithinInput, bytes output)
func (_Application *ApplicationFilterer) ParseOutputExecuted(log types.Log) (*ApplicationOutputExecuted, error) {
	event := new(ApplicationOutputExecuted)
	if err := _Application.contract.UnpackLog(event, "OutputExecuted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// ApplicationOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the Application contract.
type ApplicationOwnershipTransferredIterator struct {
	Event *ApplicationOwnershipTransferred // Event containing the contract specifics and raw log

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
func (it *ApplicationOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ApplicationOwnershipTransferred)
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
		it.Event = new(ApplicationOwnershipTransferred)
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
func (it *ApplicationOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ApplicationOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ApplicationOwnershipTransferred represents a OwnershipTransferred event raised by the Application contract.
type ApplicationOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_Application *ApplicationFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*ApplicationOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _Application.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &ApplicationOwnershipTransferredIterator{contract: _Application.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_Application *ApplicationFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *ApplicationOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _Application.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ApplicationOwnershipTransferred)
				if err := _Application.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
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

// ParseOwnershipTransferred is a log parse operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_Application *ApplicationFilterer) ParseOwnershipTransferred(log types.Log) (*ApplicationOwnershipTransferred, error) {
	event := new(ApplicationOwnershipTransferred)
	if err := _Application.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
